package endpoint

import (
	"context"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/lainio/err2"
	"github.com/lainio/err2/try"
	"github.com/pion/webrtc/v3"
	"github.com/shynome/wgortc/signaler"
	"golang.zx2c4.com/wireguard/conn"
)

type Inbound struct {
	baseEndpoint
	dc   *webrtc.DataChannel
	sess signaler.Session

	init *sync.Once
	err  error
	pc   *webrtc.PeerConnection
	ch   chan []byte
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
		init: &sync.Once{},
		ch:   make(chan []byte),
	}
}

func (ep *Inbound) Send(buf []byte) error {
	ep.init.Do(ep.HandleConnect)
	if ep.err != nil {
		return ep.err
	}
	return ep.dc.Send(buf)
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

func (ep *Inbound) HandleConnect() {
	defer err2.Catch(func(err error) {
		defer ep.sess.Reject(err)
		ep.err = err
	})

	pc := ep.pc
	try.To(pc.SetRemoteDescription(ep.sess.Description()))
	answer := try.To1(pc.CreateAnswer(nil))
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	try.To(pc.SetLocalDescription(answer))
	<-gatherComplete
	roffer := pc.LocalDescription()

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
}

func (ep *Inbound) Message() (ch <-chan []byte) {
	return ep.ch
}

var ErrInitiatorRequired = errors.New("first message initiator is required in webrtc sdp SessionInformation")
