package peer

import (
	"net/netip"

	"github.com/shynome/wgortc/bind"
	"github.com/shynome/wgortc/nat"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
)

type Peer struct {
	bind.Peer
	Src4, Dst4 tcpip.Address
	Src6, Dst6 tcpip.Address
}

var _ bind.Peer = (*Peer)(nil)
var _ nat.INAT = (*Peer)(nil)

func New(peer bind.Peer) *Peer {
	return &Peer{
		Peer: peer,
	}
}

func (ep *Peer) SetNAT4(src, dst netip.Addr) {
	ep.Src4, ep.Dst4 = tcpip.AddrFrom4(src.As4()), tcpip.AddrFrom4(dst.As4())
}

func (ep *Peer) SetNAT6(src, dst netip.Addr) {
	ep.Src6, ep.Dst6 = tcpip.AddrFrom16(src.As16()), tcpip.AddrFrom16(dst.As16())
}

func (ep *Peer) NAT(buf []byte) {
	var (
		src  tcpip.Address
		dst  tcpip.Address
		nsrc tcpip.Address
		ndst tcpip.Address
	)

	var packet header.Network
	switch v := header.IPVersion(buf); v {
	case 4:
		pkt := header.IPv4(buf)
		src = pkt.SourceAddress()
		dst = pkt.DestinationAddress()
		nsrc = ep.Src4
		ndst = ep.Dst4

		switch {
		case
			nsrc.Len() == 0,
			ndst.Len() == 0:
			return
		}

		pkt.SetSourceAddressWithChecksumUpdate(nsrc)
		pkt.SetDestinationAddressWithChecksumUpdate(ndst)

		packet = pkt
	case 6:
		pkt := header.IPv6(buf)
		src = pkt.SourceAddress()
		dst = pkt.DestinationAddress()
		nsrc = ep.Src6
		ndst = ep.Dst6

		switch {
		case
			nsrc.Len() == 0,
			ndst.Len() == 0:
			return
		}

		pkt.SetSourceAddress(nsrc)
		pkt.SetDestinationAddress(ndst)

		packet = pkt
	default:
		return
	}

	payload := packet.Payload()
	switch p := packet.TransportProtocol(); p {
	case header.TCPProtocolNumber:
		tcp := header.TCP(payload)
		tcp.UpdateChecksumPseudoHeaderAddress(src, nsrc, true)
		tcp.UpdateChecksumPseudoHeaderAddress(dst, ndst, true)
	case header.UDPProtocolNumber:
		udp := header.UDP(payload)
		udp.UpdateChecksumPseudoHeaderAddress(src, nsrc, true)
		udp.UpdateChecksumPseudoHeaderAddress(dst, ndst, true)
	}
}
