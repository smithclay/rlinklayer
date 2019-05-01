// Copyright 2019 Clay Smith

// +build linux

package utils

import (
	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/tcpip/link/fdbased"
	"github.com/google/netstack/tcpip/link/rawfile"
	"github.com/google/netstack/tcpip/link/tun"
	"log"
)

func NewTapLink(tapName string, addr tcpip.LinkAddress) tcpip.LinkEndpointID {
	mtu, err := rawfile.GetMTU(tapName)
	if err != nil {
		log.Fatalf("newTapLink: could not get mtu: %v", err)
	}

	var fd int
	fd, err = tun.OpenTAP(tapName)

	if err != nil {
		log.Fatalf("newTapLink: could not open %v: %v", tapName, err)
	}

	linkID := fdbased.New(&fdbased.Options{
		FD:             fd,
		MTU:            mtu,
		Address:        addr,
		EthernetHeader: true, // set to true if tap
	})
	return linkID
}

func NewTunLink(tunName string) tcpip.LinkEndpointID {
	mtu, err := rawfile.GetMTU(tunName)
	if err != nil {
		log.Fatalf("newTunLink: could not get mtu: %v", err)
	}

	var fd int
	fd, err = tun.Open(tunName)

	if err != nil {
		log.Fatalf("newTunLink: could not open %v: %v", tunName, err)
	}

	linkID := fdbased.New(&fdbased.Options{
		FD:             fd,
		MTU:            mtu,
		EthernetHeader: false, // set to true if tap
	})
	return linkID
}
