package wgortc

import (
	"errors"
	"net/netip"
	"sync"
	"time"

	"github.com/lainio/err2"
	"github.com/lainio/err2/try"
	"github.com/pion/webrtc/v3"
	"golang.zx2c4.com/wireguard/conn"
)

type Endpoint struct {
	id   string
	dc   *webrtc.DataChannel
	err  error
	init *sync.Once

	bind *Bind

	dsiableReconnect bool
}

var _ conn.Endpoint = (*Endpoint)(nil)

func (b *Bind) NewEndpoint(id string) *Endpoint {
	return &Endpoint{
		id:   id,
		bind: b,
		init: &sync.Once{},
	}
}

// used for mac2 cookie calculations
func (ep *Endpoint) DstToBytes() []byte {
	return []byte(ep.id)
}
func (ep *Endpoint) DstToString() string { return ep.id } // returns the destination address (ip:port)

func (*Endpoint) ClearSrc()           {}            // clears the source address
func (*Endpoint) SrcToString() string { return "" } // returns the local source address (ip:port)
func (*Endpoint) DstIP() netip.Addr   { return netip.Addr{} }
func (*Endpoint) SrcIP() netip.Addr   { return netip.Addr{} }

func (ep *Endpoint) connect() {
	var pc *webrtc.PeerConnection
	defer err2.Catch(func(err error) {
		if pc != nil {
			pc.Close()
		}
		ep.err = err
	})

	b := ep.bind
	config := webrtc.Configuration{
		ICEServers: b.ICEServers,
	}
	pc = try.To1(b.api.NewPeerConnection(config))
	go ep.checkalive(pc)

	dcinit := webrtc.DataChannelInit{
		Ordered:        refVal(false),
		MaxRetransmits: refVal(uint16(0)),
	}
	dc := try.To1(pc.CreateDataChannel("wgortc", &dcinit))
	dc.OnMessage(b.receiveMsg(ep))

	offer := try.To1(pc.CreateOffer(nil))
	try.To(pc.SetLocalDescription(offer))
	offer = *pc.LocalDescription()

	roffer := try.To1(b.signaler.Handshake(ep.id, offer))

	try.To(pc.SetRemoteDescription(*roffer))

	try.To(waitDC(dc, 5*time.Second))
	ep.dc = dc

	go func() {
		b.pcsL.Lock()
		defer b.pcsL.Unlock()
		i := len(b.pcs)
		b.pcs = append(b.pcs, pc)
		pc.OnConnectionStateChange(func(pcs webrtc.PeerConnectionState) {
			if pcs == webrtc.PeerConnectionStateClosed {
				b.pcsL.Lock()
				defer b.pcsL.Unlock()
				b.pcs[i] = nil
			}
		})
	}()

}
func (ep *Endpoint) checkalive(pc *webrtc.PeerConnection) {

	dcinit := &webrtc.DataChannelInit{
		Ordered: refVal(false),
	}
	dc := try.To1(pc.CreateDataChannel("alive", dcinit))

	t := time.NewTimer(10 * time.Second)
	defer func() {
		<-t.C
		pc.Close()
	}()
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		t.Reset(10 * time.Second)
	})
	var ticker *time.Ticker
	dc.OnOpen(func() {
		ticker = time.NewTicker(3 * time.Second)
		go func() {
			for range ticker.C {
				dc.SendText("ping")
			}
		}()
	})
	dc.OnClose(func() {
		if ticker != nil {
			ticker.Stop()
		}
		t.Stop()
	})
}
func (ep *Endpoint) resetWhenWrong(err *error) {
	if ep.dsiableReconnect {
		return
	}
	if *err == nil {
		return
	}
	if ep.bind.isClosed() { // if bind is closed, don't reset
		return
	}
	ep.init = &sync.Once{}
	ep.err = nil
}
func (ep *Endpoint) Send(data []byte) (err error) {
	ep.init.Do(ep.connect)
	defer ep.resetWhenWrong(&err)
	if err = ep.err; err != nil {
		return
	}
	if ep.dc == nil {
		return ErrEndpointDataChannelNotReady
	}
	return ep.dc.Send(data)
}

var ErrEndpointDataChannelNotReady = errors.New("endpoint dc is not ready")
