package endpoint

import (
	"encoding/base64"
	"errors"
	"net"
	"time"

	"github.com/lainio/err2"
	"github.com/lainio/err2/try"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
	"github.com/shynome/wgortc/signaler"
	"golang.zx2c4.com/wireguard/conn"
)

type Outbound struct {
	baseEndpoint
	pc  *webrtc.PeerConnection
	dc  *webrtc.DataChannel
	hub Hub
	ch  chan []byte
}

var (
	_ conn.Endpoint = (*Outbound)(nil)
	_ Sender        = (*Outbound)(nil)
)

type Hub interface {
	NewPeerConnection() (*webrtc.PeerConnection, error)
	signaler.Channel
}

func NewOutbound(id string, hub Hub) *Outbound {
	return &Outbound{
		baseEndpoint: baseEndpoint{id: id},

		hub: hub,
		ch:  make(chan []byte),
	}
}

func (ep *Outbound) Send(buf []byte) (err error) {
	closed := ep.dcIsClosed()
	if buf[0] == 1 && closed {
		go ep.Connect(buf)
		return
	}
	if closed {
		return net.ErrClosed
	}
	go ep.dc.Send(buf)
	return
}

func (ep *Outbound) dcIsClosed() bool {
	if ep.dc == nil {
		return true
	}
	return ep.dc.ReadyState() != webrtc.DataChannelStateOpen
}

func (ep *Outbound) Connect(buf []byte) (err error) {
	defer err2.Handle(&err)

	var pc *webrtc.PeerConnection = ep.pc
	if pc != nil {
		pc.Close()
	}

	pc = try.To1(ep.hub.NewPeerConnection())
	ep.pc = pc

	dcinit := webrtc.DataChannelInit{
		Ordered:        refVal(false),
		MaxRetransmits: refVal(uint16(0)),
	}
	dc := try.To1(pc.CreateDataChannel("wgortc", &dcinit))
	ep.dc = dc

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		ep.ch <- msg.Data
	})

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	offer := try.To1(pc.CreateOffer(nil))
	try.To(pc.SetLocalDescription(offer))
	<-gatherComplete
	offer = *pc.LocalDescription()

	initiator := sdp.Information(base64.StdEncoding.EncodeToString(buf))
	sdp := try.To1(offer.Unmarshal())
	sdp.SessionInformation = &initiator
	offer.SDP = string(try.To1(sdp.Marshal()))

	anwser := try.To1(ep.hub.Handshake(ep.id, offer))

	try.To(pc.SetRemoteDescription(*anwser))

	sdp2 := try.To1(anwser.Unmarshal())
	if sdp2.SessionInformation == nil {
		return ErrInitiatorResponderRequired
	}
	responder := try.To1(base64.StdEncoding.DecodeString(string(*sdp2.SessionInformation)))

	try.To(WaitDC(dc, 5*time.Second))
	ep.ch <- responder

	return
}

var ErrInitiatorResponderRequired = errors.New("first message initiator responder is required in webrtc sdp SessionInformation")

func (ep *Outbound) Close() (err error) {
	if pc := ep.pc; pc != nil {
		if err = pc.Close(); err != nil {
			return
		}
	}
	return
}

func (ep *Outbound) Message() (ch <-chan []byte) {
	return ep.ch
}

func (ep *Outbound) DstToString() string {
	return getPCRemote(ep.pc)
}
