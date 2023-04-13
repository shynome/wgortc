package wgortc

import (
	"bytes"
	"encoding/json"
	"errors"
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

	try.To(waitDC(dc, 10*time.Second))
	ep.dc = dc

	go func() {
		b.pcsL.Lock()
		defer b.pcsL.Unlock()
		b.pcs = append(b.pcs, pc)
	}()

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