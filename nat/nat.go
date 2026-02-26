package nat

import (
	"net/netip"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
)

type NATC struct {
	Src4, Dst4 tcpip.Address
	Src6, Dst6 tcpip.Address
	Ports      map[uint16]struct{}
}

func New() *NATC {
	return &NATC{}
}

var _ INAT = (*NATC)(nil)

func (n *NATC) SetNAT4(src, dst netip.Addr) {
	n.Src4, n.Dst4 = tcpip.AddrFrom4(src.As4()), tcpip.AddrFrom4(dst.As4())
}

func (n *NATC) SetNAT6(src, dst netip.Addr) {
	n.Src6, n.Dst6 = tcpip.AddrFrom16(src.As16()), tcpip.AddrFrom16(dst.As16())
}

var (
	blackhole_ipv6 = tcpip.AddrFrom16(netip.MustParseAddr("100::").As16())
	blackhole_ipv4 = tcpip.AddrFrom4(netip.MustParseAddr("203.0.113.0").As4())
)

func (n *NATC) NAT(buf []byte) {
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
		nsrc = n.Src4
		ndst = n.Dst4

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
		nsrc = n.Src6
		ndst = n.Dst6

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

	applyPortFilter := func(port uint16) {
		if n.Ports == nil {
			return
		}
		if _, ok := n.Ports[port]; ok {
			return
		}
		switch ndst.Len() {
		case 16:
			ndst = blackhole_ipv6
		case 4:
			ndst = blackhole_ipv4
		}
		packet.SetDestinationAddress(ndst)
	}

	payload := packet.Payload()
	switch p := packet.TransportProtocol(); p {
	case header.TCPProtocolNumber:
		tcp := header.TCP(payload)
		port := tcp.DestinationPort()
		applyPortFilter(port)
		tcp.UpdateChecksumPseudoHeaderAddress(src, nsrc, true)
		tcp.UpdateChecksumPseudoHeaderAddress(dst, ndst, true)
	case header.UDPProtocolNumber:
		udp := header.UDP(payload)
		port := udp.DestinationPort()
		applyPortFilter(port)
		udp.UpdateChecksumPseudoHeaderAddress(src, nsrc, true)
		udp.UpdateChecksumPseudoHeaderAddress(dst, ndst, true)
	}
}

// NAT Instance
type INAT interface {
	NAT(buf []byte)
}
