FROM quay.io/deis/go-dev:latest

ENV DEBIAN_FRONTEND noninteractive
ENV CGO_ENABLED 0

RUN apt-get update
RUN apt-get install -y iproute2
RUN apt-get install -y telnet
RUN apt-get install -y curl
RUN apt-get install -y iputils-ping
RUN apt-get install -y inetutils-traceroute
RUN apt-get install -y socat
RUN apt-get install -y netcat-openbsd

RUN go get -u github.com/google/btree
RUN go get -u golang.org/x/sys/unix

RUN go get -u github.com/google/netstack/tcpip

RUN go get -u github.com/aws/aws-sdk-go/aws
RUN go get -u github.com/aws/aws-sdk-go/service/lambda
RUN go get github.com/derekparker/delve/cmd/dlv

EXPOSE 40000

RUN mkdir -p /go/src/github.com/smithclay/rlinklayer
WORKDIR /go/src/github.com/smithclay/rlinklayer

COPY . /go/src/github.com/smithclay/rlinklayer

RUN go build -gcflags "all=-N -l" -o server examples/cwlink_bridge/main.go
RUN go build -gcflags "all=-N -l" -o client examples/cwlink_client/main.go

