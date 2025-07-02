package bind

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/shynome/err0"
	"github.com/shynome/err0/try"
	"github.com/shynome/websocket"
	"github.com/shynome/websocket/wsjson"
	"github.com/shynome/wgortc/bind/browser"
	"github.com/shynome/wgortc/bind/whip"
	"github.com/shynome/wgortc/device/pubkey"
	"github.com/shynome/wgortc/nat"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
)

func (b *Bind) ParseEndpoint(s string) (conn.Endpoint, error) {
	peer := b.config.GetPeer(nil, s)
	if peer == nil {
		return nil, fmt.Errorf("can't find peer by %s", s)
	}

	// init outbound
	outbound := &Outbound{}
	if !strings.HasPrefix(s, "[") {
		outbound.links = []string{s}
	} else {
		if err := json.Unmarshal([]byte(s), &outbound.links); err != nil {
			return nil, err
		}
	}
	outbound.bind = b
	outbound.logger = b.logger.With("peer", peer.GetID()).With("endpoint", outbound.links)
	outbound.peer = peer
	outbound.ping = make(chan string)
	outbound.pong = make(chan string)

	outbound.INAT = nat.Empty{}
	if natc, ok := peer.(nat.INAT); ok {
		outbound.INAT = natc
	}

	return outbound, nil
}

type Outbound struct {
	Endpoint
	nat.INAT
	lc    int //link cursor
	links []string

	connecting atomic.Bool
	wantClear  atomic.Bool
	ping       chan string
	pong       chan string
}

var _ conn.Endpoint = (*Outbound)(nil)
var _ whip.Sender = (*Outbound)(nil)
var _ nat.INAT = (*Outbound)(nil)

func (ep *Outbound) Send(buf []byte) error {
	if connecting := ep.connecting.Load(); connecting {
		ep.wantClear.Store(false)
	}
	if buf[0] == WireGuardMessageData && len(buf) == device.MessageTransportSize {
		select {
		case ep.ping <- "ping":
		default:
		}
	}
	err := ep.Endpoint.Send(buf)
	if err == nil {
		return nil
	}
	if err == ErrNoDataChannel && buf[0] == WireGuardMessageInitiator {
		// 握手过程有点耗时
		go ep.connect(buf)
		return nil
	}
	return err
}

func (ep *Outbound) ClearSrc() {
	if connecting := ep.connecting.Load(); connecting {
		ep.wantClear.Store(true)
		return
	}
	ep.wantClear.Store(false)
	ep.Endpoint.ClearSrc()
}

