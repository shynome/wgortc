package endpoint

import (
	"fmt"
	"net"
	"net/netip"

	"github.com/pion/webrtc/v3"
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
func (ep *baseEndpoint) DstToString() string { return getPCRemote(nil) } // returns the destination address (ip:port)

func (*baseEndpoint) ClearSrc()           {}            // clears the source address
func (*baseEndpoint) SrcToString() string { return "" } // returns the local source address (ip:port)
func (*baseEndpoint) DstIP() netip.Addr   { return netip.Addr{} }
func (*baseEndpoint) SrcIP() netip.Addr   { return netip.Addr{} }

func getPCRemote(pc *webrtc.PeerConnection) (addr string) {
	addr = "[fdd9:f800::]:80"
	if pc == nil {
		return
	}
	sctp := pc.SCTP()
	if sctp == nil {
		return
	}
	dtls := sctp.Transport()
	if dtls == nil {
		return
	}
	ice := dtls.ICETransport()
	if ice == nil {
		return
	}
	pair, err := ice.GetSelectedCandidatePair()
	if err != nil {
		return "[fdd9:f800::1]:80"
	}
	remote := pair.Remote
	if remote == nil {
		return "[fdd9:f800::2]:80"
	}
	ip := net.ParseIP(remote.Address)
	isv4 := ip.To4() != nil
	if isv4 {
		return fmt.Sprintf("%s:%d", remote.Address, remote.Port)
	}
	return fmt.Sprintf("[%s]:%d", remote.Address, remote.Port)
}
