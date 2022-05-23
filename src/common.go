package src

import (
	"context"
	"io"
	"math/rand"
	"sync"
	"time"
)

type Key string
type Value string

func ChooseRandomTimeout(from, to time.Duration) time.Duration {
	return from + time.Duration(rand.Int63n((to-from).Milliseconds()))*time.Millisecond
}

func WaitUntil(t time.Time) <-chan time.Time {
	duration := t.Sub(time.Now())
	if duration < 0 {
		duration = 0
	}

	return time.NewTimer(duration).C
}

func WriteAll(dst io.Writer, src io.Reader) error {
	written := 0
	buf := make([]byte, 1024)
	for {
		readed, err := src.Read(buf)
		written += readed
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		needToWrite := written + readed
		for written < needToWrite {
			currentWritten, err := dst.Write(buf[:readed])
			written += currentWritten
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func ConvertInterfacesToType[T any](a []interface{}) []T {
	result := make([]T, len(a))
	for i, x := range a {
		result[i] = x.(T)
	}

	return result
}

type CallResult struct {
	success bool
	result  interface{}
}

func CreateCallResults(n int) []chan CallResult {
	callsDone := make([]chan CallResult, n)
	for i := range callsDone {
		callsDone[i] = make(chan CallResult)
	}

	return callsDone
}

type NoHalfSuccessfulCallResults struct {
}

var (
	_ error = &NoHalfSuccessfulCallResults{}
)

func (err *NoHalfSuccessfulCallResults) Error() string {
	return "No enough successful responses"
}

func Await(calls []chan CallResult, num int) ([]interface{}, error) {
	mu := sync.Mutex{}
	done := false
	callResults := make([]interface{}, 0)
	majorityReturned := make(chan struct{})

	allFinished := make(chan struct{})
	wg := sync.WaitGroup{}

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
	}()

	for _, call := range calls {
		call := call
		wg.Add(1)
		go func() {
			select {
			case callResult := <-call:
				if callResult.success {
					mu.Lock()
					if !done {
						callResults = append(callResults, callResult.result)
						if len(callResults) >= num {
							done = true
							close(majorityReturned)
						}
					}
					mu.Unlock()
				}
			case <-ctx.Done():
			}
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		allFinished <- struct{}{}
	}()

	select {
	case <-majorityReturned:
		return callResults, nil
	case <-allFinished:
		if len(callResults) >= num {
			return callResults, nil
		}

		return callResults, &NoHalfSuccessfulCallResults{}
	}
}
