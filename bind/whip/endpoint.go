package whip

import "golang.zx2c4.com/wireguard/conn"

type Sender interface {
	Send(buf []byte) error
}

type Endpoint interface {
	Sender
	conn.Endpoint
}
