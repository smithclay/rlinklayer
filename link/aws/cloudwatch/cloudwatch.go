package cloudwatch

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/tcpip/buffer"
	"github.com/google/netstack/tcpip/header"
	"github.com/google/netstack/tcpip/stack"
	"log"
)

// todo: configure this
const MTU = 1024

type endpoint struct {
	dispatcher stack.NetworkDispatcher
	laddr      tcpip.LinkAddress
	raddr      tcpip.LinkAddress
	netName    string
	logLink    *LogLink
	hdrSize    int
	p2p        bool
}

type Options struct {
	Address        tcpip.LinkAddress
	RemoteAddress  tcpip.LinkAddress // for point-to-point configuration
	PointToPoint   bool
	EthernetHeader bool
	NetworkName    string
	LinkEndpoint   tcpip.LinkEndpointID
}

// New creates a new endpoint for transmitting data using Amazon Cloudwathc gorups
func New(opts *Options) (tcpip.LinkEndpointID, *endpoint) {
	sess, _ := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2")},
	)
	svc := cloudwatchlogs.New(sess)

	ep := &endpoint{
		laddr:   opts.Address,
		raddr:   opts.RemoteAddress,
		netName: opts.NetworkName,
		p2p:     opts.PointToPoint,
	}

	if opts.PointToPoint && opts.RemoteAddress == "" {
		log.Fatalf("New: Cannot create point-to-point endpoint without a remote link address.")
	}

	ep.hdrSize = 0
	if opts.EthernetHeader {
		ep.hdrSize = header.EthernetMinimumSize
	}

	ep.logLink = NewLogLink(&LogConfig{LogService: svc, Endpoint: ep, NetName: opts.NetworkName})

	return stack.RegisterLinkEndpoint(ep), ep
}

// Attach implements the stack.LinkEndpoint interface. It saves the dispatcher
// and registers with the lower endpoint as its dispatcher so that "e" is called
// for inbound packets.
func (e *endpoint) Attach(dispatcher stack.NetworkDispatcher) {
	e.dispatcher = dispatcher
	e.logLink.Start()

	go e.dispatchLoop()
}

// IsAttached implements stack.LinkEndpoint.IsAttached.
func (e *endpoint) IsAttached() bool {
	return e.dispatcher != nil
}

// MTU implements stack.LinkEndpoint.MTU. It just forwards the request to the
// lower endpoint.
func (e *endpoint) MTU() uint32 {
	return MTU
}

// Capabilities implements stack.LinkEndpoint.Capabilities. It just forwards the
// request to the lower endpoint.
func (e *endpoint) Capabilities() stack.LinkEndpointCapabilities {

	return stack.LinkEndpointCapabilities(0)
}

// MaxHeaderLength implements the stack.LinkEndpoint interface. It just forwards
// the request to the lower endpoint.
func (e *endpoint) MaxHeaderLength() uint16 {

	return uint16(e.hdrSize)
}

func (e *endpoint) LinkAddress() tcpip.LinkAddress {

	return e.laddr
}

func (e *endpoint) WritePacket(r *stack.Route, _ *stack.GSO, hdr buffer.Prependable, payload buffer.VectorisedView, protocol tcpip.NetworkProtocolNumber) *tcpip.Error {
	if r.RemoteLinkAddress == "" {
		log.Fatalf("WritePacket: no remote link address found for: '%v'", r.LocalLinkAddress)
	}

	if e.hdrSize > 0 {
		// Add ethernet header if needed.
		eth := header.Ethernet(hdr.Prepend(header.EthernetMinimumSize))
		ethHdr := &header.EthernetFields{
			DstAddr: r.RemoteLinkAddress,
			Type:    protocol,
		}

		// Preserve the src address if it's set in the route.
		if r.LocalLinkAddress != "" {
			ethHdr.SrcAddr = r.LocalLinkAddress
		} else {
			ethHdr.SrcAddr = e.LinkAddress()
		}
		eth.Encode(ethHdr)
	}

	views := make([]buffer.View, 0, len(payload.Views()))
	views = append(views, payload.Views()...)
	vv := buffer.NewVectorisedView(payload.Size(), views)

	// Fail if there is no remote address to write to
	cwLinkAddr := CloudwatchLinkAddress{r.LocalLinkAddress, r.RemoteLinkAddress, e.netName}

	// Open stream for writing (which creates if it doesn't exist)
	err := e.logLink.OpenLogStream(cwLinkAddr)
	if err != nil {
		log.Fatalf("WritePacket: Could not create remote log group: %v", err)
	}
	//log.Printf("WritePacket: %v -> %v (%v)", cwLinkAddr.raddr, cwLinkAddr.raddr, protocol)

	// Write outbound packet
	_, err = e.logLink.Write(cwLinkAddr, protocol, hdr.View(), vv.ToView())
	if err != nil {
		log.Printf("WritePacket: Error writing to link buffer, dropping packet: %v", err)
		return nil
	}
	return nil
}

func (e *endpoint) ReadPacket() {
	vv, err := e.logLink.Read()
	if err != nil {
		log.Printf("ReadPacket: error reading: %v", err)
		return
	}
	var (
		p             tcpip.NetworkProtocolNumber
		remote, local tcpip.LinkAddress
	)

	hdr := vv.Views()[0]

	if e.hdrSize > 0 {
		eth := header.Ethernet(hdr)
		p = eth.Type()
		remote = eth.SourceAddress()
		local = eth.DestinationAddress()
	} else {
		// We don't get any indication of what the packet is, so try to guess
		// if it's an IPv4 or IPv6 packet.
		switch header.IPVersion(hdr) {
		case header.IPv4Version:
			p = header.IPv4ProtocolNumber
		case header.IPv6Version:
			p = header.IPv6ProtocolNumber
		}
	}

	// Message coming from the sending link, ignore
	if remote != "" && remote == e.LinkAddress() {
		log.Printf("ReadPacket: ignoring frame, address is local")
		return
	}

	vv.TrimFront(e.hdrSize)

	e.dispatcher.DeliverNetworkPacket(e, remote, local, p, *vv)

}

func (e *endpoint) dispatchLoop() {
	for {
		e.ReadPacket()
	}
}
