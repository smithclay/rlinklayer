package tag

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/lambda"
)

type FunctionTags map[string]string

// TagStats captures data on packets sent or received in AWS Lambda tags
type TagStats struct {
	RxErrors      uint32
	TxErrors      uint32
	AwsRequests   uint32
	UpdatedTxTags uint32
	DeletedRxTags uint32
}

func (t FunctionTags) String() string {
	a := make([]string, 0)
	for k := range t {
		a = append(a, k)
	}
	return fmt.Sprintf("[%v]", strings.Join(a, ","))
}

// TagLink reads/writes L2 data to AWS service(s)
type TagLink struct {
	svc      *lambda.Lambda
	txArn    string
	rxArn    string
	ep       *endpoint
	rxBuffer *TagRing
	txBuffer *TagRing
	stats    *TagStats
	mtu      int
	// todo: look into implementing this with channels
	txHarvester *TagHarvester
	rxHarvester *TagHarvester
	txMux       sync.Mutex
	rxMux       sync.Mutex
}

// todo: can we just read in a bunch of packets at once?
// https://github.com/google/netstack/blob/74ad0f9b269317db70f62402f7d4c51b2c7ca0b7/tcpip/link/fdbased/endpoint.go#L155
// e.views = make([][]buffer.View, msgsPerRecv)

// recieving multiple packets at once:
// https://github.com/google/netstack/blob/74ad0f9b269317db70f62402f7d4c51b2c7ca0b7/tcpip/link/fdbased/endpoint.go#L330

type TagConfig struct {
	LambdaService *lambda.Lambda
	Endpoint      *endpoint
	RxArn         string // local (receive lambda tags)
	TxArn         string // remote (transmit lambda tags)
}

type TagHarvester struct {
	t          *time.Ticker
	d          time.Duration
	svc        *lambda.Lambda
	arn        string
	mux        sync.Mutex
	tagHandler func(map[string]*string, error)
	err        chan error
}

func NewTagHarvester(d time.Duration, svc *lambda.Lambda, arn string, mux sync.Mutex, tagHandler func(map[string]*string, error)) *TagHarvester {
	return &TagHarvester{
		d:          d,
		arn:        arn,
		svc:        svc,
		mux:        mux,
		tagHandler: tagHandler,
	}
}

func (th *TagHarvester) Start() {
	th.t = time.NewTicker(th.d)
	go func() {
		for {
			select {
			case <-th.t.C:
				th.mux.Lock()
				tagsOutput, err := th.svc.ListTags(&lambda.ListTagsInput{Resource: aws.String(th.arn)})
				if err != nil {
					th.tagHandler(nil, err)
					th.mux.Unlock()
					return
				}
				th.tagHandler(tagsOutput.Tags, nil)
				th.mux.Unlock()
			}
		}
	}()
}

func (th *TagHarvester) Stop() {
	th.t.Stop()
}

// todo: define how this maps to multi-tags (?)
var BufConfig = []int{255, 255, 255, 255, 255, 255, 255, 255}

const PollInterval = 500 * time.Millisecond

func NewTagLink(config *TagConfig) *TagLink {
	tagLink := TagLink{mtu: 255, txArn: config.TxArn, rxArn: config.RxArn, svc: config.LambdaService, stats: &TagStats{}, ep: config.Endpoint}
	tagLink.txBuffer = NewTagRing(len(BufConfig), TransmitType)
	tagLink.rxBuffer = NewTagRing(len(BufConfig), ReceiveType)
	tagLink.txHarvester = NewTagHarvester(PollInterval, config.LambdaService, config.TxArn, tagLink.txMux, tagLink.refreshTxInternalBuffers)
	tagLink.rxHarvester = NewTagHarvester(PollInterval, config.LambdaService, config.RxArn, tagLink.rxMux, tagLink.refreshRxInternalBuffers)
	return &tagLink
}

func (t *TagLink) StartPolling() {
	t.txHarvester.Start()
	t.rxHarvester.Start()
}

// tagHandler
func (t *TagLink) refreshTxInternalBuffers(tags map[string]*string, err error) {
	if err != nil {
		panic(err)
	}
	t.txBuffer.Reset()
	for i := 0; i < len(BufConfig); i++ {
		var buf []byte
		// Transmit tags
		if val, ok := tags[t.TxTagIndex(i)]; ok {
			buf = []byte(aws.StringValue(val))
		} else {
			buf = make([]byte, 0)
		}
		if err := t.txBuffer.Replace(i, buf); err != nil {
			panic(err)
		}
	}

	return
}

