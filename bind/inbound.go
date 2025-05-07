package bind

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/pion/webrtc/v4"
	"github.com/shynome/err0"
	"github.com/shynome/err0/try"
	"github.com/shynome/websocket"
	"github.com/shynome/websocket/wsjson"
	"github.com/shynome/wgortc/bind/nat"
	"github.com/shynome/wgortc/bind/whip"
	wgconn "golang.zx2c4.com/wireguard/conn"
)

var _ http.Handler = (*Bind)(nil)

func (b *Bind) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	defer err0.Then(&err, nil, func() {
		b.logger.Error("失败了", "error", err)
	})
	opts := &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
		Subprotocols:   []string{magicStr},
	}
	conn := try.To1(websocket.Accept(w, r, opts))
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()
	ctx, closeConn := context.WithCancelCause(ctx)
	defer closeConn(nil)

	var hinit HandshakeInitiation
	try.To(wsjson.Read(ctx, conn, &hinit))

	peer := b.config.GetPeer(hinit.Initiator, "")
	if peer == nil {
		conn.Close(WsStatusBadRequest, "WireGuard reject you")
		return
	}

	nowsc := peer.WsTransportDisabled()
	hresp := HandshakeResponse{
		WsTransportDisabled: nowsc,
	}
	try.To(wsjson.Write(ctx, conn, hresp))

	// init inbound
	inbound := &Inbound{}
	inbound.bind = b
	inbound.logger = b.logger.With("peer", peer.GetID())
	inbound.peer = peer
	inbound.nowsc.Store(peer.WsTransportDisabled())

	var ep wgconn.Endpoint = inbound
	if natc, ok := peer.(nat.INAT); ok {
		ep = nat.New(ep, natc)
	}

	inbound.logger.Info("websocket 连接成功")
	inbound.conn.Store(conn)
	defer func() {
		inbound.conn.Store(nil)
	}()

	if received := inbound.bind.Receive(ep, hinit.Initiator); !received {
		conn.Close(WsStatusServiceUnavailable, "wg device is not up")
		return
	}

	candidates := make(chan webrtc.ICECandidateInit, 1024)
	clientCandidates := make(chan webrtc.ICECandidateInit, 1024)
	offerCh := make(chan webrtc.SessionDescription)
	answerCh := make(chan webrtc.SessionDescription)
	signler := &serverSignaler{
		candidates:       candidates,
		clientCandidates: clientCandidates,
		offer:            offerCh,
		answer:           answerCh,
	}

	go func() (err error) {
		defer err0.Then(&err, nil, func() {
			conn.Close(websocket.StatusInvalidFramePayloadData, "failed to unmarshal JSON")
			closeConn(fmt.Errorf("read got error. %w", err))
		})
		defer func() {
			close(offerCh)
			close(clientCandidates)
		}()
		for {
			typ, msg := try.To2(conn.Read(ctx))
			switch typ {
			case websocket.MessageBinary:
				if nowsc && msg[0] == WireGuardMessageData {
					continue
				}
				inbound.bind.Receive(ep, msg)
			case websocket.MessageText:
				var payload whip.Payload[json.RawMessage]
				try.To(json.Unmarshal(msg, &payload))
				// logger.Debug("收到信息", "msg", string(msg))
				switch payload.Type {
				case whip.PyalodTypeOffer:
					var offer webrtc.SessionDescription
					try.To(json.Unmarshal(payload.Data, &offer))
					offerCh <- offer
				case whip.PyalodTypeCandidate:
					var cinit webrtc.ICECandidateInit
					try.To(json.Unmarshal(payload.Data, &cinit))
					clientCandidates <- cinit
				}
			}
		}
	}()
	go iter(ctx, candidates, func(c webrtc.ICECandidateInit) (err error) {
		defer err0.Then(&err, nil, nil)
		if c.SDPMid == nil {
			return nil
		}
		payload := whip.PayloadCandidate(c)
		s := try.To1(json.Marshal(payload))
		try.To(conn.Write(ctx, websocket.MessageText, s))
		return nil
	})
	go iter(ctx, answerCh, func(c webrtc.SessionDescription) (err error) {
		defer err0.Then(&err, nil, nil)
		payload := whip.PayloadAnswer(c)
		s := try.To1(json.Marshal(payload))
		try.To(conn.Write(ctx, websocket.MessageText, s))
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			return
		case offer, ok := <-signler.offer:
			if !ok {
				return
			}
			err := inbound.handshake(signler, offer)
			if err != nil {
				continue
			}
			return
		}
	}
}

type HandshakeInitiation struct {
	Initiator []byte `json:"initiator"`
}

type HandshakeResponse struct {
	WsTransportDisabled bool `json:"nowsc"`
}

type Inbound struct {
	Endpoint
}

var _ wgconn.Endpoint = (*Inbound)(nil)
var _ Sender = (*Endpoint)(nil)

func (ep *Inbound) handshake(signaler *serverSignaler, offer webrtc.SessionDescription) (err error) {
	defer err0.Then(&err, nil, nil)

	ep.logger.Debug("开始握手")
	pcinit := ep.peer.GetPeerInit()
	pc := try.To1(ep.bind.NewPeerConnection(pcinit))
	if pc := ep.pc.Swap(pc); pc != nil {
		pc.Close() // 关闭旧的
	}
	defer err0.Then(&err, nil, func() {
		pc.Close() // 出错的话关闭
	})

	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}
		select {
		case signaler.candidates <- i.ToJSON():
		default:
		}
	})

	ctx := context.Background()
	wctx, cause := context.WithCancelCause(ctx)
	defer cause(nil)
	pc.OnConnectionStateChange(func(pcs webrtc.PeerConnectionState) {
		switch pcs {
		case webrtc.PeerConnectionStateConnected:
		case webrtc.PeerConnectionStateFailed:
			cause(whip.ErrPCConnectionFailed)
		case webrtc.PeerConnectionStateClosed:
			cause(whip.ErrPCConnectionClosed)
		}
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != magicStr {
			dc.Close()
			return
		}
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			ep.bind.Receive(ep, msg.Data)
		})

		dc.OnOpen(func() {
			defer cause(nil)
			ep.dc.Store(dc)
		})
		dc.OnClose(func() {
			defer cause(net.ErrClosed)
			ep.dc.Store(nil)
		})
		// dc.OnError(func(err error) {
		// 	cause(net.ErrClosed)
		// 	sess.setStatus(whip.StatusDCFailed)
		// })
	})

	ep.logger.Debug("发送握手信息")
	defer err0.Then(&err, func() {
		ep.logger.Info("握手成功")
	}, func() {
		ep.logger.Error("握手失败", "error", err)
	})

	try.To(pc.SetRemoteDescription(offer))
	go iter(wctx, signaler.clientCandidates, func(c webrtc.ICECandidateInit) error {
		if err := pc.AddICECandidate(c); err != nil {
			ep.logger.Warn("add ice candidate failed", "error", err)
		} else {
			ep.logger.Debug("added ice candidate", "candidate", c)
		}
		return nil
	})

	answer := try.To1(pc.CreateAnswer(nil))
	signaler.answer <- answer
	try.To(pc.SetLocalDescription(answer))

	<-wctx.Done()
	if err := context.Cause(wctx); errors.Is(err, context.Canceled) {
		return nil
	} else {
		return err
	}
}

type serverSignaler struct {
	candidates       chan<- webrtc.ICECandidateInit
	clientCandidates <-chan webrtc.ICECandidateInit

	offer  <-chan webrtc.SessionDescription
	answer chan<- webrtc.SessionDescription
}
