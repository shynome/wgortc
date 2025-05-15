package vtun

import (
	"fmt"
	"net/netip"

	"github.com/shynome/err0"
	"github.com/shynome/err0/try"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

func RouteUp(tdev GetStack, routes []string) (err error) {
	defer err0.Then(&err, nil, nil)
	stk := tdev.GetStack()
	for _, route := range routes {
		pf := try.To1(netip.ParsePrefix(route))
		protoNumber := ipv6.ProtocolNumber
		if pf.Addr().Is4() {
			protoNumber = ipv4.ProtocolNumber
		}
		protoAddr := tcpip.ProtocolAddress{
			Protocol: protoNumber,
			AddressWithPrefix: tcpip.AddressWithPrefix{
				Address:   tcpip.AddrFromSlice(pf.Addr().AsSlice()),
				PrefixLen: pf.Bits(),
			},
		}
		nic := tdev.NIC()
		tcpipErr := stk.AddProtocolAddress(nic, protoAddr, stack.AddressProperties{})
		if tcpipErr != nil {
			return fmt.Errorf("AddProtocolAddress(%v): %v", pf.String(), tcpipErr)
		}
	}
	return nil
}
