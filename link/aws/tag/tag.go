package tag

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/tcpip/buffer"
	"github.com/google/netstack/tcpip/header"
	"github.com/google/netstack/tcpip/network/ipv4"
	"github.com/google/netstack/tcpip/stack"
	"io"
	"log"
)

type endpoint struct {
	dispatcher stack.NetworkDispatcher
	tagLink    *TagLink
	stats      *AwsStats
	laddr      tcpip.LinkAddress
	raddr      tcpip.LinkAddress
}

// Options specify the details about the AWS service-based endpoint to be created.
type Options struct {
	LocalArn      string
	RemoteArn     string
	LocalAddress  tcpip.LinkAddress
	RemoteAddress tcpip.LinkAddress
}

// AwsStats collects link-specific stats.
type AwsStats struct {
	RxPackets uint32
	TxPackets uint32
	TxErrors  uint32
	RxErrors  uint32
}

// New creates a new endpoint for transmitting data using AWS Lambda tags.
func New(opts *Options) tcpip.LinkEndpointID {
	ep := &endpoint{
		laddr: opts.LocalAddress,
		raddr: opts.RemoteAddress,
		stats: &AwsStats{},
	}
	ep.tagLink = newTagLink(opts.LocalArn, opts.RemoteArn, ep)
	log.Printf("New AWS Link: local %s, remote %s", ep.laddr, ep.raddr)
	return stack.RegisterLinkEndpoint(ep)
}

func newTagLink(localArn string, remoteArn string, e *endpoint) *TagLink {
	sess, _ := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2")},
	)
	svc := lambda.New(sess, &aws.Config{Region: aws.String("us-west-2")})
	config := TagConfig{
		LambdaService: svc,
		Endpoint:      e,
		TxArn:         remoteArn,
		RxArn:         localArn,
	}
	return NewTagLink(&config)
}

// Attach implements stack.LinkEndpoint.Attach. It just saves the stack network-
// layer dispatcher for later use when packets need to be dispatched.
func (e *endpoint) Attach(dispatcher stack.NetworkDispatcher) {
	e.dispatcher = dispatcher
	e.tagLink.StartPolling()
	go e.dispatchLoop()
}

func (e *endpoint) dispatchLoop() {
	for {
		decoded, err := e.readSinglePacket(BufConfig[0])
		if err != nil {
			log.Printf("dispatchLoop: Error reading single packet: %v", err)
			e.stats.RxErrors++
			continue
		}
		if len(decoded) > 0 {
			e.stats.RxPackets++
			e.dispatchSinglePacket(decoded)
		}
	}
}

func (e *endpoint) readSinglePacket(size int) ([]byte, error) {
	buf := make([]byte, size)
	n, err := e.tagLink.Read(buf)
	if err != nil && err != io.EOF {
		return nil, err
	}

	return buf[:n], nil
}

func (e *endpoint) dispatchSinglePacket(decoded []byte) bool {

	//log.Printf("dispatchLoop: read buffer %v", decoded)
	ipv4Packet := header.IPv4(decoded)
	if ipv4Packet.IsValid(len(decoded)) {
		//log.Printf("dispatchLoop: valid ipv4 packet received, src: %s, dest: %s", ipv4Packet.SourceAddress(), ipv4Packet.DestinationAddress())
		vv := buffer.NewViewFromBytes(decoded).ToVectorisedView()
		e.dispatcher.DeliverNetworkPacket(e, "", "", ipv4.ProtocolNumber, vv)
		return true
	}
	log.Printf("dispatchLoop: Invalid ipv4 packet recvd: %v", decoded)
	return false
}

// IsAttached implements stack.LinkEndpoint.IsAttached.
func (e *endpoint) IsAttached() bool {
	return e.dispatcher != nil
}

// MTU implements stack.LinkEndpoint.MTU.
// Maximum tag length
func (*endpoint) MTU() uint32 {
	return 189
}

// Capabilities implements stack.LinkEndpoint.Capabilities. Loopback advertises
// itself as supporting checksum offload, but in reality it's just omitted.
func (*endpoint) Capabilities() stack.LinkEndpointCapabilities {
	return stack.LinkEndpointCapabilities(0)
	//return stack.CapabilityChecksumOffload | stack.CapabilitySaveRestore | stack.CapabilityLoopback
}

// MaxHeaderLength implements stack.LinkEndpoint.MaxHeaderLength. Given that the
// loopback interface doesn't have a header, it just returns 0.
func (*endpoint) MaxHeaderLength() uint16 {
	return 0
}

// LinkAddress returns the link address of this endpoint.
func (e *endpoint) LinkAddress() tcpip.LinkAddress {
	return e.laddr
}

// WritePacket implements stack.LinkEndpoint.WritePacket. It delivers outbound
// packets to the network-layer dispatcher.
func (e *endpoint) WritePacket(s *stack.Route, _ *stack.GSO, hdr buffer.Prependable, payload buffer.VectorisedView, protocol tcpip.NetworkProtocolNumber) *tcpip.Error {
	views := make([]buffer.View, 1, 1+len(payload.Views()))
	views[0] = hdr.View()
	views = append(views, payload.Views()...)
	vv := buffer.NewVectorisedView(len(views[0])+payload.Size(), views)

	_, err := e.tagLink.Write(vv.ToView())
	if err != nil {
		log.Printf("WritePacket: Error writing to link buffer, dropping packet: %v", err)
		log.Printf("WritePacket: Available Write Buffers: %d", e.tagLink.txBuffer.avail)
		e.stats.TxErrors++
		return nil
	}
	e.stats.TxPackets++
	return nil
}
