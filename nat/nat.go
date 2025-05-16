package nat

import (
	"github.com/shynome/wgortc/bind/whip"
	"golang.zx2c4.com/wireguard/conn"
)

type Endpoint struct {
	whip.Endpoint
	INAT
}

func New(ep whip.Endpoint, natc INAT) conn.Endpoint {
	return &Endpoint{
		Endpoint: ep,
		INAT:     natc,
	}
}

var _ conn.Endpoint = (*Endpoint)(nil)
var _ INAT = (*Endpoint)(nil)

// NAT Instance
type INAT interface {
	NAT(buf []byte)
}
