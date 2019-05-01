// Copyright 2019 Clay Smith

// +build linux

package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/tcpip/link/sniffer"
	"github.com/google/netstack/tcpip/network/arp"
	"github.com/google/netstack/tcpip/network/ipv4"
	"github.com/google/netstack/tcpip/stack"
	"github.com/google/netstack/tcpip/transport/tcp"
	linkaws "github.com/smithclay/rlinklayer/link/aws/cloudwatch"
	"github.com/smithclay/rlinklayer/utils"
)

var tap = flag.Bool("tap", false, "use tap instead of tun")
var mac = flag.String("mac", "\x74\x74\x74\x74\x74\x74", "mac address to use in tap device")

func main() {
	flag.Parse()

	_, err := newStack()
	if err != nil {
		log.Fatalf("newStack: could not create: %v", err)
	}

	fmt.Println("Press CTRL-C to exit.")
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		fmt.Println()
		fmt.Println(sig)
		done <- true
	}()
	<-done
	fmt.Println("exiting")
}

func newStack() (*stack.Stack, *tcpip.Error) {
	s := stack.New([]string{ipv4.ProtocolName, arp.ProtocolName}, []string{tcp.ProtocolName}, stack.Options{})

	// AWS network stack
	var linkID tcpip.LinkEndpointID
	ethernetEnabled := false
	pointToPoint := true
	var remoteAddress tcpip.LinkAddress
	localLink := tcpip.LinkAddress(*mac)

	if *tap {
		log.Printf("newStack: Creating tap link endpoint ...")
		linkID = utils.NewTapLink("tap0", tcpip.LinkAddress(*mac))
		ethernetEnabled = true
		pointToPoint = false
	} else {
		log.Printf("newStack: Creating tun link endpoint...")
		linkID = utils.NewTunLink("tun0")
		remoteAddress = tcpip.LinkAddress("\x11\x22\x33\x44\x55\x66")
	}

	sniffedTunTap := sniffer.New(linkID)

	log.Printf("newStack: Creating New Cloudwatch Endpoint: Local: %v, Remote: %v", localLink.String(), remoteAddress)
	opts := &linkaws.Options{
		NetworkName:    "TestNet",
		EthernetHeader: ethernetEnabled,
		PointToPoint:   pointToPoint,
		Address:        localLink,
		LinkEndpoint:   sniffedTunTap,
		RemoteAddress:  remoteAddress,
	}
	awsLinkID, _ := linkaws.NewBridge(opts)

	if err := s.CreateNIC(1, awsLinkID); err != nil {
		log.Fatalf("Could not create NIC card")
	}
	addr := utils.IpToAddress(net.ParseIP("192.168.1.3"))
	if err := s.AddAddress(1, ipv4.ProtocolNumber, addr); err != nil {
		log.Fatalf("error %s", err)
	}
	if err := s.AddAddress(1, arp.ProtocolNumber, arp.ProtocolAddress); err != nil {
		log.Fatalf("newStack: Could enable ARP: %s", err)
	}

	s.SetRouteTable([]tcpip.Route{{
		Destination: "\x00\x00\x00\x00",
		Mask:        "\x00\x00\x00\x00",
		Gateway:     "",
		NIC:         1,
	}})

	return s, nil
}
