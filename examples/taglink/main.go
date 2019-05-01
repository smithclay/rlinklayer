package main

import (
	"fmt"
	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/tcpip/network/arp"
	"github.com/google/netstack/tcpip/network/ipv4"
	"github.com/google/netstack/tcpip/stack"
	"github.com/google/netstack/tcpip/transport/tcp"
	"github.com/smithclay/rlinklayer/link/aws/tag"
	"github.com/smithclay/rlinklayer/utils"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	s := stack.New([]string{ipv4.ProtocolName, arp.ProtocolName}, []string{tcp.ProtocolName}, stack.Options{})
	opts := &tag.Options{
		RemoteArn:     "arn:aws:lambda:us-west-2:275197385476:function:helloWorldTestFunction",
		LocalArn:      "arn:aws:lambda:us-west-2:275197385476:function:helloWorldTestFunction",
		RemoteAddress: utils.GenerateRandomMac(),
		LocalAddress:  utils.GenerateRandomMac(),
	}
	linkID := tag.New(opts)
	if err := s.CreateNIC(1, linkID); err != nil {
		log.Fatalf("Could not create NIC card")
	}
	addr := utils.IpToAddress(net.ParseIP("192.168.1.1"))
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
