package bind

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync/atomic"

	"github.com/pion/webrtc/v4"
	"github.com/shynome/err0"
	"golang.zx2c4.com/wireguard/conn"
)

type Bind struct {
	config Config
	msgs   chan Packet
	name   string // logger prefix
	logger *slog.Logger
	api    atomic.Pointer[webrtc.API]
	cancel context.CancelFunc
}

var _ conn.Bind = (*Bind)(nil)

type Config interface {
	GetPeer(initiator []byte, endpoint string) Peer
}

type Peer interface {
	GetID() string // 用以辨别节点
	GetPeerInit() webrtc.Configuration
	WsTransportDisabled() bool // webrtc data channel 连通性调试用
}

func New(config Config) *Bind {
	b := &Bind{
		config: config,
		name:   magicStr,
		msgs:   make(chan Packet),
	}
	b.SetName(b.name)
	return b
}

func (b *Bind) SetName(name string) {
	b.name = name
	b.logger = slog.With("WireGuardBind", name)
}

func (b *Bind) Open(port uint16) (fns []conn.ReceiveFunc, actualPort uint16, err error) {
	defer err0.Then(&err, nil, nil)
	eg := webrtc.SettingEngine{}
	ctx := context.Background()
	ctx, b.cancel = context.WithCancel(ctx)
	if listenTry != nil {
		actualPort = listenTry(ctx, &eg, int(port))
	}
	api := webrtc.NewAPI(webrtc.WithSettingEngine(eg))
	b.api.Swap(api)
	fns = []conn.ReceiveFunc{b.makeReceiveFunc(ctx)}
	return
}

type bindPower interface {
	Receive(ep conn.Endpoint, buf []byte) bool
	NewPeerConnection(cfg webrtc.Configuration) (*webrtc.PeerConnection, error)
}

var _ bindPower = (*Bind)(nil)

func (b *Bind) Receive(ep conn.Endpoint, buf []byte) bool {
	select {
	case b.msgs <- Packet{data: buf, ep: ep}:
		return true
	default:
		return false
	}
}

func (b *Bind) NewPeerConnection(cfg webrtc.Configuration) (*webrtc.PeerConnection, error) {
	if api := b.api.Load(); api == nil {
		return nil, fmt.Errorf("wg device is not ready")
	} else {
		return api.NewPeerConnection(cfg)
	}
}

func (b *Bind) makeReceiveFunc(ctx context.Context) conn.ReceiveFunc {
	batch := b.BatchSize()
	return func(packets [][]byte, sizes []int, eps []conn.Endpoint) (n int, err error) {
		for i := range batch {
			select {
			case <-ctx.Done():
				return 0, net.ErrClosed
			case msg := <-b.msgs:
				eps[i] = msg.ep
				sizes[i] = copy(packets[i], msg.data)
				n += 1
			}
		}
		return
	}
}

func (b *Bind) Close() error {
	if cancel := b.cancel; cancel != nil {
		cancel()
	}
	return nil
}

func (*Bind) Send(bufs [][]byte, ep conn.Endpoint) error {
	sender, ok := ep.(Sender)
	if !ok {
		return ErrEndpointImpl
	}
	for _, buf := range bufs {
		err := sender.Send(buf)
		if err != nil {
			return err
		}
	}
	return nil
}

var ErrEndpointImpl = errors.New("endpoint is not wgortc.Endpoint")

const MTU = 2400 - 80
const BatchSize = 1

func (*Bind) SetMark(mark uint32) error { return nil }
func (*Bind) BatchSize() int            { return BatchSize }

type Packet struct {
	data []byte
	ep   conn.Endpoint
}
