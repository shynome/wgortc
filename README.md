# wgortc (Wireguard Over Webrtc)

## How to Use

replace `conn.Bind` with this. more details see [example/main.go](./example/main.go)

```go
	// the signaler server is only for test
	signaler := lens2.NewSignaler("client", "https://test:test@signaler.slive.fun")
	bind := wgortc.NewBind(signaler)
	dev = device.NewDevice(tun, bind, device.NewLogger(loglevel, "client"))
```

## Custom Signaler Server

implement the `signaler.Channel` interface

```go
package signaler

import "github.com/pion/webrtc/v3"

type SDP = webrtc.SessionDescription

type Channel interface {
	Handshake(endpoint string, offer SDP) (answer *SDP, err error)
	Accept() (offerCh <-chan Session, err error)

	Close() error
}

type Session interface {
	Description() (offer SDP)
	Resolve(answer *SDP) (err error)
	Reject(err error)
}
```
