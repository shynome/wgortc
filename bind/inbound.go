package bind

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/shynome/err0"
	"github.com/shynome/err0/try"
	"github.com/shynome/websocket"
	"github.com/shynome/websocket/wsjson"
	"github.com/shynome/wgortc/bind/whip"
	"github.com/shynome/wgortc/nat"
	wgconn "golang.zx2c4.com/wireguard/conn"
)

var _ http.Handler = (*Bind)(nil)

var WsAcceptOptions = &websocket.AcceptOptions{
	OriginPatterns: []string{"*"},
	Subprotocols:   []string{magicStr},
}

func (b *Bind) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	defer err0.Then(&err, nil, func() {
		b.logger.Error("失败了", "error", err)
	})
	conn := try.To1(websocket.Accept(w, r, WsAcceptOptions))
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

	var tm TransportMode = 0
	if peer, ok := peer.(PeerMode); ok {
		tm = peer.TransportMode()
	}

	if tm&AllTransportDisabled == AllTransportDisabled {
		conn.Close(WsStatusBadRequest, "All transports are disabled")
		return
	}

	hresp := HandshakeResponse{
		TransportMode: uint32(tm),
	}
	if peer, ok := peer.(PeerHandshakeHook); ok {
		peer.HandshakeInitiationHook(&hinit)
		peer.HandshakeResponseHook(&hresp)
	}
	try.To(wsjson.Write(ctx, conn, hresp))

	// init inbound
	inbound := &Inbound{}
	inbound.bind = b
	inbound.logger = b.logger.With("peer", peer.GetID())
	inbound.peer = peer
	inbound.mode.Store(uint32(tm))
	if p, ok := peer.(PeerPubkey); ok {
		inbound.pubkey = p.GetPubkey()
	}

	inbound.INAT = nat.Empty{}
	if natc, ok := peer.(nat.INAT); ok {
		inbound.INAT = natc
	}

	inbound.logger.Info("websocket 连接成功")
	inbound.conn.Store(conn)
	defer func() {
		inbound.conn.Store(nil)
	}()

	if received := inbound.receive(hinit.Initiator); !received {
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
		nowsc := tm&WSTransportDisabled != 0
		for {
			typ, msg := try.To2(conn.Read(ctx))
			switch typ {
			case websocket.MessageBinary:
				if nowsc && msg[0] == WireGuardMessageData {
					continue
				}
				inbound.receive(msg)
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

	Extra json.RawMessage `json:"extra"`
}

type HandshakeResponse struct {
	TransportMode uint32 `json:"transport_mode"`

	Extra json.RawMessage `json:"extra"`
}

type Inbound struct {
	Endpoint
	nat.INAT
}

var _ wgconn.Endpoint = (*Inbound)(nil)
var _ whip.Sender = (*Inbound)(nil)
var _ nat.INAT = (*Inbound)(nil)

func (ep *Inbound) handshake(signaler *serverSignaler, offer webrtc.SessionDescription) (err error) {
	defer err0.Then(&err, nil, nil)

	tm := TransportMode(ep.mode.Load())
	nowrtc := tm&WebRTCTransportDisabled != 0
	if nowrtc {
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-signaler.clientCandidates:
					// 任何事都不做, 只是避免阻塞
				}
			}
		}()
		<-ctx.Done()
		return ErrWebRTCDisabled
	}

	ep.logger.Debug("webrtc 开始握手")
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
			if msg.IsString {
				dc.SendText("pong")
				return
			}
			ep.receive(msg.Data)
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

	ep.logger.Debug("webrtc 发送握手信息")
	defer err0.Then(&err, func() {
		ep.logger.Info("webrtc 握手成功")
	}, func() {
		ep.logger.Error("webrtc 握手失败", "error", err)
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
