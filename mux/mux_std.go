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
		if err = initPort(port); err != nil {
			return
		}
		f := ice.UDPMuxFromPortWithIPFilter(checkIP)
		if mux, err = ice.NewMultiUDPMuxFromPort(int(*port), f); err != nil {
			return
		}
		engine.SetICEUDPMux(mux)
		return
	}
}

func initPort(port *uint16) (err error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: int(*port)})
	if err != nil {
		return
	}
	defer conn.Close()

	p := netip.MustParseAddrPort(conn.LocalAddr().String())
	*port = p.Port()
	return nil
}

func checkIP(ip net.IP) bool {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: ip})
	if err != nil {
		return false
	}
	defer conn.Close()

	return true
}
