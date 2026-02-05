package simple

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"sync"

	"github.com/pion/webrtc/v4"
	"github.com/shynome/err0"
	"github.com/shynome/err0/try"
	"github.com/shynome/wgortc/bind"
	"github.com/shynome/wgortc/bind/whip"
	"github.com/shynome/wgortc/device/pubkey"
	"github.com/shynome/wgortc/nat"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type Config struct {
	Key      string
	NAT      string
	NAT6     string
	ICE      []string
	Peer     string
	Peers    []*Peer
	LogLevel slog.Level

	key   device.NoisePrivateKey
	peers []*Peer
	ices  []webrtc.ICEServer
	init  func() error
}

type Peer struct {
	nat.INAT

	Pubkey   string
	PSK      string
	Endpoint string
	Auto     int
	Allow    string
	Allow6   string

	root *Config
}

var _ bind.Config = (*Config)(nil)

func (c *Config) Normalize() error {
	if c.init != nil {
		return c.init()
	}
	c.init = sync.OnceValue(c.normalize)
	return c.init()
}

func (c *Config) normalize() (err error) {
	defer err0.Then(&err, nil, nil)

	if c.Key != "" {
		key := try.To1(wgtypes.ParseKey(c.Key))
		c.key = device.NoisePrivateKey(key)
	}

	if c.NAT == "" {
		c.NAT = "192.168.211.1/20"
	}
	if c.NAT6 == "" {
		c.NAT6 = "2001:00f0::/28"
	}

	pf := try.To1(netip.ParsePrefix(c.NAT))
	dst := pf.Addr()
	dst6 := try.To1(netip.ParsePrefix(c.NAT6)).Addr()
	peers := []*Peer{}
	if c.Peer != "" {
		u := try.To1(url.Parse(c.Peer))
		pubkey := u.Fragment
		u.Fragment = ""
		allow := netip.PrefixFrom(pf.Addr().Next(), 32)
		p := &Peer{
			Pubkey:   pubkey,
			Endpoint: u.String(),
			Auto:     10,
			Allow:    allow.String(),
		}
		peers = append(peers, p)
	}
	peers = append(peers, c.Peers...)
	for _, p := range peers {
		p.root = c
		n := nat.New()
		p.INAT = n
		allow6 := p.Allow6
		if p.Allow != "" {
			allow := try.To1(netip.ParsePrefix(p.Allow))
			n.SetNAT4(allow.Addr(), dst)
			if allow6 == "" {
				allow6 = fmt.Sprintf("2001:00f4::%s/128", allow.Addr().String())
			}
		}
		if allow6 != "" {
			allow := try.To1(netip.ParsePrefix(allow6))
			n.SetNAT6(allow.Addr(), dst6)
		}
		c.peers = append(c.peers, p)
	}

	for _, ice := range c.ICE {
		ice := whip.ParseICEServer(ice)
		c.ices = append(c.ices, ice)
	}

	c.peers = peers
	return
}

func (c *Config) Pubkey() wgtypes.Key {
	key := wgtypes.Key(c.key)
	return key.PublicKey()
}

func (c *Config) GetPeer(initiator []byte, endpoint string) (p bind.Peer) {
	if len(initiator) != 0 {
		pubkey, err := pubkey.Unpack(c.key, initiator)
		if err != nil {
			slog.Warn("can't unpack initiator", "error", err)
			return nil
		}
		pk := base64.StdEncoding.EncodeToString(pubkey[:])
		for _, p := range c.peers {
			if p.Pubkey == pk {
				return p
			}
		}
		return nil
	}
	for _, p := range c.peers {
		if p.Endpoint == endpoint {
			return p
		}
	}
	return nil
}

var _ nat.INAT = (*Peer)(nil)
var _ bind.Peer = (*Peer)(nil)
var _ bind.PeerMode = (*Peer)(nil)

func (p *Peer) GetID() string {
	return p.Pubkey
}

func (p *Peer) GetPubkey() device.NoisePublicKey {
	pubkey := decodeBase64(p.Pubkey)
	return device.NoisePublicKey(pubkey)
}

func (p *Peer) GetPeerInit() webrtc.Configuration {
	return webrtc.Configuration{
		ICEServers: p.root.ices,
	}
}

func (p *Peer) TransportMode() bind.TransportMode {
	return bind.WSRedirectEnabled
}
