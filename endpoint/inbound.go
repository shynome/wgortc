//go:build ierr

package endpoint

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"time"

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

func (ep *Inbound) ExtractInitiator() (initiator []byte, ierr error) {
	offer := ep.sess.Description()
	sdp, ierr := offer.Unmarshal()
	rawStr := sdp.SessionInformation
	if rawStr == nil {
		return nil, ErrInitiatorRequired
	}
	initiator, ierr = base64.StdEncoding.DecodeString(string(*rawStr))
	return initiator, nil
}

func (ep *Inbound) HandleConnect(buf []byte) (ierr error) {
	defer then(&ierr, nil, func() {
		ep.sess.Reject(ierr)
	})

	pc := ep.pc
	pc.OnConnectionStateChange(func(pcs webrtc.PeerConnectionState) {
		switch pcs {
		case webrtc.PeerConnectionStateDisconnected:
			pc.Close()
		}
	})

	ierr = pc.SetRemoteDescription(ep.sess.Description())
	answer, ierr := pc.CreateAnswer(nil)
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	ierr = pc.SetLocalDescription(answer)
	<-gatherComplete
	roffer := pc.LocalDescription()

	responder := sdp.Information(base64.StdEncoding.EncodeToString(buf))
	sdp, ierr := roffer.Unmarshal()
	sdp.SessionInformation = &responder
	rsdp, ierr := sdp.Marshal()
	roffer.SDP = string(rsdp)

	ierr = ep.sess.Resolve(roffer)

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
		ierr = err
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
