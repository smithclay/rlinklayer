package cloudwatch

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs/cloudwatchlogsiface"
	"log"
	"time"
)

type ReadPollOutput struct {
	data []byte
	err  error
}

func (p *ReadPollOutput) Data() []byte {
	return p.data
}

func (p *ReadPollOutput) Error() error {
	return p.err
}

type ReadPoller struct {
	client            cloudwatchlogsiface.CloudWatchLogsAPI
	readThrottle      <-chan time.Time
	broadcastThrottle <-chan time.Time
	limit             int
	nextTokens        map[string]*string
	startTimes        map[string]int64

	Cr chan ReadPollOutput
}

func NewReadPoller(client cloudwatchlogsiface.CloudWatchLogsAPI) *ReadPoller {
	p := &ReadPoller{
		readThrottle:      time.Tick(time.Second / 4),
		broadcastThrottle: time.Tick(time.Second / 1),
		limit:             32,
		nextTokens:        map[string]*string{},
		startTimes:        map[string]int64{},
		client:            client,
		Cr:                make(chan ReadPollOutput, 32),
	}
	return p
}

func (p *ReadPoller) fetch(groupName string) {
	params := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String(groupName),
		NextToken:    p.nextTokens[groupName],
		Interleaved:  aws.Bool(true),
		StartTime:    aws.Int64(p.startTimes[groupName]),
	}

	resp, err := p.client.FilterLogEvents(params)
	if err != nil {
		p.Cr <- ReadPollOutput{err: err}
		return
	}

	// We want to re-use the existing token in the event that
	// NextForwardToken is nil, which means there's no new messages to
	// consume.
	if resp.NextToken != nil {
		p.nextTokens[groupName] = resp.NextToken
	}

	// If there are no messages, return so that the consumer can read again.
	if len(resp.Events) == 0 {
		return
	}
	for _, event := range resp.Events {
		p.Cr <- ReadPollOutput{[]byte(*event.Message), nil}
		p.startTimes[groupName] = aws.Int64Value(event.Timestamp) + 1
	}
}

func (p *ReadPoller) ReadPollForBroadcast(groupName string) {
	log.Printf("Reading bcast poll: %v", groupName)
	p.startTimes[groupName] = time.Now().Unix() * 1000
	for {
		<-p.broadcastThrottle
		p.fetch(groupName)
	}
}

func (p *ReadPoller) ReadPollForLogGroup(groupName string) {
	log.Printf("Reading stream poll: %v", groupName)
	p.startTimes[groupName] = time.Now().Unix() * 1000
	for {
		<-p.readThrottle
		p.fetch(groupName)
	}
}
