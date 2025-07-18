package vtun

import (
	"context"
	"net"
	"net/netip"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
)

func DialContext(s GetStack) func(ctx context.Context, network, addr string) (net.Conn, error) {
	stk, nic := s.GetStack(), s.NIC()
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		ap, err := netip.ParseAddrPort(addr)
		if err != nil {
			return nil, err
		}
		fa, pn := ConvertToFullAddr(nic, ap)
		return gonet.DialContextTCP(ctx, stk, fa, pn)
	}
}

func ConvertToFullAddr(NICID tcpip.NICID, endpoint netip.AddrPort) (tcpip.FullAddress, tcpip.NetworkProtocolNumber) {
	var protoNumber tcpip.NetworkProtocolNumber
	if endpoint.Addr().Is4() {
		protoNumber = ipv4.ProtocolNumber
	} else {
		protoNumber = ipv6.ProtocolNumber
	}
	return tcpip.FullAddress{
		NIC:  NICID,
		Addr: tcpip.AddrFromSlice(endpoint.Addr().AsSlice()),
		Port: endpoint.Port(),
	}, protoNumber
}
