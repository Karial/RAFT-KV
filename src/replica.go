package src

import (
	"log"
	"sync"
	"time"
)

type ReplicaState uint8

const (
	Listener ReplicaState = iota
	Candidate
	Leader
)

type Log struct {
	Command  Command
	Round    uint64
	Response *ACResponse
}

type Replica struct {
	id          string   // Replica unique ID
	replicasIDs []string // IDs of other replicas in cluster

	dataMu   sync.Mutex // Mutex to protect all shared data
	commitMu sync.Mutex // Mutex to protect commit calls

	commServ *CommunicationService // Service to communicate with other replicas
	rsm      RSM                   // Object that we replicate

	voteID string       // Replica vote for other replica
	state  ReplicaState // Current replica state

	currentLeader string // Leader for current round

	round uint64 // Current round number

	lastLeaderHeartbeat time.Time      // Last time leader did heartbeat us
	logs                []Log          // Logs of our replica
	nextIndex           map[string]int // Last index of log that saved on particular replica

	matchIndex      map[string]int // Highest index known to be replicated on replica
	lastCommitIndex int            // Index of last log that can be committed

	lastAppliedIndex int // Index of last applied to local machine log
}

func NewReplica(id string, replicasIDs []string, communicationService *CommunicationService, rsm RSM) *Replica {
	replica := &Replica{
		id:                  id,
		replicasIDs:         replicasIDs,
		dataMu:              sync.Mutex{},
		commitMu:            sync.Mutex{},
		voteID:              "",
		state:               Listener,
		round:               1,
		lastLeaderHeartbeat: time.Now(),
		commServ:            communicationService,
		rsm:                 rsm,
		logs:                make([]Log, 0),
		nextIndex:           make(map[string]int),
		matchIndex:          make(map[string]int),
		lastCommitIndex:     -1,
		lastAppliedIndex:    -1,
	}

	return replica
}

type ACResponse struct {
	Result     string
	LeaderHint string
	Err        error
	Done       chan struct{}
}

type NotLeaderError struct {
}

func (err *NotLeaderError) Error() string {
	return "I am not leader"
}

var (
	_ error = &NotLeaderError{}
)

// AppendNewCommand RPC call
func (rep *Replica) AppendNewCommand(command string, resp *ACResponse) error {
	rep.dataMu.Lock()

	// Only leader can append new commands to log
	if rep.state != Leader {
		log.Printf("[Replica %s] Cannot append command: %v because we are not leader.", rep.id, command)
		resp.Err = &NotLeaderError{}
		resp.LeaderHint = rep.currentLeader
		rep.dataMu.Unlock()
		return nil
	}

	rep.logs = append(rep.logs,
		Log{
			Command:  Command(command),
			Round:    rep.round,
			Response: resp,
		})
	log.Printf("[Replica %s] Successfully append command to log", rep.id)
	resp.Done = make(chan struct{})
	rep.dataMu.Unlock()

	<-resp.Done

	return nil
}

func (rep *Replica) Majority() int {
	return len(rep.replicasIDs)/2 + 1
}

// ContainsLog Should be called with mutex locked
func (rep *Replica) ContainsLog(index int, round uint64) bool {
	if index == -1 {
		return true
	}

	return index < len(rep.logs) && rep.logs[index].Round == round
}

func (rep *Replica) LastLogIndexAndRound() (int, uint64) {
	lastLogIndex, lastLogRound := len(rep.logs)-1, uint64(0)
	if lastLogIndex >= 0 {
		lastLogRound = rep.logs[lastLogIndex].Round
	}

	return lastLogIndex, lastLogRound
}

func (rep *Replica) NewerThanUs(index int, round uint64) bool {
	lastLogIndex, lastLogRound := rep.LastLogIndexAndRound()
	return round > lastLogRound || (round == lastLogRound && index >= lastLogIndex)
}

