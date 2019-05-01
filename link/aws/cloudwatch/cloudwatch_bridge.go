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

type endpointBridge struct {
	dispatcher stack.NetworkDispatcher
	laddr      tcpip.LinkAddress
	raddr      tcpip.LinkAddress
	netName    string
	logLink    *LogLink
	lower      stack.LinkEndpoint // Optional wrapping of another link (tun/tap)
	hdrSize    int
	p2p        bool
}

// New creates a new endpoint for transmitting data using Amazon Cloudwathc gorups
func NewBridge(opts *Options) (tcpip.LinkEndpointID, *endpointBridge) {
	sess, _ := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2")},
	)
	svc := cloudwatchlogs.New(sess)

	ep := &endpointBridge{
		laddr:   opts.Address,
		raddr:   opts.RemoteAddress,
		netName: opts.NetworkName,
		p2p:     opts.PointToPoint,
	}

	if opts.LinkEndpoint != 0 {
		ep.lower = stack.FindLinkEndpoint(opts.LinkEndpoint)
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
func (e *endpointBridge) Attach(dispatcher stack.NetworkDispatcher) {
	e.dispatcher = dispatcher
	e.logLink.Start()
	e.lower.Attach(e)
	go e.dispatchLoop()
}

// DeliverNetworkPacket implements the stack.NetworkDispatcher interface. It is
// called by the link-layer endpoint being wrapped when a packet arrives
func (e *endpointBridge) DeliverNetworkPacket(rxEP stack.LinkEndpoint, srcLinkAddr, dstLinkAddr tcpip.LinkAddress, p tcpip.NetworkProtocolNumber, vv buffer.VectorisedView) {
	log.Printf("DeliverNetworkPacket: %v -> %v", srcLinkAddr, dstLinkAddr)

	//broadcast := false

	switch dstLinkAddr {
	case broadcastMAC:
		//broadcast = true
	case e.laddr:
		e.dispatcher.DeliverNetworkPacket(e, srcLinkAddr, dstLinkAddr, p, vv)
		return
	}

	route := &stack.Route{
		NetProto:          p,
		LocalLinkAddress:  srcLinkAddr,
		RemoteLinkAddress: dstLinkAddr,
	}

	payload := vv
	hdr := buffer.NewPrependable(int(e.MaxHeaderLength()) + len(payload.First()))
	copy(hdr.Prepend(len(payload.First())), payload.ToView())
	payload.RemoveFirst()

	// Don't write back out interface from which the frame arrived
	// because that causes interoperability issues with a router.
	if rxEP.LinkAddress() != dstLinkAddr {
		e.WritePacket(route, nil, hdr, payload, p)
	}
}

// IsAttached implements stack.LinkEndpoint.IsAttached.
func (e *endpointBridge) IsAttached() bool {
	return e.dispatcher != nil
}

// MTU implements stack.LinkEndpoint.MTU. It just forwards the request to the
// lower endpoint.
func (e *endpointBridge) MTU() uint32 {
	return e.lower.MTU()
}

// Capabilities implements stack.LinkEndpoint.Capabilities. It just forwards the
// request to the lower endpoint.
func (e *endpointBridge) Capabilities() stack.LinkEndpointCapabilities {
	return e.lower.Capabilities()
}

// MaxHeaderLength implements the stack.LinkEndpoint interface. It just forwards
// the request to the lower endpoint.
func (e *endpointBridge) MaxHeaderLength() uint16 {
	return e.lower.MaxHeaderLength()
}

func (e *endpointBridge) LinkAddress() tcpip.LinkAddress {
	return e.lower.LinkAddress()
}

func (e *endpointBridge) WritePacket(r *stack.Route, _ *stack.GSO, hdr buffer.Prependable, payload buffer.VectorisedView, protocol tcpip.NetworkProtocolNumber) *tcpip.Error {
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
	//log.Printf("WritePacket: %v -> %v (%v)", cwLinkAddr.laddr, cwLinkAddr.raddr, protocol)

	// Write outbound packet
	_, err = e.logLink.Write(cwLinkAddr, protocol, hdr.View(), vv.ToView())
	if err != nil {
		log.Printf("WritePacket: Error writing to link buffer, dropping packet: %v", err)
		return nil
	}
	return nil
}

func (e *endpointBridge) ReadPacket() {
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

	//log.Printf("ReadPacket: %v -> %v (%v)", remote, local, p)
	// Remove ethernet header, if exists
	vv.TrimFront(e.hdrSize)

	route := &stack.Route{
		NetProto:          p,
		LocalLinkAddress:  remote, // FIXME: Is this right?
		RemoteLinkAddress: local,
	}

	// Create header with padding for a link-layer header
	transportHeader := vv.Views()[0]
	h := buffer.NewPrependable(int(e.MaxHeaderLength()) + len(transportHeader))
	copy(h.Prepend(len(transportHeader)), transportHeader)
	// Remove header from vectorized view, leaving only the payload
	vv.RemoveFirst()
	e.lower.WritePacket(route, nil, h, *vv, p)

}

func (e *endpointBridge) dispatchLoop() {
	for {
		e.ReadPacket()
	}
}
