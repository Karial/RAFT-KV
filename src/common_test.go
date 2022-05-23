package src

import (
	"github.com/stretchr/testify/assert"
	"sort"
	"testing"
	"time"
)

func TestWork(t *testing.T) {
	callsDone := CreateCallResults(3)

	go func() {
		callsDone[0] <- CallResult{true, 1}
	}()

	go func() {
		callsDone[1] <- CallResult{true, 2}
	}()

	go func() {
		callsDone[2] <- CallResult{true, 3}
	}()

	results, err := Await(callsDone, 2)
	assert.NoError(t, err)

	resultInt := ConvertInterfacesToType[int](results)

	sort.Slice(resultInt, func(i, j int) bool {
		return resultInt[i] < resultInt[j]
	})

	valuesSet := make(map[int]interface{})
	valuesSet[1] = struct{}{}
	valuesSet[2] = struct{}{}
	valuesSet[3] = struct{}{}

	assert.Equal(t, len(resultInt), 2)
	assert.NotEqual(t, resultInt[0], resultInt[1])

	_, ok := valuesSet[resultInt[0]]
	assert.True(t, ok)

	_, ok = valuesSet[resultInt[1]]
	assert.True(t, ok)
}

func TestNotWaitForSlowpoke(t *testing.T) {
	callsDone := CreateCallResults(3)

	go func() {
		callsDone[0] <- CallResult{true, 1}
	}()

	go func() {
		callsDone[1] <- CallResult{true, 2}
	}()

	go func() {
		time.Sleep(1 * time.Second)
		callsDone[2] <- CallResult{true, 3}
	}()

	results, err := Await(callsDone, 2)
	assert.NoError(t, err)

	resultInt := ConvertInterfacesToType[int](results)

	sort.Slice(resultInt, func(i, j int) bool {
		return resultInt[i] < resultInt[j]
	})

	assert.Equal(t, len(resultInt), 2)
	assert.Equal(t, resultInt, []int{1, 2})
}

func TestError(t *testing.T) {
	callsDone := CreateCallResults(3)

	go func() {
		callsDone[0] <- CallResult{false, 1}
	}()

	go func() {
		callsDone[1] <- CallResult{false, 2}
	}()

	go func() {
		callsDone[2] <- CallResult{true, 3}
	}()

	results, err := Await(callsDone, 2)
	assert.Error(t, err)

	resultInt := ConvertInterfacesToType[int](results)

	sort.Slice(resultInt, func(i, j int) bool {
		return resultInt[i] < resultInt[j]
	})

	assert.Equal(t, len(resultInt), 1)
	assert.Equal(t, resultInt, []int{3})
}
