package wgortc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
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

	sdp := try.To1(offer.Unmarshal())
	sdp.Origin.Username = b.id
	offer.SDP = string(try.To1(sdp.Marshal()))

	body := try.To1(json.Marshal(offer))
	req := try.To1(b.signaler.newReq(http.MethodPost, ep.id, bytes.NewReader(body)))
	res := try.To1(b.signaler.doReq(req))

	var roffer webrtc.SessionDescription
	try.To(json.NewDecoder(res.Body).Decode(&roffer))
	try.To(pc.SetRemoteDescription(roffer))

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

	evs := map[string]chan any{}
	var locker sync.Locker = &sync.Mutex{}
	deleteCh := func(id string) {
		locker.Lock()
		defer locker.Unlock()
		ch, ok := evs[id]
		if !ok {
			return
		}
		close(ch)
		delete(evs, id)
	}
	addCh := func() (string, <-chan any) {
		locker.Lock()
		defer locker.Unlock()
		ch := make(chan any)
		buf := make([]byte, 32)
		var id string

		for {
			rand.Read(buf)
			id = string(buf)
			if _, ok := evs[string(id)]; !ok {
				evs[string(id)] = ch
				break
			}
		}
		return id, ch
	}
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		id := string(msg.Data)
		deleteCh(id)
	})
	t := time.NewTicker(3 * time.Second)
	dc.OnOpen(func() {
		for range t.C {
			func() {
				if ep.err != nil {
					return
				}

				id, ch := addCh()

				ctx := context.Background()
				ctx, cancelWith := context.WithCancelCause(ctx)
				ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				go func() {
					<-ctx.Done()
					deleteCh(id)
				}()

				dc.SendText(id)

				select {
				case <-ctx.Done():
					switch err := context.Cause(ctx); err {
					case context.Canceled:
						// normal cancel
					default:
						ep.err = err
						pc.Close()
					}
				case <-ch:
					cancelWith(nil)
				}
			}()
		}
	})
	dc.OnClose(func() {
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
