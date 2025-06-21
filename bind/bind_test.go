package bind_test

import (
	"encoding/hex"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/shynome/err0/try"
	"github.com/shynome/websocket"
	"github.com/shynome/wgortc/bind"
	"github.com/shynome/wgortc/device/logger"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

var peer = &Peer{id: "p2"}
var serverNet *netstack.Net

func TestMain(m *testing.M) {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	tdev, tnet := try.To2(netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("192.168.7.1")},
		[]netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("8.8.4.4")},
		bind.MTU,
	))
	serverNet = tnet
	bind := bind.New(&Config{peer: peer})
	bind.SetName("server")
	logger := logger.New("server")
	dev := device.NewDevice(tdev, bind, logger)
	try.To(dev.IpcSet(p1cfg))
	try.To(dev.Up())
	defer dev.Close()

	{
		srv := http.NewServeMux()
		srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, testCheckResponseText)
		})
		l := try.To1(tnet.ListenTCP(&net.TCPAddr{Port: 80}))
		defer l.Close()
		go http.Serve(l, srv)
	}

	{
		l := try.To1(net.Listen("tcp", ":7788"))
		go http.Serve(l, bind)
		time.Sleep(time.Second)
	}

	m.Run()
}

const testCheckResponseText = "Hello from userspace TCP!"

func TestClient(t *testing.T) {
	peer.tm = 0
	testClient(t)
	peer.tm = bind.WSTransportDisabled
	testClient(t)
	peer.tm = bind.WebRTCTransportDisabled
	testClient(t)
}

func testClient(t *testing.T) {
	tdev, tnet := try.To2(netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("192.168.7.2")},
		[]netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("8.8.4.4")},
		bind.MTU,
	))
	bind := bind.New(&Config{peer: &Peer{id: "p1"}})
	bind.SetName("client")
	logger := logger.New("client")
	dev := device.NewDevice(tdev, bind, logger)
	try.To(dev.IpcSet(p2cfg))
	try.To(dev.Up())
	defer dev.Close()

	client := &http.Client{
		Transport: &http.Transport{DialContext: tnet.DialContext},
		Timeout:   30 * time.Second,
	}
	resp := try.To1(client.Get("http://192.168.7.1/"))
	body := try.To1(io.ReadAll(resp.Body))

	if body := string(body); body != testCheckResponseText {
		t.Error(body)
	}
}

func TestWebSocketTemporaryRedirect(t *testing.T) {
	{
		l := try.To1(net.Listen("tcp", "127.0.0.1:7789"))
		defer l.Close()
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, bind.WsAcceptOptions)
			if err != nil {
				t.Error("websocket 连接失败", err)
				return
			}
			err = conn.Close(bind.WsStatusTemporaryRedirect, "ws://127.0.0.1:7788")
			if err != nil {
				t.Error(err)
			}
		})
		go http.Serve(l, h)
	}

	tdev, tnet := try.To2(netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("192.168.7.2")},
		[]netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("8.8.4.4")},
		bind.MTU,
	))
	p := &Peer{id: "p1"}
	p.tm = bind.WSRedirectEnabled
	bind := bind.New(&Config{peer: p})
	bind.SetName("client")
	logger := logger.New("client")
	dev := device.NewDevice(tdev, bind, logger)
	try.To(dev.IpcSet(p22cfg))
	try.To(dev.Up())
	defer dev.Close()

	client := &http.Client{
		Transport: &http.Transport{DialContext: tnet.DialContext},
		Timeout:   30 * time.Second,
	}
	resp := try.To1(client.Get("http://192.168.7.1/"))
	body := try.To1(io.ReadAll(resp.Body))

	if body := string(body); body != testCheckResponseText {
		t.Error(body)
	}
}

func TestWebSocketPermanentRedirect(t *testing.T) {
	{
		l := try.To1(net.Listen("tcp", "127.0.0.1:7789"))
		defer l.Close()
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, bind.WsAcceptOptions)
			if err != nil {
				t.Error("websocket 连接失败", err)
				return
			}
			err = conn.Close(bind.WsStatusPermanentRedirect, "ws://127.0.0.1:7788")
			if err != nil {
				t.Error(err)
			}
		})
		go http.Serve(l, h)
	}

	tdev, tnet := try.To2(netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("192.168.7.2")},
		[]netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("8.8.4.4")},
		bind.MTU,
	))
	linkCh := make(chan string)
	p := &Peer{id: "p1"}
	p.redirected = func(link string, lc int) {
		linkCh <- link
	}
	p.tm = bind.WSRedirectEnabled
	bind := bind.New(&Config{peer: p})
	bind.SetName("client")
	logger := logger.New("client")
	dev := device.NewDevice(tdev, bind, logger)
	try.To(dev.IpcSet(p22cfg))
	try.To(dev.Up())
	defer dev.Close()

	client := &http.Client{
		Transport: &http.Transport{DialContext: tnet.DialContext},
		Timeout:   30 * time.Second,
	}
	resp := try.To1(client.Get("http://192.168.7.1/"))
	body := try.To1(io.ReadAll(resp.Body))

	if body := string(body); body != testCheckResponseText {
		t.Error(body)
	}

	nl := <-linkCh
	if nl != "ws://127.0.0.1:7788" {
		t.Errorf("want %s, got %s", "ws://127.0.0.1:7788", nl)
		return
	}

	{
		pubkey, _ := hex.DecodeString("53027c3439d3753fd7335542f303c5ee2bb418c3f714af35a913d24251d0ee35")
		p := dev.LookupPeer(device.NoisePublicKey(pubkey))
		p.ExpireCurrentKeypairs()
	}

	{
		resp := try.To1(client.Get("http://192.168.7.1/"))
		body := try.To1(io.ReadAll(resp.Body))

		if body := string(body); body != testCheckResponseText {
			t.Error(body)
		}
	}
}

