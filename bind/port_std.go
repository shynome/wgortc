//go:build !(js || wasip1)

package bind

import (
	"context"
	"net"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
	"github.com/shynome/err0/try"
)

func init() {
	listenTry = listenStdTry
}

func listenStdTry(ctx context.Context, eg *webrtc.SettingEngine, port int) uint16 {
	eg.SetICEMulticastDNSMode(ice.MulticastDNSModeQueryOnly)

	// 暂时关闭 tcp 支持
	if false {
		listener := try.To1(net.ListenTCP("tcp", &net.TCPAddr{Port: port}))
		port = listener.Addr().(*net.TCPAddr).Port

		tcp := ice.NewTCPMuxDefault(ice.TCPMuxParams{Listener: listener})
		go func() {
			<-ctx.Done()
			tcp.Close()
		}()
		eg.SetICETCPMux(tcp)
	}

	// eg.SetNetworkTypes([]webrtc.NetworkType{
	// webrtc.NetworkTypeTCP4,
	// webrtc.NetworkTypeTCP6,
	// 	webrtc.NetworkTypeUDP4,
	// 	webrtc.NetworkTypeUDP6,
	// })

	if true {
		conn := try.To1(net.ListenUDP("udp", &net.UDPAddr{Port: port}))
		port = conn.LocalAddr().(*net.UDPAddr).Port

		udp := ice.NewUDPMuxDefault(ice.UDPMuxParams{UDPConn: conn})
		go func() {
			<-ctx.Done()
			udp.Close()
		}()
		eg.SetICEUDPMux(udp)
	}

	return uint16(port)
}