func (rep *Replica) WaitForLeaderHeartbeat() {
	rep.dataMu.Lock()
	currentRound := rep.round
	lastHeartbeatTime := rep.lastLeaderHeartbeat
	rep.dataMu.Unlock()

	log.Printf("Starting ticks loop on replica %s, in Round %d", rep.id, currentRound)

	for {
		// Wait for some time
		electionTimeout := ChooseRandomTimeout(150*time.Millisecond, 300*time.Millisecond)
		log.Printf("Replica %s waiting for heartbeat for %s in round %d", rep.id, electionTimeout.String(), currentRound)
		<-WaitUntil(lastHeartbeatTime.Add(electionTimeout))

		// Check if leader sent us heartbeat
		rep.dataMu.Lock()

		// If Round changed we should stop.
		if rep.round != currentRound {
			log.Printf("Replica: %s. Round changed, previous Round: %d, current Round: %d. Stop waiting for hearbeat",
				rep.id, currentRound, rep.round)
			rep.dataMu.Unlock()
			return
		}

		// If our state is not Listener or Candidate we should stop. If Round changed we should stop.
		if rep.state == Leader {
			log.Printf("Replica %s is leader, so it doesn't need to check heartbeat", rep.id)
			rep.dataMu.Unlock()
			return
		}

		// If leader didn't heartbeat us during this period, and we didn't vote for somebody, initialize new election
		if lastHeartbeatTime == rep.lastLeaderHeartbeat {
			log.Printf("Replica: %s. Leader didn't heartbeat and we didn't vote for somebody, so start election",
				rep.id)
			rep.dataMu.Unlock()
			rep.InitElection()
			return
		}

		lastHeartbeatTime = rep.lastLeaderHeartbeat
		rep.dataMu.Unlock()
	}
}

// RequestVote RPC Call
func (rep *Replica) RequestVote(req RVRequest, resp *RVResponse) error {
	rep.dataMu.Lock()
	log.Printf("Replica: %s. Replica %s asks us for vote", rep.id, req.RepID)

	if req.Round > rep.round {
		log.Printf("Replica %s got vote request with greater round %d", rep.id, req.Round)
		rep.round = req.Round
		rep.dataMu.Unlock()
		rep.BecomeListener()
		rep.dataMu.Lock()
	}

	if req.Round == rep.round && (rep.voteID == "" || rep.voteID == req.RepID) &&
		rep.NewerThanUs(req.LastLogIndex, req.LastLogRound) {
		log.Printf("Replica %s vote for replica %s", rep.id, req.RepID)
		resp.Voted = true
		rep.voteID = req.RepID
		rep.lastLeaderHeartbeat = time.Now()
	} else {
		log.Printf("Replica %s reject vote request for replica %s", rep.id, req.RepID)
		resp.Voted = false
	}

	resp.Round = rep.round
	rep.dataMu.Unlock()

	return nil
}

func (rep *Replica) InitElection() {
	rep.dataMu.Lock()
	rep.round++
	currentRound := rep.round
	lastLogIndex := len(rep.logs) - 1
	lastLogRound := uint64(0)
	if lastLogIndex >= 0 {
		lastLogRound = rep.logs[lastLogIndex].Round
	}
	rep.lastLeaderHeartbeat = time.Now()
	rep.dataMu.Unlock()

	log.Printf("Replica %s initialize election on Round %d.", rep.id, currentRound)

	callsDone := CreateCallResults(len(rep.replicasIDs))
	for i, repID := range rep.replicasIDs {
		go func(i int, repID string) {
			resp, err := rep.commServ.RequestVote(repID,
				RVRequest{
					Round:        rep.round,
					RepID:        rep.id,
					LastLogIndex: lastLogIndex,
					LastLogRound: lastLogRound,
				},
			)

			if err != nil {
				// If call failed we mark it as bad call result
				log.Printf("Replica: %s. Failed to get vote from replica %s, rps error", rep.id, repID)
				callsDone[i] <- CallResult{false, struct{}{}}
			} else if !resp.Voted {
				rep.dataMu.Lock()
				if resp.Round > currentRound {
					rep.round = resp.Round
					rep.dataMu.Unlock()
					rep.BecomeListener()
					rep.dataMu.Lock()
				}
				rep.dataMu.Unlock()

				// If replica don't vote for us, or it's Round not equal to our, then mark call as bad
				log.Printf("Replica: %s. Failed to get vote from replica %s, it didn't voted for us", rep.id, repID)
				callsDone[i] <- CallResult{false, struct{}{}}
			} else if resp.Round != currentRound {
				rep.dataMu.Lock()
				if resp.Round > rep.round {
					rep.round = resp.Round
					rep.BecomeListener()
				}
				rep.dataMu.Unlock()

				log.Printf("Replica: %s. Failed to get vote from replica %s, it voted in different round", rep.id, repID)
				callsDone[i] <- CallResult{false, struct{}{}}
			} else {
				// Otherwise, mark call as good
				callsDone[i] <- CallResult{true, struct{}{}}
			}
		}(i, repID)
	}

	_, err := Await(callsDone, rep.Majority()-1)
	if err == nil {
		log.Printf("Replica %s becomes leader in Round %d", rep.id, currentRound)
		rep.BecomeLeader()
	} else {
		log.Printf("Replica %s doesn't have enough votes for becoming leader in Round %d", rep.id, currentRound)
		rep.BecomeListener()
	}
}

