package bind

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"sync/atomic"

	"github.com/pion/webrtc/v4"
	"github.com/shynome/websocket"
	"github.com/shynome/wgortc/bind/whip"
	"golang.zx2c4.com/wireguard/conn"
)

type Endpoint struct {
	bind   bindPower
	logger *slog.Logger
	peer   Peer
	nowsc  atomic.Bool

	pc   atomic.Pointer[webrtc.PeerConnection]
	conn atomic.Pointer[websocket.Conn]
	dc   atomic.Pointer[webrtc.DataChannel]
}

var _ conn.Endpoint = (*Endpoint)(nil)
var _ whip.Sender = (*Endpoint)(nil)

func (ep *Endpoint) Send(buf []byte) error {
	if dc := ep.dc.Load(); dc != nil {
		err := dc.Send(buf)
		return err
	}
	if wsc := ep.conn.Load(); wsc != nil {
		if nowsc := ep.nowsc.Load(); nowsc && buf[0] == WireGuardMessageData {
			return ErrWSCDrop
		}
		ctx := context.Background()
		err := wsc.Write(ctx, websocket.MessageBinary, buf)
		return err
	}
	return ErrNoDataChannel
}

func (ep *Endpoint) ClearSrc() {
	ep.logger.Debug("clear src")
	if pc := ep.pc.Swap(nil); pc != nil {
		pc.Close()
	}
	if wsc := ep.conn.Swap(nil); wsc != nil {
		wsc.Close(websocket.StatusNormalClosure, "clear src")
	}
}

func (ep *Endpoint) SrcToString() string {
	if conn := ep.conn.Load(); conn != nil {
		return "127.0.0.1:1"
	}
	if pc := ep.pc.Load(); pc != nil {
		return "127.0.0.1:2"
	}
	if dc := ep.dc.Load(); dc != nil {
		return "127.0.0.1:3"
	}
	return ""
}

func (ep *Endpoint) DstToBytes() []byte {
	return []byte(ep.peer.GetID())
}

func (ep *Endpoint) DstToString() (addr string) {
	addr = "127.0.0.1:1"
	pc := ep.pc.Load()
	if pc == nil {
		return
	}
	sctp := pc.SCTP()
	if sctp == nil {
		return
	}
	dtls := sctp.Transport()
	if dtls == nil {
		return
	}
	ice := dtls.ICETransport()
	if ice == nil {
		return
	}
	pair, err := ice.GetSelectedCandidatePair()
	if err != nil {
		return
	}
	if pair == nil {
		return
	}
	remote := pair.Remote
	if remote == nil {
		return
	}
	raddr := remote.Address
	if raddr == "redacted-ip.invalid" {
		raddr = "255.255.255.255"
	}
	if strings.Contains(raddr, ":") {
		return fmt.Sprintf("[%s]:%d", raddr, remote.Port)
	}
	return fmt.Sprintf("%s:%d", raddr, remote.Port)
}

func (*Endpoint) DstIP() netip.Addr { return netip.Addr{} }
func (*Endpoint) SrcIP() netip.Addr { return netip.Addr{} }

var ErrNoDataChannel = errors.New("no available data channel")
var ErrWSCDrop = errors.New("drop data when nowsc is true")
