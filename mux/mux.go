package mux

import (
	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"
)

var WithUDPMux func(engine webrtc.SettingEngine, port uint16) (ice.UDPMux, error)
