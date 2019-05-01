package cloudwatch

import (
	"encoding/base64"
	"encoding/json"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs/cloudwatchlogsiface"
	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/tcpip/buffer"
	"github.com/google/netstack/tcpip/network/arp"
	"github.com/google/netstack/tcpip/network/ipv4"
	"github.com/google/netstack/tcpip/network/ipv6"
	"github.com/google/netstack/tcpip/stack"
	"log"
)

// PacketLog represents the log event emitted from Amazon Cloudwatch
type PacketLog struct {
	Type    string `json:"type"`
	Src     string `json:"src"`
	Dest    string `json:"dest"`
	Header  string `json:"header"`
	Payload string `json:"payload"`
}

// LogLink reads/writes L2 data to AWS service(s)
type LogLink struct {
	svc         cloudwatchlogsiface.CloudWatchLogsAPI
	ep          stack.LinkEndpoint
	netName     string
	readPoller  *ReadPoller
	writePoller *WritePoller
}

type LogConfig struct {
	LogService   cloudwatchlogsiface.CloudWatchLogsAPI
	Endpoint     stack.LinkEndpoint
	NetName      string
	LogGroupName string
}

// Log Group format `/network/link-address`
// Log Stream format `/network/link-address/tx-stream-local-link-address`

func NewLogLink(config *LogConfig) *LogLink {
	return &LogLink{svc: config.LogService, ep: config.Endpoint, netName: config.NetName, readPoller: NewReadPoller(config.LogService), writePoller: NewWritePoller(config.LogService)}
}

func (ll *LogLink) createLogGroup(groupName string) error {
	// Create log group, if it doesn't exist.
	_, err := ll.svc.CreateLogGroup(&cloudwatchlogs.CreateLogGroupInput{LogGroupName: aws.String(groupName)})
	if awsErr, ok := err.(awserr.Error); ok {
		// Ignore if resource already exists
		if awsErr.Code() != cloudwatchlogs.ErrCodeResourceAlreadyExistsException {
			return err
		}
	} else if err != nil {
		return err
	}

	return nil
}

var logstreamExistsCache = map[string]bool{}

var broadcastMAC = tcpip.LinkAddress([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})

func (ll *LogLink) Start() {
	// Create broadcast log group and stream (/net/broadcast/local)
	broadcastAddrRx := CloudwatchLinkAddress{ll.ep.LinkAddress(), broadcastMAC, ll.netName}
	err := ll.OpenLogStream(broadcastAddrRx)
	if err != nil {
		log.Fatalf("WritePacket: Could not create remote log group: %v", err)
	}

	localReadRx := CloudwatchLinkAddress{"", ll.ep.LinkAddress(), ll.netName}
	err = ll.createLogGroup(localReadRx.LogGroupName())
	if err != nil {
		log.Fatalf("OpenLogStream: Could not create remote log group: %v", err)
	}
	go ll.readPoller.ReadPollForLogGroup(localReadRx.LogGroupName())
	go ll.readPoller.ReadPollForBroadcast(broadcastAddrRx.LogGroupName())

	go ll.writePoller.WritePoll()
}

func (ll *LogLink) OpenLogStream(l CloudwatchLinkAddress) error {
	if _, ok := logstreamExistsCache[l.FullPath()]; !ok {
		// Create group
		err := ll.createLogGroup(l.LogGroupName())
		if err != nil {
			log.Fatalf("OpenLogStream: Could not create remote log group: %v", err)
		}

		// Create log stream
		err = ll.createLogStream(l)
		if err != nil {
			log.Fatalf("WritePacket: Could not create remote log stream: %v", err)
		}
		logstreamExistsCache[l.FullPath()] = true
	}

	return nil
}

func (ll *LogLink) createLogStream(l CloudwatchLinkAddress) error {
	_, err := ll.svc.CreateLogStream(&cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  aws.String(l.LogGroupName()),
		LogStreamName: aws.String(l.LogStreamName()),
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == cloudwatchlogs.ErrCodeResourceAlreadyExistsException {
				err = nil
			}
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// Read reads one packet from the internal buffers
func (ll *LogLink) Read() (*buffer.VectorisedView, error) {
	select {
	case event := <-ll.readPoller.Cr:
		if event.err != nil {
			log.Printf("Read: Poll Error: %v", event.err)
			break
		}

		// Unmarshal
		var packetLog PacketLog
		err := json.Unmarshal(event.data, &packetLog)
		if err != nil {
			return nil, err
		}

		h := make([]byte, 1024)
		n, err := base64.StdEncoding.Decode(h, []byte(packetLog.Header))
		if err != nil {
			return nil, err
		}
		header := buffer.NewViewFromBytes(h[:n])

		j := make([]byte, ll.ep.MTU())
		m, err := base64.StdEncoding.Decode(j, []byte(packetLog.Payload))
		if err != nil {
			return nil, err
		}
		payload := buffer.NewViewFromBytes(j[:m])

		vv := buffer.NewVectorisedView(n+m, []buffer.View{header, payload})
		return &vv, nil
	}
	return nil, nil
}

func (ll *LogLink) StringToProtocol(protocol string) tcpip.NetworkProtocolNumber {
	switch protocol {
	case ipv4.ProtocolName:
		return ipv4.ProtocolNumber
	case ipv6.ProtocolName:
		return ipv6.ProtocolNumber
	case arp.ProtocolName:
		return arp.ProtocolNumber
	default:
		return 0
	}
}

func (ll *LogLink) ProtocolToString(protocol tcpip.NetworkProtocolNumber) string {
	switch protocol {
	case ipv4.ProtocolNumber:
		return ipv4.ProtocolName
	case ipv6.ProtocolNumber:
		return ipv6.ProtocolName
	case arp.ProtocolNumber:
		return arp.ProtocolName
	default:
		return "unknown"
	}
}

// Write writes one packet to the internal buffers
func (ll *LogLink) Write(l CloudwatchLinkAddress, protocol tcpip.NetworkProtocolNumber, header []byte, payload []byte) (int, error) {
	// todo: replace with pcap-friendly format
	pl := PacketLog{ll.ProtocolToString(protocol), l.Src().String(), l.Dest().String(),
		base64.StdEncoding.EncodeToString(header), base64.StdEncoding.EncodeToString(payload)}
	plBytes, err := json.Marshal(pl)
	if err != nil {
		return 0, err
	}
	ll.writePoller.Cw <- WritePollInput{plBytes, &l}
	return len(plBytes), nil
}
