package cloudwatch

import (
	"fmt"
	"github.com/google/netstack/tcpip"
	"strings"
)

type CloudwatchLinkAddress struct {
	laddr   tcpip.LinkAddress
	raddr   tcpip.LinkAddress
	netName string
}

func NewCloudwatchLinkAddress(laddr, raddr tcpip.LinkAddress, netName string) *CloudwatchLinkAddress {
	return &CloudwatchLinkAddress{laddr, raddr, netName}
}

func (cw *CloudwatchLinkAddress) FullPath() string {
	return fmt.Sprintf("%s/%s", cw.LogGroupName(), cw.LogStreamName())
}

func (cw *CloudwatchLinkAddress) LogGroupName() string {
	return fmt.Sprintf("%v/%v", cw.netName, cw.safeLinkAddr(cw.raddr))
}

func (cw *CloudwatchLinkAddress) LogStreamName() string {
	return cw.safeLinkAddr(cw.laddr)
}

func (cw *CloudwatchLinkAddress) safeLinkAddr(a tcpip.LinkAddress) string {
	return strings.Replace(a.String(), ":", "", -1)
}

func (cw *CloudwatchLinkAddress) Src() tcpip.LinkAddress {
	return cw.laddr
}

func (cw *CloudwatchLinkAddress) Dest() tcpip.LinkAddress {
	return cw.raddr
}