func (t *TagLink) refreshRxInternalBuffers(tags map[string]*string, err error) {
	if err != nil {
		panic(err)
	}
	t.rxBuffer.Reset()
	for i := 0; i < len(BufConfig); i++ {
		var buf []byte
		// Receive tags
		if val, ok := tags[t.RxTagIndex(i)]; ok {
			buf = []byte(aws.StringValue(val))
		} else {
			buf = make([]byte, 0)
		}
		if err := t.rxBuffer.Replace(i, buf); err != nil {
			panic(err)
		}
	}

	return
}

func (t *TagLink) LinkAddressLabel() string {
	return fmt.Sprintf("link:%s", t.ep.laddr)
}

func (t *TagLink) RemoteLinkAddressLabel() string {
	return fmt.Sprintf("link:%s", t.ep.raddr)
}

func (t *TagLink) RxTagIndex(i int) string {
	return fmt.Sprintf("%s.%d", t.LinkAddressLabel(), i)
}

func (t *TagLink) TxTagIndex(i int) string {
	return fmt.Sprintf("%s.%d", t.RemoteLinkAddressLabel(), i)
}

func (t *TagLink) String() string {
	var s []string
	for i := 0; i < len(BufConfig); i++ {
		if t.rxBuffer.Seek(i).b.Offset() > 0 {
			s = append(s, fmt.Sprintf("[%d] Rx Buffer: %v", i, t.rxBuffer.Seek(i)))
		}
	}
	for j := 0; j < len(BufConfig); j++ {
		if t.txBuffer.Seek(j).b.Len() > 0 {
			s = append(s, fmt.Sprintf("[%d] Tx Buffer: %v", j, t.txBuffer.Seek(j)))
		}
	}
	return strings.Join(s, "\n")
}

func (t *TagLink) FlushTransmit() (*string, error) {
	if t.txArn == "" {
		return nil, nil
	}

	updatedTags := make(FunctionTags)
	i := t.txBuffer.lastWriteOp

	if i == TagRingNoOp {
		return nil, nil
	}

	if t.txBuffer.Seek(i).b.Len() == 0 {
		panic("FlushTransmit: Unexpected flush of empty buffer")
	}

	updatedTags[t.TxTagIndex(i)] = t.txBuffer.Seek(i).b.EncodedBytesString()

	_, err := t.updateTags(updatedTags)
	if err != nil {
		return nil, err
	}
	t.stats.UpdatedTxTags++
	return aws.String(t.TxTagIndex(i)), nil
}

func (t *TagLink) ReceivePacketLen() int {
	return t.rxBuffer.avail
}

func (t *TagLink) FlushReceive() (*string, error) {
	if t.rxArn == "" {
		return nil, nil
	}

	i := t.rxBuffer.lastReadOp
	buf := t.rxBuffer.Seek(i).b
	if i == TagRingNoOp {
		return nil, nil
	}
	if buf.Offset() == 0 {
		panic("FlushReceive: Unexpected flush of empty buffer")
	}

	buf.Reset()
	clearTag := t.RxTagIndex(i)
	_, err := t.removeTags([]string{clearTag})
	if err != nil {
		return nil, err
	}
	t.stats.DeletedRxTags++
	return aws.String(clearTag), nil
}

// TODO: make this a blocking read until new bytes are in

// Read reads one packet from the internal buffers
func (t *TagLink) Read(p []byte) (int, error) {
	t.rxMux.Lock()
	defer t.rxMux.Unlock()

	n, err := t.rxBuffer.Read(p)
	if n > 0 {
		_, err = t.FlushReceive()
		if err != nil {
			// TODO: how to recover from this
			return 0, err
		}
	}
	return n, err
}

// Write writes one packet to the internal buffers
func (t *TagLink) Write(p []byte) (int, error) {
	t.txMux.Lock()
	defer t.txMux.Unlock()

	n, err := t.txBuffer.Write(p)
	if err != nil {
		return n, err
	}

	_, err = t.FlushTransmit()
	if err != nil {
		// TODO: how to recover from this (?)
		log.Printf("WritePacket: Error flushing to remote link, dropping packet: %v", err)
		return 0, err
	}

	return n, nil
}

func (t *TagLink) removeTags(tagKeys []string) (*lambda.UntagResourceOutput, error) {
	t.stats.AwsRequests++
	tagInput := &lambda.UntagResourceInput{
		Resource: aws.String(t.rxArn),
		TagKeys:  aws.StringSlice(tagKeys),
	}
	output, err := t.svc.UntagResource(tagInput)
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (t *TagLink) updateTags(tags FunctionTags) (*lambda.TagResourceOutput, error) {
	t.stats.AwsRequests++
	tagInput := &lambda.TagResourceInput{
		Resource: aws.String(t.txArn),
		Tags:     aws.StringMap(tags),
	}
	output, err := t.svc.TagResource(tagInput)
	if err != nil {
		return nil, err
	}
	return output, nil
}
