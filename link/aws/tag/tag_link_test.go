package tag

import (
	"io"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/lambda"
)

func setupTagLink(t *testing.T) *TagLink {
	sess, _ := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2")},
	)
	svc := lambda.New(sess, &aws.Config{Region: aws.String("us-west-2")})
	endpoint := &endpoint{laddr: "ABC", raddr: "DEF"}
	config := TagConfig{
		LambdaService: svc,
		Endpoint:      endpoint,
		TxArn:         "",
		RxArn:         "",
		//Arn: "arn:aws:lambda:us-west-2:275197385476:function:helloWorldTestFunction",
	}
	return NewTagLink(&config)
}

func TestTagLink_Read(t *testing.T) {
	tl := setupTagLink(t)
	tables := []struct {
		read         []byte
		readBytes    int
		readError    error
		availPackets int
	}{
		{make([]byte, 255), 0, io.EOF, 0},
	}
	for i, table := range tables {
		tl.rxBuffer.Reset()
		if tl.rxBuffer.avail != table.availPackets {
			t.Errorf("[%d] TestTagLink_Read: expected %d avail packets, got %d", i, table.availPackets, tl.rxBuffer.avail)
		}
		n, err := tl.Read(table.read)
		if err != table.readError {
			t.Errorf("[%d] TestTagLink_Read: Expected read error %v, got: %v", i, table.readError, err)
		}
		if n != table.readBytes {
			t.Errorf("[%d] TestTagLink_Read: Expected to read bytes %d, got: %d", i, table.readBytes, n)
		}
	}
}

func TestTagLink_Write(t *testing.T) {
	tl := setupTagLink(t)

	tables := []struct {
		write        []byte
		writtenBytes int
		writeError   error
		availPackets int
	}{
		{make([]byte, 0), 0, nil, 8},
		{[]byte("helloworld"), 16, nil, 7},
	}
	for i, table := range tables {
		tl.txBuffer.Reset()
		n, err := tl.Write(table.write)

		if tl.txBuffer.avail != table.availPackets {
			t.Errorf("[%d] TestTagLink_Write: expected %d avail packets, got %d", i, table.availPackets, tl.txBuffer.avail)
		}

		if err != table.writeError {
			t.Errorf("[%d] Expected write error %v, got: %v", i, table.writeError, err)
		}
		if n != table.writtenBytes {
			t.Errorf("[%d] Expected to write bytes %d, got: %d", i, table.writtenBytes, n)
		}
	}
}