func TestClient3(t *testing.T) {
	tdev, tnet := try.To2(netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("192.168.7.2")},
		[]netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("8.8.4.4")},
		bind.MTU,
	))
	bind := bind.New(&Config{peer: &Peer{id: "p1"}})
	bind.SetName("client")
	logger := logger.New("client")
	dev := device.NewDevice(tdev, bind, logger)
	try.To(dev.IpcSet(p3cfg))
	try.To(dev.Up())
	defer dev.Close()

	client := &http.Client{
		Transport: &http.Transport{DialContext: tnet.DialContext},
		Timeout:   30 * time.Second,
	}

	{
		resp := try.To1(client.Get("http://192.168.7.1/"))
		body := try.To1(io.ReadAll(resp.Body))

		if body := string(body); body != testCheckResponseText {
			t.Error(body)
		}
	}

	{
		pubkey, _ := hex.DecodeString("53027c3439d3753fd7335542f303c5ee2bb418c3f714af35a913d24251d0ee35")
		p := dev.LookupPeer(device.NoisePublicKey(pubkey))
		p.ExpireCurrentKeypairs()
	}

	{
		resp := try.To1(client.Get("http://192.168.7.1/"))
		body := try.To1(io.ReadAll(resp.Body))

		if body := string(body); body != testCheckResponseText {
			t.Error(body)
		}
	}

}

// key: 4KKSeQMXqfT0SRV/f7LkPbWjpyjCS6IfBwr7gY2nr0M=
// key(hex): e0a292790317a9f4f449157f7fb2e43db5a3a728c24ba21f070afb818da7af43
// pubkey: UwJ8NDnTdT/XM1VC8wPF7iu0GMP3FK81qRPSQlHQ7jU=
// pubkey(hex): 53027c3439d3753fd7335542f303c5ee2bb418c3f714af35a913d24251d0ee35
var p1cfg = `private_key=e0a292790317a9f4f449157f7fb2e43db5a3a728c24ba21f070afb818da7af43
listen_port=7777
public_key=7391b59d4d3a223acde9e5a956aebe9a962754a7d54ec8bda3e81fcd8f85ba2e
allowed_ip=192.168.7.2/32`

// key: IDHBZNpXkYmavc3JhCvCA9bTh6fo2IfB1D/F6mE6xXg=
// key(hex): 2031c164da5791899abdcdc9842bc203d6d387a7e8d887c1d43fc5ea613ac578
// pubkey: c5G1nU06IjrN6eWpVq6+mpYnVKfVTsi9o+gfzY+Fui4=
// pubkey(hex): 7391b59d4d3a223acde9e5a956aebe9a962754a7d54ec8bda3e81fcd8f85ba2e
var p2cfg = `private_key=2031c164da5791899abdcdc9842bc203d6d387a7e8d887c1d43fc5ea613ac578
public_key=53027c3439d3753fd7335542f303c5ee2bb418c3f714af35a913d24251d0ee35
endpoint=ws://127.0.0.1:7788
allowed_ip=192.168.7.1/32`

var p22cfg = `private_key=2031c164da5791899abdcdc9842bc203d6d387a7e8d887c1d43fc5ea613ac578
public_key=53027c3439d3753fd7335542f303c5ee2bb418c3f714af35a913d24251d0ee35
endpoint=ws://127.0.0.1:7789
allowed_ip=192.168.7.1/32`

var p3cfg = `private_key=2031c164da5791899abdcdc9842bc203d6d387a7e8d887c1d43fc5ea613ac578
public_key=53027c3439d3753fd7335542f303c5ee2bb418c3f714af35a913d24251d0ee35
endpoint=["ws://127.0.0.1:7789","ws://127.0.0.1:7788"]
allowed_ip=192.168.7.1/32`

type Config struct {
	peer *Peer
}

var _ bind.Config = (*Config)(nil)

func (c *Config) GetPeer(initiator []byte, endpoint string) bind.Peer { return c.peer }

type Peer struct {
	id     string
	pcinit webrtc.Configuration
	tm     bind.TransportMode

	redirected func(link string, lc int)
}

var _ bind.Peer = (*Peer)(nil)
var _ bind.PeerMode = (*Peer)(nil)
var _ bind.PeerEndpiontRedirected = (*Peer)(nil)

func (p *Peer) GetPeerInit() webrtc.Configuration { return p.pcinit }
func (p *Peer) GetID() string                     { return p.id }
func (p *Peer) TransportMode() bind.TransportMode { return p.tm }

func (p *Peer) EndpiontRedirected(link string, lc int) {
	if p.redirected != nil {
		p.redirected(link, lc)
	}
}