func (rep *Replica) BecomeListener() {
	log.Printf("Replica: %s became listener on Round: %d\n", rep.id, rep.round)

	rep.dataMu.Lock()
	rep.state = Listener
	rep.voteID = ""
	rep.dataMu.Unlock()

	go rep.WaitForLeaderHeartbeat()
}

func (rep *Replica) BecomeCandidate() {
	// We change our state to candidate
	log.Printf("Replica %s becomes candidate", rep.id)
	rep.dataMu.Lock()
	rep.state = Candidate
	rep.dataMu.Unlock()

	rep.InitElection()
}

// AppendEntries RPC Call
func (rep *Replica) AppendEntries(req AERequest, resp *AEResponse) error {
	log.Printf("[Replica %s] Append entries: %v", rep.id, req)
	rep.dataMu.Lock()

	// Become listener if someone sends us heartbeat with greater Round
	if req.Round > rep.round {
		rep.round = req.Round
		rep.dataMu.Unlock()
		rep.BecomeListener()
		rep.dataMu.Lock()
	}

	if req.Round == rep.round {
		if rep.state != Listener {
			rep.dataMu.Unlock()
			rep.BecomeListener()
			rep.dataMu.Lock()
		}

		rep.currentLeader = req.RepID

		if rep.ContainsLog(req.PrevLogIndex, req.PrevLogRound) {
			log.Printf("[Replica %s] Successfully saved logs from leader", rep.id)
			resp.Success = true
			for i, logEntry := range req.AppendLogs {
				logsI := req.PrevLogIndex + 1 + i
				if logsI < len(rep.logs) && rep.logs[logsI].Round == logEntry.Round {
					continue
				}

				rep.logs = append(rep.logs[:logsI], req.AppendLogs[i:]...)
				break
			}

			if req.LastCommitIndex > rep.lastCommitIndex {
				log.Printf("[Replica %s] Leader commit logs from index: %d", rep.id, req.LastCommitIndex)
				rep.lastCommitIndex = req.LastCommitIndex
				if rep.lastCommitIndex > len(rep.logs) {
					rep.lastCommitIndex = len(rep.logs) - 1
				}

				go rep.TryCommit()
			}
		}

		rep.lastLeaderHeartbeat = time.Now()
	}

	resp.Round = rep.round
	rep.dataMu.Unlock()

	return nil
}

func (rep *Replica) Commit() {
	log.Printf("[Replica %s] Comitting logs", rep.id)
	rep.commitMu.Lock()
	rep.commitMu.Unlock()

	rep.dataMu.Lock()

	var logsToApply []Log
	if rep.lastAppliedIndex < rep.lastCommitIndex {
		log.Printf("[Replica %s] Commit logs from %d to %d", rep.id, rep.lastAppliedIndex+1, rep.lastCommitIndex)
		logsToApply = rep.logs[rep.lastAppliedIndex+1 : rep.lastCommitIndex+1]
		rep.lastAppliedIndex = rep.lastCommitIndex
	}

	rep.dataMu.Unlock()

	for _, logEntry := range logsToApply {
		// Apply log
		log.Printf("[Replica %s] Apply log: %v", rep.id, logEntry)
		result, err := rep.rsm.Apply(logEntry.Command)
		if err != nil {
			logEntry.Response.Err = err
		} else {
			logEntry.Response.Result = result
		}

		if logEntry.Response.Done != nil {
			close(logEntry.Response.Done)
		}
	}
}

