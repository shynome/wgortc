package wgortc

import (
	"errors"
	"net"
	"sync"

	"github.com/lainio/err2"
	"github.com/lainio/err2/try"
	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"
	"github.com/shynome/wgortc/endpoint"
	"github.com/shynome/wgortc/mux"
	"github.com/shynome/wgortc/signaler"
	"golang.zx2c4.com/wireguard/conn"
)

type Bind struct {
	signaler.Channel

	api *webrtc.API
	mux ice.UDPMux

	ICEServers []webrtc.ICEServer

	msgCh chan packetMsg

	closed bool
	locker *sync.RWMutex
}

var _ conn.Bind = (*Bind)(nil)

func NewBind(signaler signaler.Channel) *Bind {
	return &Bind{
		Channel: signaler,

		closed: false,
		locker: &sync.RWMutex{},
	}
}

func (b *Bind) Open(port uint16) (fns []conn.ReceiveFunc, actualPort uint16, err error) {
	defer err2.Handle(&err)
	b.locker.Lock()
	defer b.locker.Unlock()

	fns = append(fns, b.receiveFunc)

	b.msgCh = make(chan packetMsg, b.BatchSize()-1)

	settingEngine := webrtc.SettingEngine{}
	if mux.WithUDPMux != nil {
		b.mux = try.To1(mux.WithUDPMux(settingEngine, &port))
		actualPort = port
	}
	b.api = webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))

	ch := try.To1(b.Accept())
	go func() {
		for ev := range ch {
			go b.handleConnect(ev)
		}
	}()

	b.closed = false
	return
}

type packetMsg struct {
	data []byte
	ep   conn.Endpoint
}

func (b *Bind) receiveFunc(packets [][]byte, sizes []int, eps []conn.Endpoint) (n int, err error) {
	if b.isClosed() {
		return 0, net.ErrClosed
	}
	for i := 0; i < b.BatchSize(); i++ {
		msg, ok := <-b.msgCh
		if !ok {
			return 0, net.ErrClosed
		}
		sizes[i] = copy(packets[i], msg.data)
		eps[i] = msg.ep
		n += 1
	}
	return
}

func (b *Bind) handleConnect(sess signaler.Session) {
	defer err2.Catch()

	config := webrtc.Configuration{
		ICEServers: b.ICEServers,
	}
	pc := try.To1(b.api.NewPeerConnection(config))
	defer pc.Close()

	inbound := endpoint.NewInbound(sess, pc)
	initiator := try.To1(inbound.ExtractInitiator())
	b.msgCh <- packetMsg{
		data: initiator,
		ep:   inbound,
	}

	ch := inbound.Message()
	for d := range ch {
		if b.isClosed() {
			break
		}
		b.msgCh <- packetMsg{
			data: d,
			ep:   inbound,
		}
	}

	return
}

func (b *Bind) isClosed() bool {
	b.locker.RLock()
	defer b.locker.RUnlock()
	return b.closed
}

func (b *Bind) Close() (err error) {
	defer err2.Handle(&err)

	b.locker.Lock()
	defer b.locker.Unlock()

	b.closed = true

	if b.mux != nil {
		try.To(b.mux.Close())
	}
	if b.Channel != nil {
		try.To(b.Channel.Close())
	}
	if b.msgCh != nil {
		close(b.msgCh)
	}
	return
}

func (b *Bind) ParseEndpoint(s string) (ep conn.Endpoint, err error) {
	outbound := endpoint.NewOutbound(s, b)
	go func() {
		ch := outbound.Message()
		for d := range ch {
			if b.isClosed() {
				break
			}
			b.msgCh <- packetMsg{
				data: d,
				ep:   outbound,
			}
		}
	}()
	return outbound, nil
}

var _ endpoint.Hub = (*Bind)(nil)

func (b *Bind) NewPeerConnection() (*webrtc.PeerConnection, error) {
	config := webrtc.Configuration{
		ICEServers: b.ICEServers,
	}
	return b.api.NewPeerConnection(config)
}

func (b *Bind) Send(bufs [][]byte, ep conn.Endpoint) (err error) {
	if b.isClosed() {
		return net.ErrClosed
	}
	sender, ok := ep.(endpoint.Sender)
	if !ok {
		return ErrEndpointImpl
	}
	for _, buf := range bufs {
		if err := sender.Send(buf); err != nil {
			return err
		}
	}
	return nil
}

var ErrEndpointImpl = errors.New("endpoint is not wgortc.Endpoint")

func (b *Bind) SetMark(mark uint32) error { return nil }
func (b *Bind) BatchSize() int            { return 1 }
