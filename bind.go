package wgortc

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/donovanhide/eventsource"
	"github.com/lainio/err2"
	"github.com/lainio/err2/try"
	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"
	"github.com/shynome/wgortc/mux"
	"golang.zx2c4.com/wireguard/conn"
)

type Bind struct {
	id       string
	signaler *signaler

	pcs  []*webrtc.PeerConnection
	pcsL sync.Locker

	api *webrtc.API
	mux ice.UDPMux

	connectionStream *eventsource.Stream

	ICEServers []webrtc.ICEServer

	msgCh chan packetMsg

	closed uint32
}

var _ conn.Bind = (*Bind)(nil)

func NewBind(id string, signaler string) *Bind {
	return &Bind{
		id:       id,
		signaler: newSignaler(signaler),

		pcsL: &sync.Mutex{},

		closed: 0,
	}
}

func (b *Bind) Open(port uint16) (fns []conn.ReceiveFunc, actualPort uint16, err error) {
	defer err2.Handle(&err)

	fns = append(fns, b.receiveFunc)

	b.msgCh = make(chan packetMsg, b.BatchSize()-1)
	b.pcs = make([]*webrtc.PeerConnection, 0)

	settingEngine := webrtc.SettingEngine{}
	if mux.WithUDPMux != nil {
		b.mux = try.To1(mux.WithUDPMux(settingEngine, port))
		actualPort = port
	}
	b.api = webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))

	req := try.To1(b.signaler.newReq(http.MethodGet, b.id, http.NoBody))
	b.connectionStream = try.To1(eventsource.SubscribeWithRequest("", req))
	go func() {
		for ev := range b.connectionStream.Events {
			go b.handleConnect(ev)
		}
	}()

	atomic.StoreUint32(&b.closed, 0)
	return
}

type packetMsg struct {
	data *[]byte
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
		sizes[i] = copy(packets[i], *msg.data)
		eps[i] = msg.ep
		n += 1
	}
	return
}

func (b *Bind) receiveMsg(ep conn.Endpoint) func(msg webrtc.DataChannelMessage) {
	return func(msg webrtc.DataChannelMessage) {
		if b.isClosed() {
			return
		}
		b.msgCh <- packetMsg{
			data: &msg.Data,
			ep:   ep,
		}
	}
}

func (b *Bind) handleConnect(ev eventsource.Event) {
	var pc *webrtc.PeerConnection
	defer err2.Catch(func(err error) {
		if pc != nil {
			pc.Close()
		}
	})

	var offer webrtc.SessionDescription
	try.To(json.Unmarshal([]byte(ev.Data()), &offer))
	sdp := try.To1(offer.Unmarshal())
	if sdp.Origin.Username == "" {
		return
	}

	config := webrtc.Configuration{
		ICEServers: b.ICEServers,
	}
	pc = try.To1(b.api.NewPeerConnection(config))
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		ep := b.NewEndpoint(sdp.Origin.Username)
		ep.init.Do(func() {
			ep.dc = dc
			ep.dsiableReconnect = true
		})
		dc.OnMessage(b.receiveMsg(ep))
	})
	pc.OnConnectionStateChange(func(pcs webrtc.PeerConnectionState) {
		if pcs == webrtc.PeerConnectionStateConnected {
			b.pcsL.Lock()
			defer b.pcsL.Unlock()
			b.pcs = append(b.pcs, pc)
		}
	})

	try.To(pc.SetRemoteDescription(offer))
	answer := try.To1(pc.CreateAnswer(nil))
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	try.To(pc.SetLocalDescription(answer))
	<-gatherComplete
	roffer := pc.LocalDescription()

	body := try.To1(json.Marshal(roffer))
	req := try.To1(b.signaler.newReq(http.MethodDelete, b.id, bytes.NewReader(body)))
	req.Header.Set("X-Event-Id", ev.Id())
	try.To1(b.signaler.doReq(req))

	return
}

func (b *Bind) isClosed() bool {
	return atomic.LoadUint32(&b.closed) != 0
}

func (b *Bind) Close() (err error) {
	defer err2.Handle(&err)

	atomic.StoreUint32(&b.closed, 1)

	if len(b.pcs) > 0 {
		func() {
			b.pcsL.Lock()
			defer b.pcsL.Unlock()
			for _, pc := range b.pcs {
				if pc != nil {
					try.To(pc.Close())
				}
			}
		}()
	}
	if b.mux != nil {
		try.To(b.mux.Close())
	}
	if b.connectionStream != nil {
		b.connectionStream.Close()
	}
	if b.msgCh != nil {
		close(b.msgCh)
	}
	return
}

func (b *Bind) ParseEndpoint(s string) (ep conn.Endpoint, err error) {
	return b.NewEndpoint(s), nil
}

func (b *Bind) Send(bufs [][]byte, ep conn.Endpoint) (err error) {
	if b.isClosed() {
		return net.ErrClosed
	}
	sender, ok := ep.(*Endpoint)
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
