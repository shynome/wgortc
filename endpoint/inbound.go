package endpoint

import (
	"context"
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

type Inbound struct {
	baseEndpoint
	dc   *webrtc.DataChannel
	sess signaler.Session

	pc *webrtc.PeerConnection
	ch chan []byte
}

var (
	_ conn.Endpoint = (*Inbound)(nil)
	_ Sender        = (*Inbound)(nil)
)

func NewInbound(sess signaler.Session, pc *webrtc.PeerConnection) *Inbound {
	return &Inbound{
		baseEndpoint: baseEndpoint{
			id: sess.Description().SDP,
		},
		pc:   pc,
		sess: sess,
		ch:   make(chan []byte),
	}
}

func (ep *Inbound) Send(buf []byte) (err error) {
	closed := ep.dcIsClosed()
	if buf[0] == 2 && closed {
		go ep.HandleConnect(buf)
		return
	}
	if closed {
		return net.ErrClosed
	}
	go ep.dc.Send(buf)
	return
}

func (ep *Inbound) dcIsClosed() bool {
	if ep.dc == nil {
		return true
	}
	return ep.dc.ReadyState() != webrtc.DataChannelStateOpen
}

func (ep *Inbound) ExtractInitiator() (initiator []byte, err error) {
	defer err2.Handle(&err)
	offer := ep.sess.Description()
	sdp := try.To1(offer.Unmarshal())
	rawStr := sdp.SessionInformation
	if rawStr == nil {
		return nil, ErrInitiatorRequired
	}
	initiator = try.To1(base64.StdEncoding.DecodeString(string(*rawStr)))
	return initiator, nil
}

func (ep *Inbound) HandleConnect(buf []byte) (err error) {
	defer err2.Handle(&err, func() {
		ep.sess.Reject(err)
	})

	pc := ep.pc
	pc.OnConnectionStateChange(func(pcs webrtc.PeerConnectionState) {
		switch pcs {
		case webrtc.PeerConnectionStateDisconnected:
			pc.Close()
		}
	})

	try.To(pc.SetRemoteDescription(ep.sess.Description()))
	answer := try.To1(pc.CreateAnswer(nil))
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	try.To(pc.SetLocalDescription(answer))
	<-gatherComplete
	roffer := pc.LocalDescription()

	responder := sdp.Information(base64.StdEncoding.EncodeToString(buf))
	sdp := try.To1(roffer.Unmarshal())
	sdp.SessionInformation = &responder
	roffer.SDP = string(try.To1(sdp.Marshal()))

	try.To(ep.sess.Resolve(roffer))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx, cause := context.WithCancelCause(context.Background())
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		switch dc.Label() {
		case "wgortc":
			defer cause(nil)
			ep.dc = dc
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				ep.ch <- msg.Data
			})
		}
	})
	<-ctx.Done()
	if err := context.Cause(ctx); err != context.Canceled {
		try.To(err)
	}

	return
}

func (ep *Inbound) Message() (ch <-chan []byte) {
	return ep.ch
}

var ErrInitiatorRequired = errors.New("first message initiator is required in webrtc sdp SessionInformation")

func (ep *Inbound) DstToString() string {
	return getPCRemote(ep.pc)
}
