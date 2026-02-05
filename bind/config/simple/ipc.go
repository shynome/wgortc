package simple

import (
	"bytes"
	"fmt"
	"net/netip"
)

func (c *Config) IpcConfig() string {
	var b = new(bytes.Buffer)
	fmt.Fprintf(b, "private_key=%s\n", Base64ToHex(c.Key))
	for _, p := range c.peers {
		b.WriteString(p.String())
	}
	return b.String()
}

const defaultPSK = "0000000000000000000000000000000000000000000000000000000000000000"

func (p *Peer) String() string {
	var b = new(bytes.Buffer)
	fmt.Fprintf(b, "public_key=%s\n", Base64ToHex(p.Pubkey))

	psk := p.PSK
	if psk == "" {
		psk = defaultPSK
	}
	fmt.Fprintf(b, "preshared_key=%s\n", Base64ToHex(psk))

	fmt.Fprintf(b, "replace_allowed_ips=true\n")
	if p.Allow != "" {
		fmt.Fprintf(b, "allowed_ip=%s\n", p.Allow)
		if allow, err := netip.ParsePrefix(p.Allow); err == nil {
			addr := allow.Addr()
			fmt.Fprintf(b, "allowed_ip=2001:00f4::%s/128\n", addr.String())
		}
	}
	if p.Allow6 != "" {
		fmt.Fprintf(b, "allowed_ip=%s\n", p.Allow6)
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
