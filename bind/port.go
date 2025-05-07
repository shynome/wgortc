package bind

import (
	"context"

	"github.com/pion/webrtc/v4"
)

var listenTry func(ctx context.Context, eg *webrtc.SettingEngine, port int) uint16
