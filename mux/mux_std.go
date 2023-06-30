//go:build !(js || wasip1)

package mux

import (
	"net"
	"net/netip"

	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"
)

func init() {
	WithUDPMux = func(engine webrtc.SettingEngine, port *uint16) (mux ice.UDPMux, err error) {
		conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: int(*port)})
		if err != nil {
			return
		}
		p := netip.MustParseAddrPort(conn.LocalAddr().String())
		*port = p.Port()
		if mux, err = ice.NewMultiUDPMuxFromPort(int(*port)); err != nil {
			return
		}
		engine.SetICEUDPMux(mux)
		return
	}
}
