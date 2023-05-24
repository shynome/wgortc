package endpoint

import (
	"encoding/base64"
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
	if buf[0] == 1 {
		return ep.Connect(buf)
	}
	return ep.dc.Send(buf)
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

	try.To(WaitDC(dc, 5*time.Second))

	return
}

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
