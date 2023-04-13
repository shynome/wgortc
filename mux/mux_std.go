//go:build !(js || wasip1)

package mux

import (
	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"
)

func init() {
	WithUDPMux = func(engine webrtc.SettingEngine, port uint16) (mux ice.UDPMux, err error) {
		if mux, err = ice.NewMultiUDPMuxFromPort(int(port)); err != nil {
			return
		}
		engine.SetICEUDPMux(mux)
		return
	}
}
