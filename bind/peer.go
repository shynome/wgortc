package bind

import "errors"

type PeerMode interface {
	TransportMode() TransportMode
}

type TransportMode uint32

const (
	WSTransportDisabled TransportMode = 1 << (iota + 1)
	WebRTCTransportDisabled
	WSRedirectEnabled
)

const (
	AllTransportDisabled TransportMode = WSTransportDisabled | WebRTCTransportDisabled
)

var ErrNoDataChannel = errors.New("no available data channel")
var ErrWSCDrop = errors.New("drop data when ws transport disabled")
var ErrWebRTCDisabled = errors.New("drop data when ws transport disabled")
