package bind

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"sync/atomic"

	"github.com/pion/webrtc/v4"
	"github.com/shynome/websocket"
	"github.com/shynome/wgortc/bind/whip"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
)

type Endpoint struct {
	bind   bindPower
	logger *slog.Logger
	peer   Peer
	mode   atomic.Uint32

	pc   atomic.Pointer[webrtc.PeerConnection]
	conn atomic.Pointer[websocket.Conn]
	dc   atomic.Pointer[webrtc.DataChannel]

	expired atomic.Bool
	pubkey  device.NoisePublicKey
}

var _ conn.Endpoint = (*Endpoint)(nil)
var _ whip.Sender = (*Endpoint)(nil)

func (ep *Endpoint) Send(buf []byte) error {
	tm := TransportMode(ep.mode.Load())
	if peer, ok := ep.peer.(PeerHandshakeHook); ok && buf[0] == WireGuardMessageResponder {
		peer.HandshakedHook(ep)
	}
	if dc := ep.dc.Load(); dc != nil {
		err := dc.Send(buf)
		return err
	}
	if wsc := ep.conn.Load(); wsc != nil {
		nowsc := tm&WSTransportDisabled != 0
		if nowsc && buf[0] == WireGuardMessageData {
			return ErrWSCDrop
		}
		ctx := context.Background()
		err := wsc.Write(ctx, websocket.MessageBinary, buf)
		return err
	}
	ep.failFast()
	return ErrNoDataChannel
}

type PeerPubkey interface {
	GetPubkey() device.NoisePublicKey
}

func (ep *Endpoint) failFast() {
	if ep.expired.Swap(true) {
		return
	}
	if ep.pubkey.IsZero() {
		return
	}
	dev := ep.bind.GetDevice()
	if dev == nil {
		return
	}
	peer := dev.LookupPeer(ep.pubkey)
	if peer == nil {
		return
	}
	peer.ExpireCurrentKeypairs()
}

func (ep *Inbound) receive(buf []byte) bool {
	if buf[0] == WireGuardMessageResponder {
		ep.expired.Store(false)
		if peer, ok := ep.peer.(PeerHandshakeHook); ok {
			peer.HandshakedHook(ep)
		}
	}
	return ep.bind.Receive(ep, buf)
}

func (ep *Outbound) receive(buf []byte) bool {
	if buf[0] == WireGuardMessageResponder {
		ep.expired.Store(false)
		if peer, ok := ep.peer.(PeerHandshakeHook); ok {
			peer.HandshakedHook(ep)
		}
	}
	return ep.bind.Receive(ep, buf)
}

func (ep *Endpoint) ClearSrc() {
	ep.logger.Debug("clear src")
	if pc := ep.pc.Swap(nil); pc != nil {
		go pc.Close()
	}
	if wsc := ep.conn.Swap(nil); wsc != nil {
		go wsc.Close(websocket.StatusNormalClosure, "clear src")
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
