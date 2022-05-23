package main

import (
	"Trying_to_implement_atomic_KV/src"
	"flag"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

func main() {
	http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = 100
	rand.Seed(time.Now().Unix())

	port := flag.Int("port", 3000, "Port for the server to listen on")
	//db := flag.String("db", "", "Path to leveldb")
	isServer := flag.Bool("server", false, "Start server")
	isClient := flag.Bool("client", false, "Start client")
	replicasIDsStr := flag.String("replicasIDs", "", "Replicas ids, comma separated")
	replicasAddressesStr := flag.String("replicasAddrs", "", "Replicas addresses, comma separated")
	guid := flag.String("guid", "", "Global unique identifier for coordinator. "+
		"Should be unique between all coordinators.")
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()

	if !*verbose {
		log.SetOutput(ioutil.Discard)
	} else {
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	}

	if !*isServer && !*isClient {
		panic("You should choose whether replica or coordinator")
	}

	if *isServer {
		server := src.NewCommunicationService(*guid, strings.Split(*replicasIDsStr, ","),
			strings.Split(*replicasAddressesStr, ","))
		server.Start(*port)
	}

	if *isClient {
		client := src.NewClient(strings.Split(*replicasAddressesStr, ","))
		client.Start()
	}
}
