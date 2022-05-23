package src

import (
	"encoding/json"
	"fmt"
	"log"
)

type KVRsm struct {
}

type CommandType uint8

const (
	Set CommandType = iota
	Get
	Del
	CAS
)

type CommandJson struct {
	Type CommandType `json:"CommandType"`
	Args string      `json:"Args"`
}

type SetArgs struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

type GetArgs struct {
	Key string `json:"Key"`
}

type DelArgs struct {
	Key string `json:"Key"`
}

type CASArgs struct {
	Key      string `json:"Key"`
	OldValue string `json:"OldValue"`
	NewValue string `json:"NewValue"`
}

type KVRSM struct {
	kv map[string]string
}

func NewKVRSM() *KVRSM {
	return &KVRSM{
		kv: make(map[string]string),
	}
}

func (rsm *KVRSM) Apply(command Command) (string, error) {
	serializedCommand := CommandJson{}
	err := json.Unmarshal([]byte(command), &serializedCommand)
	if err != nil {
		log.Printf("Unknown command: %s", command)
		return "", err
	}

	switch serializedCommand.Type {
	case Set:
		setArgs := SetArgs{}
		err = json.Unmarshal([]byte(serializedCommand.Args), &setArgs)
		if err != nil {
			log.Printf("Cannot serialize Set command args: %s", serializedCommand.Args)
			return "", err
		}

		rsm.kv[setArgs.Key] = setArgs.Value
		return "OK", nil
	case Get:
		getArgs := GetArgs{}
		err = json.Unmarshal([]byte(serializedCommand.Args), &getArgs)
		if err != nil {
			log.Printf("Cannot serialize Set command args: %s", serializedCommand.Args)
			return "", err
		}

		res, ok := rsm.kv[getArgs.Key]
		if ok {
			return res, nil
		}

		return "", fmt.Errorf("no such key")
	default:
		return "", fmt.Errorf("no such command")
	}
}
