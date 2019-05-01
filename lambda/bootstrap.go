package main

import (
	"github.com/smithclay/rlinklayer/lambda/overlay"
	"github.com/smithclay/rlinklayer/lambda/runtime"
	"github.com/smithclay/rlinklayer/lambda/utils"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

func execProcess() {
	handler := os.Getenv("RUN_CMD")
	args, err := utils.ParseCommandLine(handler)
	if err != nil {
		log.Printf("Error: could parse run cmd: '%v'", handler)
		os.Exit(1)
	}
	if len(args) < 1 {
		log.Printf("Error: could not get run cmd '%v'", handler)
		os.Exit(1)
	}
	log.Printf("args: %v", args)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	err = cmd.Run()
	if err != nil {
		log.Printf("Error: could not run handler: %v", err)
	}
	log.Printf("Handler exited prematurely.")
	os.Exit(2)
}

func processEvents(rc *runtime.Client) {
	for {
		requestId, deadline, err := rc.GetInvocation()
		if err != nil {
			log.Printf("Error: could not get next invocation: %v", err)
			err = rc.PostInitializationError(err.Error())
			if err != nil {
				os.Exit(1)
			}
		}
		// TODO: configure this
		log.Printf("Function deadline in: %v", deadline)

		// Keep accepting connections until 5 seconds before the funtion times out.
		remainingTime := time.Duration(deadline-time.Now().Unix()*1000) * time.Millisecond
		time.Sleep(remainingTime - (5 * time.Second))

		err = rc.PostResponse(requestId, "SUCCESS")
	}
}

func startNetwork() {
	netName := os.Getenv("OL_NET_NAME")
	macAddress := os.Getenv("OL_MAC_ADDR")
	ipAddr := os.Getenv("OL_IP_ADDR")
	opts := overlay.Options{MacAddress: macAddress,
		IP:          ipAddr,
		NetworkName: netName,
		OverlayType: overlay.CloudwatchLog,
	}
	no := overlay.New(opts)
	no.Start()
}

func main() {
	log.SetPrefix("[bootstrap] ")

	runtimeClient := runtime.New(&http.Client{})
	go execProcess()
	startNetwork()
	processEvents(runtimeClient)
}
