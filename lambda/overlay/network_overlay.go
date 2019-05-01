package overlay

import (
	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/tcpip/adapters/gonet"
	"github.com/google/netstack/tcpip/link/sniffer"
	"github.com/google/netstack/tcpip/network/arp"
	"github.com/google/netstack/tcpip/network/ipv4"
	"github.com/google/netstack/tcpip/stack"
	"github.com/google/netstack/tcpip/transport/tcp"
	"github.com/google/netstack/waiter"
	cwLink "github.com/smithclay/rlinklayer/link/aws/cloudwatch"
	tagLink "github.com/smithclay/rlinklayer/link/aws/tag"
	"github.com/smithclay/rlinklayer/utils"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
)

type NetworkType int

const (
	CloudwatchLog NetworkType = 1
	LambdaTag     NetworkType = 2
)

type NetworkOverlay struct {
	netName   string
	stack     *stack.Stack
	mac       tcpip.LinkAddress
	remoteMac tcpip.LinkAddress
	netType   NetworkType
	ip        string
	// Lambda Tag Specific
	localArn  string
	remoteArn string
}

type Options struct {
	IP               string
	OverlayType      NetworkType
	NetworkName      string
	MacAddress       string
	RemoteMacAddress string
	// Lambda Tag specific
	LocalArn  string
	RemoteArn string
}

func New(opts Options) *NetworkOverlay {
	return &NetworkOverlay{netName: opts.NetworkName,
		mac:     tcpip.LinkAddress(opts.MacAddress),
		ip:      opts.IP,
		netType: opts.OverlayType}
}

func (no *NetworkOverlay) Start() {
	no.stack = stack.New([]string{ipv4.ProtocolName, arp.ProtocolName}, []string{tcp.ProtocolName}, stack.Options{})

	var endpointID tcpip.LinkEndpointID

	if no.netType == LambdaTag {
		opts := &tagLink.Options{no.localArn,
			no.remoteArn,
			tcpip.LinkAddress(no.mac),
			tcpip.LinkAddress(no.remoteMac),
		}
		endpointID = tagLink.New(opts)
	}

	if no.netType == CloudwatchLog {
		opts := &cwLink.Options{
			NetworkName:    no.netName,
			Address:        tcpip.LinkAddress(no.mac),
			EthernetHeader: true,
		}
		endpointID, _ = cwLink.New(opts)
	}

	sniffed := sniffer.New(endpointID)
	if err := no.stack.CreateNIC(1, sniffed); err != nil {
		log.Fatalf("Could not create NIC card")
	}
	addr := utils.IpToAddress(net.ParseIP(no.ip))

	if err := no.stack.AddAddress(1, ipv4.ProtocolNumber, addr); err != nil {
		log.Fatalf("AddAddress error [ipv4]: %s", err)
	}

	if err := no.stack.AddAddress(1, arp.ProtocolNumber, arp.ProtocolAddress); err != nil {
		log.Fatalf("AddAddress error [arp]: %s", err)
	}

	no.stack.SetRouteTable([]tcpip.Route{
		{
			Destination: tcpip.Address(strings.Repeat("\x00", 4)),
			Mask:        tcpip.AddressMask(strings.Repeat("\x00", 4)),
			Gateway:     "",
			NIC:         1,
		},
	})
	no.forwardTCP()
}

func (no *NetworkOverlay) forwardTCP() {
	var wq waiter.Queue
	fwd := tcp.NewForwarder(no.stack, 0, 10, func(r *tcp.ForwarderRequest) {
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
	no.stack.SetTransportProtocolHandler(tcp.ProtocolNumber, fwd.HandlePacket)
}
