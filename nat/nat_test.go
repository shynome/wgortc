package nat_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"testing"

	"github.com/pion/webrtc/v4"
	"github.com/shynome/err0/try"
	"github.com/shynome/wgortc/bind"
	"github.com/shynome/wgortc/device/logger"
	"github.com/shynome/wgortc/device/pubkey"
	"github.com/shynome/wgortc/nat"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

func TestMain(m *testing.M) {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	tdev, tnet := try.To2(netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("192.168.4.1")},
		[]netip.Addr{},
		bind.MTU,
	))
	c := &Config{
		Key: "4KKSeQMXqfT0SRV/f7LkPbWjpyjCS6IfBwr7gY2nr0M=",
		Peer: &Peer{
			Pubkey: "c5G1nU06IjrN6eWpVq6+mpYnVKfVTsi9o+gfzY+Fui4=",
			Allow:  "192.168.4.2/32",
		},
	}
	key := try.To1(base64.StdEncoding.DecodeString(c.Key))
	c.key = device.NoisePrivateKey(key)

	c.Peer.nat = nat.New()
	c.Peer.nat.SetNAT4(netip.MustParseAddr("192.168.4.2"), netip.MustParseAddr("192.168.4.1"))

	b := bind.New(c)
	b.SetName("server")
	logger := logger.New("server")
	dev := device.NewDevice(tdev, b, logger)
	defer dev.Close()

	{
		c := c.String()
		try.To(dev.IpcSet(c))
		try.To(dev.Up())
	}

	l := try.To1(net.Listen("tcp", "0.0.0.0:7788"))
	defer l.Close()
	go http.Serve(l, b)

	{
		l := try.To1(tnet.ListenTCP(&net.TCPAddr{Port: 80}))
		defer l.Close()
		s := http.NewServeMux()
		s.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, "ok")
		})
		go http.Serve(l, s)
	}

	m.Run()
}

func TestNAT(t *testing.T) {
	tdev, tnet := try.To2(netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("192.168.4.1")},
		[]netip.Addr{},
		bind.MTU,
	))

	c := &Config{
		Key: "IDHBZNpXkYmavc3JhCvCA9bTh6fo2IfB1D/F6mE6xXg=",
		Peer: &Peer{
			Pubkey:   "UwJ8NDnTdT/XM1VC8wPF7iu0GMP3FK81qRPSQlHQ7jU=",
			Allow:    "192.168.4.2/32",
			Endpoint: "ws://127.0.0.1:7788",
			Auto:     15,
		},
	}
	key := try.To1(base64.StdEncoding.DecodeString(c.Key))
	c.key = device.NoisePrivateKey(key)

	c.Peer.nat = nat.New()
	c.Peer.nat.SetNAT4(netip.MustParseAddr("192.168.4.2"), netip.MustParseAddr("192.168.4.1"))

	b := bind.New(c)
	b.SetName("client")
	logger := logger.New("client")
	dev := device.NewDevice(tdev, b, logger)
	defer dev.Close()

	{
		c := c.String()
		try.To(dev.IpcSet(c))
		try.To(dev.Up())
	}

	hc := &http.Client{
		Transport: &http.Transport{
			DialContext: tnet.DialContext,
		},
	}

	resp := try.To1(hc.Get("http://192.168.4.2"))
	body := try.To1(io.ReadAll(resp.Body))

	t.Log(body)
	if b := string(body); b != "ok" {
		t.Error("want ok, but got", b)
		return
	}
}

type Config struct {
	Key  string
	key  device.NoisePrivateKey
	Peer *Peer
}

var _ bind.Config = (*Config)(nil)

func (c *Config) GetPeer(initiator []byte, endpoint string) bind.Peer {
	if len(initiator) > 0 {
		pubkey, err := pubkey.Unpack(c.key, initiator)
		if err != nil {
			return nil
		}
		k := base64.StdEncoding.EncodeToString(pubkey[:])
		if k != c.Peer.Pubkey {
			return nil
		}
		return c.Peer
	}

	if c.Peer.Endpoint != endpoint {
		return nil
	}
	return c.Peer
}

type Peer struct {
	Pubkey   string
	Endpoint string
	Allow    string
	Auto     int
	nat      *nat.NATC
}

var _ bind.Peer = (*Peer)(nil)
var _ nat.INAT = (*Peer)(nil)

func (c *Peer) GetID() string {
	return c.Pubkey
}

func (p *Peer) GetPeerInit() webrtc.Configuration {
	return webrtc.Configuration{}
}

func (p *Peer) NAT(buf []byte) {
	p.nat.NAT(buf)
	return
}

func (c *Config) String() string {
	var b = new(bytes.Buffer)
	fmt.Fprintf(b, "private_key=%s\n", Base64ToHex(c.Key))
	b.WriteString(c.Peer.String())
	return b.String()
}

func (p *Peer) String() string {
	var b = new(bytes.Buffer)
	fmt.Fprintf(b, "public_key=%s\n", Base64ToHex(p.Pubkey))

	fmt.Fprintf(b, "replace_allowed_ips=true\n")
	if p.Allow != "" {
		fmt.Fprintf(b, "allowed_ip=%s\n", p.Allow)
	}

	ep := p.Endpoint
	if ep != "" {
		fmt.Fprintf(b, "endpoint=%s\n", ep)
	}

	t := p.Auto
	// 只有存在 endpoint 时 auto 配置才有意义
	if ep == "" {
		t = 0
	}
	fmt.Fprintf(b, "persistent_keepalive_interval=%d\n", t)

	return b.String()
}

func Base64ToHex(s string) string {
	b := try.To1(base64.StdEncoding.DecodeString(s))
	return hex.EncodeToString(b)
}
