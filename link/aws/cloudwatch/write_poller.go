package cloudwatch

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs/cloudwatchlogsiface"
	"log"
	"strings"
	"time"
)

type WritePollInput struct {
	data   []byte
	cwLink *CloudwatchLinkAddress
}

func NewWritePollInput(data []byte, link *CloudwatchLinkAddress) WritePollInput {
	return WritePollInput{data, link}
}

type WritePoller struct {
	client         cloudwatchlogsiface.CloudWatchLogsAPI
	writeThrottle  <-chan time.Time
	limit          int
	sequenceTokens map[string]*string
	Cw             chan WritePollInput
}

func NewWritePoller(client cloudwatchlogsiface.CloudWatchLogsAPI) *WritePoller {
	p := &WritePoller{
		writeThrottle:  time.Tick(time.Second / 5),
		limit:          16,
		client:         client,
		sequenceTokens: map[string]*string{},
		Cw:             make(chan WritePollInput, 16),
	}
	return p
}

func (p *WritePoller) putLogEvents(events []*cloudwatchlogs.InputLogEvent, sequenceToken *string, groupName string, streamName string) (nextSequenceToken *string, err error) {
	resp, err := p.client.PutLogEvents(&cloudwatchlogs.PutLogEventsInput{
		LogEvents:     events,
		LogGroupName:  aws.String(groupName),
		LogStreamName: aws.String(streamName),
		SequenceToken: sequenceToken,
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() != cloudwatchlogs.ErrCodeInvalidSequenceTokenException {
				log.Printf(
					"Failed to put log: events: errorCode: %s message: %s, origError: %s log-group: %s log-stream: %s",
					awsErr.Code(),
					awsErr.Message(),
					awsErr.OrigErr(),
					groupName,
					streamName,
				)
			}
		} else {
			log.Printf("Failed to put log: %s", err)
		}

		return nil, err
	}
	sequenceToken = resp.NextSequenceToken
	return sequenceToken, nil
}

func (p *WritePoller) flush(events []*cloudwatchlogs.InputLogEvent, fullPath string, groupName string, streamName string) error {
	nextSequenceToken, err := p.putLogEvents(events, p.sequenceTokens[fullPath], groupName, streamName)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == cloudwatchlogs.ErrCodeDataAlreadyAcceptedException {
				// already submitted, just grab the correct sequence token
				parts := strings.Split(awsErr.Message(), " ")
				nextSequenceToken = &parts[len(parts)-1]
				// TODO log locally...
				log.Println(
					"Data already accepted, ignoring error",
					"errorCode: ", awsErr.Code(),
					"message: ", awsErr.Message(),
					"logGroupName: ", groupName,
					"logStreamName: ", streamName,
				)
				err = nil
			} else if awsErr.Code() == cloudwatchlogs.ErrCodeInvalidSequenceTokenException {

				// sequence code is bad, grab the correct one and retry
				parts := strings.Split(awsErr.Message(), " ")
				token := parts[len(parts)-1]
				nextSequenceToken, err = p.putLogEvents(events, aws.String(token), groupName, streamName)
			}
		}
	}

	if err != nil {
		log.Println("error flushing", err)
		return err
	} else {
		p.sequenceTokens[fullPath] = nextSequenceToken
	}
	return err
}

type PutEventInput struct {
	groupName  string
	streamName string
	fullPath   string
}

func (p *WritePoller) WritePoll() {
	for {
		<-p.writeThrottle
		events := make(map[PutEventInput][]*cloudwatchlogs.InputLogEvent, 0)
		select {
		case writeInput := <-p.Cw:
			cwInput := &cloudwatchlogs.InputLogEvent{
				Message:   aws.String(string(writeInput.data)),
				Timestamp: aws.Int64(time.Now().UnixNano() / 1000000),
			}
			pei := PutEventInput{writeInput.cwLink.LogGroupName(), writeInput.cwLink.LogStreamName(), writeInput.cwLink.FullPath()}
			events[pei] = append(events[pei], cwInput)
		default:
			continue
		}
		if len(events) > 0 {
			// Flush written events for each unique EndpointLogStream
			for k, v := range events {
				err := p.flush(v, k.fullPath, k.groupName, k.streamName)
				if err != nil {
					log.Printf("Error flushing: %v", err)
				}
			}
		} else {
			log.Printf("WritePoll: no events to flush")
		}
	}
}
