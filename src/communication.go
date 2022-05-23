package src

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
)

type CommunicationService struct {
	replica *Replica

	replicasClients   map[string]*rpc.Client
	replicasIDs       []string
	replicasAddresses map[string]string
}

func NewCommunicationService(id string, replicasIDs []string, replicasAddresses []string) *CommunicationService {
	for i, replicaID := range replicasIDs {
		if replicaID == id {
			replicasIDs = append(replicasIDs[:i], replicasIDs[i+1:]...)
			replicasAddresses = append(replicasAddresses[:i], replicasAddresses[i+1:]...)
		}
	}

	commServ := new(CommunicationService)
	rsm := NewKVRSM()
	commServ.replica = NewReplica(id, replicasIDs, commServ, rsm)
	commServ.replicasClients = make(map[string]*rpc.Client)
	commServ.replicasIDs = replicasIDs

	commServ.replicasAddresses = make(map[string]string)
	for i := range replicasIDs {
		commServ.replicasAddresses[replicasIDs[i]] = replicasAddresses[i]
	}

	if len(replicasIDs) != len(replicasAddresses) {
		panic("Urls and ids for replicas does not have same length")
	}

	return commServ
}

func (commServ *CommunicationService) Start(port int) {
	for _, repID := range commServ.replicasIDs {
		commServ.ConnectToReplica(repID)
	}

	err := rpc.Register(commServ.replica)
	if err != nil {
		panic("Cannot start replica rpc")
	}
	rpc.HandleHTTP()

	list, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		panic("Cannot start server")
	}

	go commServ.replica.BecomeListener()

	err = http.Serve(list, nil)
	if err != nil {
		log.Fatal("Server shutdown with error")
	}
}

func (commServ *CommunicationService) ConnectToReplica(repID string) bool {
	replicaAddress := commServ.replicasAddresses[repID]
	replicaClient, err := rpc.DialHTTP("tcp", replicaAddress)
	if err != nil {
		log.Printf("Cannot connect to client for replica with id %s and address %s", repID, replicaAddress)
		return false
	}

	if commServ.replicasClients[repID] != nil {
		_ = commServ.replicasClients[repID].Close()
	}
	commServ.replicasClients[repID] = replicaClient
	return true
}

type RVRequest struct {
	Round        uint64
	RepID        string
	LastLogIndex int
	LastLogRound uint64
}

type RVResponse struct {
	Round uint64
	Voted bool
}

type NoClientConnection struct {
}

func (err *NoClientConnection) Error() string {
	return "No connection to replica client"
}

func (commServ *CommunicationService) RequestVote(repID string, req RVRequest) (*RVResponse, error) {
	if commServ.replicasClients[repID] == nil && !commServ.ConnectToReplica(repID) {
		return nil, &NoClientConnection{}
	}

	resp := RVResponse{}
	call := <-commServ.replicasClients[repID].Go("Replica.RequestVote", req, &resp, nil).Done
	if errors.Is(call.Error, rpc.ErrShutdown) {
		commServ.ConnectToReplica(repID)
		call = <-commServ.replicasClients[repID].Go("Replica.RequestVote", req, &resp, nil).Done
	}

	if call.Error != nil {
		return nil, call.Error
	}

	return &resp, nil
}

type AERequest struct {
	Round uint64
	RepID string

	PrevLogIndex int
	PrevLogRound uint64
	AppendLogs   []Log

	LastCommitIndex int
}

type AEResponse struct {
	Round   uint64
	Success bool
}

func (commServ *CommunicationService) AppendEntries(repID string, req AERequest) (*AEResponse, error) {
	if commServ.replicasClients[repID] == nil && !commServ.ConnectToReplica(repID) {
		return nil, &NoClientConnection{}
	}

	resp := AEResponse{}
	call := <-commServ.replicasClients[repID].Go("Replica.AppendEntries", req, &resp, nil).Done
	if errors.Is(call.Error, rpc.ErrShutdown) {
		commServ.ConnectToReplica(repID)
		call = <-commServ.replicasClients[repID].Go("Replica.AppendEntries", req, &resp, nil).Done
	}
	if call.Error != nil {
		return nil, call.Error
	}

	return &resp, nil
}
