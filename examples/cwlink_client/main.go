package main

import (
	"fmt"
	"github.com/google/netstack/tcpip/network/arp"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/google/netstack/waiter"

	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/tcpip/adapters/gonet"
	"github.com/google/netstack/tcpip/link/sniffer"
	"github.com/google/netstack/tcpip/network/ipv4"
	"github.com/google/netstack/tcpip/stack"
	"github.com/google/netstack/tcpip/transport/tcp"
	linkaws "github.com/smithclay/rlinklayer/link/aws/cloudwatch"
	"github.com/smithclay/rlinklayer/utils"
)

func listenAndServe(s *stack.Stack, addr tcpip.Address, port int, mux *http.ServeMux) {
	fullAddr := tcpip.FullAddress{
		NIC:  1,
		Addr: "",
		Port: uint16(port),
	}

	ln, _ := gonet.NewListener(s, fullAddr, ipv4.ProtocolNumber)

	http.Serve(ln, mux)
}

func main() {
	s := stack.New([]string{ipv4.ProtocolName, arp.ProtocolName}, []string{tcp.ProtocolName}, stack.Options{})

	opts := &linkaws.Options{
		NetworkName:    "TestNet",
		EthernetHeader: true,
		Address:        "\x42\x42\x42\x42\x42\x42",
	}
	cwLink, _ := linkaws.New(opts)

	sniffed := sniffer.New(cwLink)
	if err := s.CreateNIC(1, sniffed); err != nil {
		log.Fatalf("Could not create NIC card")
	}

	addr := utils.IpToAddress(net.ParseIP("192.168.1.21"))
	if err := s.AddAddress(1, ipv4.ProtocolNumber, addr); err != nil {
		log.Fatalf("error %s", err)
	}

	if err := s.AddAddress(1, arp.ProtocolNumber, arp.ProtocolAddress); err != nil {
		log.Fatalf("error: %s", err)
	}

	s.SetRouteTable([]tcpip.Route{
		{
			Destination: tcpip.Address(strings.Repeat("\x00", 4)),
			Mask:        tcpip.AddressMask(strings.Repeat("\x00", 4)),
			Gateway:     "",
			NIC:         1,
		},
	})
	//s.SetForwarding(true)

	/*mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello from netstack over tags!\n"))
	})
	go listenAndServe(s, addr, 8080, mux)*/

	var wq waiter.Queue
	fwd := tcp.NewForwarder(s, 0, 10, func(r *tcp.ForwarderRequest) {
		ep, er := r.CreateEndpoint(&wq)
		if er != nil {
			transportEndpointID := r.ID()
			log.Println(er, net.JoinHostPort(transportEndpointID.LocalAddress.String(), strconv.Itoa(int(transportEndpointID.LocalPort))))
			r.Complete(false)
			return
		}
		defer ep.Close()
		transportEndpointID := r.ID()
		r.Complete(false)
		log.Printf("NewForwarder Remote: %v:%v", transportEndpointID.RemoteAddress, transportEndpointID.RemotePort)
		conn, err := net.Dial("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(int(transportEndpointID.LocalPort))))
		if err != nil {
			log.Println(err)
			return
		}
		defer conn.Close()
		fwdConn := gonet.NewConn(&wq, ep)
		go io.Copy(fwdConn, conn)
		io.Copy(conn, fwdConn)
	})
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, fwd.HandlePacket)

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
	fmt.Println("Exiting...")
}
