package utils

import (
	"crypto/rand"
	"fmt"
	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/tcpip/network/ipv4"
	"github.com/google/netstack/tcpip/network/ipv6"
	"net"
)

// ipToAddressAndProto converts IP to tcpip.Address and a protocol number.
//
// Note: don't use 'len(ip)' to determine IP version because length is always 16.
func IpToAddressAndProto(ip net.IP) (tcpip.NetworkProtocolNumber, tcpip.Address) {
	if i4 := ip.To4(); i4 != nil {
		return ipv4.ProtocolNumber, tcpip.Address(i4)
	}
	return ipv6.ProtocolNumber, tcpip.Address(ip)
}

// IpToAddress converts IP to tcpip.Address, ignoring the protocol.
func IpToAddress(ip net.IP) tcpip.Address {
	_, addr := IpToAddressAndProto(ip)
	return addr
}

// GenerateRandomMac generates a random. locally-administered MAC address.
func GenerateRandomMac() tcpip.LinkAddress {
	buf := make([]byte, 6)
	_, err := rand.Read(buf)
	if err != nil {
		fmt.Println("error:", err)
		panic("GenerateRandomMac fail")
	}
	// Set the local bit
	buf[0] |= 2
	return tcpip.LinkAddress(buf)
}