func (ep *Outbound) connect(buf []byte) (err error) {
	if connecting := ep.connecting.Load(); connecting {
		return
	}
	ep.connecting.Store(true)

	defer err0.Then(&err, func() {
		ep.lc = 0 // 连接成功后将lc重置为0, 还是以第一个endpoint为主, 剩余的作为辅助
	}, func() {
		ep.logger.Error("connect failed", "error", err, "lc", ep.lc)
		lc := (ep.lc + 1) % len(ep.links)
		ep.lc = lc
	})

	ctx := context.Background()
	ctx, p2p_connected := context.WithCancelCause(ctx)
	go func() {
		<-ctx.Done()
		ep.connecting.Store(false)
		if wantClear := ep.wantClear.Load(); wantClear {
			ep.ClearSrc()
		}
	}()
	defer err0.Then(&err, nil, func() {
		p2p_connected(err)
	})

	t := time.AfterFunc(pubkey.WebRTCConnectTimeout, func() {
		p2p_connected(context.DeadlineExceeded)
	})
	defer t.Stop()

	var hinit = HandshakeInitiation{
		Initiator: buf,
	}
	var hresp HandshakeResponse

	var tm TransportMode
	if m, ok := ep.peer.(PeerMode); ok {
		tm = m.TransportMode()
	}

	var conn *websocket.Conn
	link := ep.links[ep.lc]
	for {
		opts := websocket.DialOptions{
			Subprotocols: []string{magicStr},
		}
		srv, auth := try.To2(browser.SplitAuth(link))
		if auth != nil {
			if uname := auth.Username(); uname != "" {
				opts.Subprotocols = append(opts.Subprotocols, uname)
			}
			if pass, _ := auth.Password(); pass != "" {
				opts.Subprotocols = append(opts.Subprotocols, pass)
			}
		}
		conn, _ = try.To2(websocket.Dial(ctx, srv, &opts))
		wsjson.Write(ctx, conn, hinit) // 虽然这里也有可能出错, 但忽略它不影响下方的出错, 这样只处理一个出错点更简单
		err2 := wsjson.Read(ctx, conn, &hresp)
		if err2 == nil {
			break
		}
		conn.CloseNow()
		var e websocket.CloseError
		if errors.As(err2, &e) {
			redirectEnabled := tm&WSRedirectEnabled != 0
			if redirectEnabled {
				switch e.Code {
				case WsStatusTemporaryRedirect:
					link = e.Reason
					continue
				case WsStatusPermanentRedirect:
					link = e.Reason
					defer err0.Then(&err, func() {
						ep.links[ep.lc] = link
						ep.logger = ep.logger.With("endpoint", ep.links)
						if h, ok := ep.peer.(PeerEndpiontRedirected); ok {
							h.EndpiontRedirected(link, ep.lc)
						}
					}, nil)
					continue
				}
			}
		}
		try.To(err2)
	}
	t.Stop()

	ep.mode.Store(hresp.TransportMode)

	candidates := make(chan webrtc.ICECandidateInit, 1024)
	serverCandidates := make(chan webrtc.ICECandidateInit, 1024)
	offerCh := make(chan webrtc.SessionDescription)
	answerCh := make(chan webrtc.SessionDescription)
	signaler := &clientSignaler{
		candidates:       candidates,
		serverCandidates: serverCandidates,
		offer:            offerCh,
		answer:           answerCh,
	}

	go func() (err error) {
		defer err0.Then(&err, nil, func() {
			conn.Close(websocket.StatusInvalidFramePayloadData, "failed to unmarshal JSON")
			err = fmt.Errorf("read got error. %w", err)
			if !errors.Is(err, context.Canceled) {
				ep.logger.Error("websocket 连接失败", "error", err)
			}
			p2p_connected(err)
		})
		defer func() {
			close(answerCh)
			close(serverCandidates)
		}()
		for {
			typ, msg := try.To2(conn.Read(ctx))
			switch typ {
			case websocket.MessageBinary:
				ep.bind.Receive(ep, msg)
			case websocket.MessageText:
				var payload whip.Payload[json.RawMessage]
				try.To(json.Unmarshal(msg, &payload))
				switch payload.Type {
				case whip.PyalodTypeAnswer:
					var answer webrtc.SessionDescription
					try.To(json.Unmarshal(payload.Data, &answer))
					answerCh <- answer
				case whip.PyalodTypeCandidate:
					var cinit webrtc.ICECandidateInit
					try.To(json.Unmarshal(payload.Data, &cinit))
					serverCandidates <- cinit
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
	go iter(ctx, offerCh, func(c webrtc.SessionDescription) (err error) {
		defer err0.Then(&err, nil, nil)
		payload := whip.PayloadOffer(c)
		s := try.To1(json.Marshal(payload))
		try.To(conn.Write(ctx, websocket.MessageText, s))
		return nil
	})

	ep.logger.Info("websocket 连接成功")
	ep.conn.Store(conn)

	handshake := func() {
		defer p2p_connected(nil)
		defer func() {
			ep.conn.Store(nil)
		}()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				err := ep.handshake(signaler)
				if err != nil {
					time.Sleep(time.Second)
					continue
				}
				return
			}
		}
	}

	go handshake()
	return nil
}

func (ep *Outbound) handshake(signaler *clientSignaler) (err error) {
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
				case <-signaler.serverCandidates:
					// 任何事都不做, 只是避免阻塞
				case <-signaler.answer:
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
	dcinit := webrtc.DataChannelInit{
		Ordered:        ref(false),
		MaxRetransmits: ref[uint16](0),
	}
	dc := try.To1(pc.CreateDataChannel(magicStr, &dcinit))
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if msg.IsString {
			select {
			case ep.pong <- string(msg.Data):
			default:
			}
			return
		}
		ep.bind.Receive(ep, msg.Data)
	})
	if pc := ep.pc.Swap(pc); pc != nil {
		pc.Close()
	}
	defer err0.Then(&err, nil, func() {
		pc.Close()
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
		case webrtc.PeerConnectionStateClosed:
			cause(whip.ErrPCConnectionClosed)
		case webrtc.PeerConnectionStateFailed:
			cause(whip.ErrPCConnectionFailed)
		}
	})

	dc.OnOpen(func() {
		defer cause(nil)
		ep.dc.Store(dc)
		go func() {
			defer ep.dc.Store(nil)
			defer dc.Close()
			for {
				select {
				case <-ctx.Done():
					return
				case ping := <-ep.ping:
					if err := dc.SendText(ping); err != nil {
						return
					}
					if exit := func() (exit bool) {
						ctx, cancel := context.WithTimeout(ctx, device.RekeyTimeout)
						defer cancel()
						select {
						case <-ctx.Done():
							return true
						case <-ep.pong:
							return false
						}
					}(); exit {
						return
					}
				}
			}
		}()
	})
	dc.OnClose(func() {
		defer cause(net.ErrClosed)
		ep.dc.Store(nil)
	})

	ep.logger.Debug("webrtc 发送握手信息")
	defer err0.Then(&err, func() {
		ep.logger.Info("webrtc 握手成功")
	}, func() {
		ep.logger.Info("webrtc 握手失败", "error", err)
	})

	offer := try.To1(pc.CreateOffer(nil))
	try.To(pc.SetLocalDescription(offer))
	signaler.offer <- offer
	answer, ok := <-signaler.answer
	if !ok {
		return fmt.Errorf("signler is closed")
	}
	try.To(pc.SetRemoteDescription(answer))
	go iter(wctx, signaler.serverCandidates, func(c webrtc.ICECandidateInit) error {
		if err := pc.AddICECandidate(c); err != nil {
			ep.logger.Warn("add ice candidate failed", "error", err)
		} else {
			ep.logger.Debug("added ice candidate", "candidate", c)
		}
		return nil
	})

	<-wctx.Done()
	if err := context.Cause(wctx); errors.Is(err, context.Canceled) {
		return nil
	} else {
		return err
	}
}

type clientSignaler struct {
	candidates       chan<- webrtc.ICECandidateInit
	serverCandidates <-chan webrtc.ICECandidateInit

	offer  chan<- webrtc.SessionDescription
	answer <-chan webrtc.SessionDescription
}