func (rep *Replica) TryCommit() {
	log.Printf("[Replica %s] Trying to commit logs", rep.id)
	rep.dataMu.Lock()
	defer rep.dataMu.Unlock()

	haveSomethingToCommit := false
	for i := rep.lastCommitIndex + 1; i < len(rep.logs); i++ {
		if rep.logs[i].Round != rep.round {
			continue
		}
		replicatedOnReplicas := 1
		for _, repID := range rep.replicasIDs {
			if rep.matchIndex[repID] >= i {
				replicatedOnReplicas++
			}
		}
		log.Printf("[Replica %s] Log with index %d replicated on %d replicas", rep.id, i, replicatedOnReplicas)
		if replicatedOnReplicas >= rep.Majority() {
			haveSomethingToCommit = true
			rep.lastCommitIndex = i
		}
	}

	if haveSomethingToCommit {
		go rep.Commit()
	}
}

func (rep *Replica) HeartbeatReplicas() {
	rep.dataMu.Lock()
	log.Printf("Replica %s heartbeat other replicas in round %d", rep.id, rep.round)
	rep.dataMu.Unlock()

	for _, repID := range rep.replicasIDs {
		go func(repID string) {
			log.Printf("Replica %s trying to heartbeat replica %s", rep.id, repID)
			rep.dataMu.Lock()
			currentRound, currentID := rep.round, rep.id
			nextLogIndex := rep.nextIndex[repID]
			prevLogIndex := nextLogIndex - 1
			prevLogRound := uint64(0)
			var appendLogs []Log
			if prevLogIndex >= 0 {
				prevLogRound = rep.logs[prevLogIndex].Round
			}
			appendLogs = rep.logs[prevLogIndex+1:]

			req := AERequest{
				Round:           currentRound,
				RepID:           currentID,
				PrevLogIndex:    prevLogIndex,
				PrevLogRound:    prevLogRound,
				AppendLogs:      appendLogs,
				LastCommitIndex: rep.lastCommitIndex,
			}
			rep.dataMu.Unlock()

			resp, err := rep.commServ.AppendEntries(repID, req)
			if err != nil {
				log.Printf("[Replica %s] Failed to heartbeat append logs %s because of rps call", rep.id, repID)
				return
			}

			rep.dataMu.Lock()
			if resp.Round > rep.round {
				log.Printf("Replica %s is not leader anymore because there is greater Round: %d",
					rep.id, rep.round)
				resp.Round = rep.round
				rep.dataMu.Unlock()
				rep.BecomeListener()
				rep.dataMu.Lock()
			}
			defer rep.dataMu.Unlock()

			if rep.state == Leader {
				if resp.Success {
					log.Printf("[Replica %s] Successfully appended logs entries to replica %s", rep.id, repID)
					rep.nextIndex[repID] = nextLogIndex + len(appendLogs)
					rep.matchIndex[repID] = rep.nextIndex[repID] - 1
					go rep.TryCommit()
				} else {
					log.Printf("[Replica %s] Failed to append logs to replica %s as it rejected our request",
						rep.id, repID)
					if nextLogIndex > 0 {
						rep.nextIndex[repID] = nextLogIndex - 1
					}
				}
			} else {
				log.Printf("[Replica %s] Cannot process response from replica %s as are not leader anymore",
					rep.id, repID)
			}

		}(repID)
	}

	go rep.TryCommit()

	return
}

func (rep *Replica) BecomeLeader() {
	rep.dataMu.Lock()
	rep.state = Leader
	log.Printf("Replica %s is leader in Round %d", rep.id, rep.round)
	rep.dataMu.Unlock()

	for {
		rep.HeartbeatReplicas()
		<-time.NewTimer(50 * time.Millisecond).C

		rep.dataMu.Lock()
		if rep.state != Leader {
			log.Printf("Replica %s is not leader anymore", rep.id)
			rep.dataMu.Unlock()
			return
		}

		rep.dataMu.Unlock()
	}
}
