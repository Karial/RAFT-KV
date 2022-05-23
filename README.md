# RAFT-KV

Simple distributed kv-store using RAFT algorithm.

### Client API

- Get\tkey
    - Get value by key.
- Set\tkey\tvalue
    - Put key with value. 
- Del\tkey
    - Delete value by key.
- CAS\tOldValue\tNewValue
    - Cas value by key. If it is equal to old value, then change to new value.

### Start replicas

```
./run.sh --server --port 3000 --replicasAddrs localhost:3001,localhost:3002,localhost:3003 --replicasIDs 1,2,3 --guid 1 &
./run.sh --server --port 3000 --replicasAddrs localhost:3001,localhost:3002,localhost:3003 --replicasIDs 1,2,3 --guid 1 &
./run.sh --server --port 3000 --replicasAddrs localhost:3001,localhost:3002,localhost:3003 --replicasIDs 1,2,3 --guid 1 &
```

### Start client

```
./run.sh --client --replicasAddrs localhost:3001,localhost:3002,localhost:3003
```


### Usage

```
Set	k	v
Response Result: Ok
Get	k
Response Result: v
CAS	k	v	v2
Response Result: true
Del	k
Response Result: Ok
```

### ./run.sh Usage

```
Usage: ./run.sh <client, server>
  -client
        Start client
  -guid string
        Global unique identifier for coordinator. Should be unique between all coordinators.
  -port int
        Port for the server to listen on (default 3000)
  -replicasAddrs string
        Replicas addresses, comma separated
  -replicasIDs string
        Replicas ids, comma separated
  -server
        Start server
  -v    Verbose output
```
