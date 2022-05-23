package src

import (
	"log"
	"os/exec"
	"strconv"
	"testing"
)

const binaryPath = "../Trying_to_implement_atomic_KV"

var config = []string{
	"--server --port 3001 --replicasIDs 1,2,3 --replicasAddrs localhost:3001,localhost:3002,localhost:3003 --guid 1 --v",
	"--server --port 3002 --replicasIDs 1,2,3 --replicasAddrs localhost:3001,localhost:3002,localhost:3003 --guid 2 --v",
	"--server --port 3003 --replicasIDs 1,2,3 --replicasAddrs localhost:3001,localhost:3002,localhost:3003 --guid 3 --v",
}

func StartReplicas() chan struct{} {
	done := make(chan struct{})
	go func() {
		for _, commandArgs := range config {
			cmd := exec.Command(binaryPath, commandArgs)
			if err := cmd.Start(); err != nil {
				log.Fatal(err)
			}

			<-done

			// Kill it:
			if err := cmd.Process.Kill(); err != nil {
				log.Fatal("failed to kill process: ", err)
			}
		}
	}()

	return done
}

func BenchmarkSystem(b *testing.B) {
	done := StartReplicas()
	client := NewClient([]string{"localhost:3001", "localhost:3002", "localhost:3003"})
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		client.SetRequest(strconv.Itoa(i), "v")
	}

	close(done)
}
