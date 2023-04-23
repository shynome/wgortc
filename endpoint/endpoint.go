package endpoint

import (
	"net/netip"

	"golang.zx2c4.com/wireguard/conn"
)

type Sender interface {
	Send(buf []byte) error
}

type baseEndpoint struct {
	id string
}

var _ conn.Endpoint = (*baseEndpoint)(nil)

// used for mac2 cookie calculations
func (ep *baseEndpoint) DstToBytes() []byte {
	return []byte(ep.id)
}
func (ep *baseEndpoint) DstToString() string { return ep.id } // returns the destination address (ip:port)

func (*baseEndpoint) ClearSrc()           {}            // clears the source address
func (*baseEndpoint) SrcToString() string { return "" } // returns the local source address (ip:port)
func (*baseEndpoint) DstIP() netip.Addr   { return netip.Addr{} }
func (*baseEndpoint) SrcIP() netip.Addr   { return netip.Addr{} }
