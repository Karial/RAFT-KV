package src

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/rpc"
	"os"
	"strings"
)

type Client struct {
	replicasAddresses []string
	lastLeader        string
}

func NewClient(replicasAddresses []string) *Client {
	return &Client{
		replicasAddresses: replicasAddresses,
	}
}

func (client *Client) Start() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		commandLine := scanner.Text()
		if strings.HasPrefix(commandLine, "Set") {
			splittedCommand := strings.Split(commandLine[4:], "\t")
			client.SetRequest(splittedCommand[0], splittedCommand[1])
		} else if strings.HasPrefix(commandLine, "Get") {
			splittedCommand := strings.Split(commandLine[4:], "\t")
			client.GetRequest(splittedCommand[0])
		}
	}
}

func (client *Client) SetRequest(key, value string) {
	command := CommandJson{}
	args, _ := json.Marshal(
		SetArgs{
			Key:   key,
			Value: value,
		})

	command.Type = Set
	command.Args = string(args)
	commandBytes, _ := json.Marshal(command)
	client.AppendCommand(string(commandBytes))
}

func (client *Client) GetRequest(key string) {
	command := CommandJson{}
	args, _ := json.Marshal(
		GetArgs{
			Key: key,
		})

	command.Type = Get
	command.Args = string(args)
	commandBytes, _ := json.Marshal(command)
	client.AppendCommand(string(commandBytes))
}

func (client *Client) AppendCommand(command string) {
	for {
		var currentReplica string
		if client.lastLeader == "" {
			currentReplica = client.ChooseRandomReplica()
		} else {
			currentReplica = client.lastLeader
		}

		currentReplicaClient, err := client.ConnectToReplica(currentReplica)
		if err != nil {
			log.Printf("Error connection to replica: %s", currentReplica)
			continue
		}

		resp := ACResponse{}
		call := <-currentReplicaClient.Go("Replica.AppendNewCommand", command, &resp, nil).Done
		if call.Error != nil {
			log.Printf("Error calling replica: %s", currentReplica)
			client.lastLeader = ""
			continue
		}

		if resp.Err != nil {
			client.lastLeader = resp.LeaderHint
			continue
		}

		fmt.Printf("Resonse Result: %s\n", resp.Result)
		break
	}
}

func (client *Client) ChooseRandomReplica() string {
	return client.replicasAddresses[rand.Intn(len(client.replicasAddresses))]
}

func (client *Client) ConnectToReplica(addr string) (*rpc.Client, error) {
	return rpc.DialHTTP("tcp", addr)
}
