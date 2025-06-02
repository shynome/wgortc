package bind

import "errors"

type PeerMode interface {
	TransportMode() TransportMode
}

type TransportMode uint32

const (
	WSTransportDisabled     TransportMode = 0b0000_0000_0000_0001
	WebRTCTransportDisabled TransportMode = 0b0000_0000_0000_0010
)

const (
	AllTransportDisabled TransportMode = WSTransportDisabled | WebRTCTransportDisabled
)

var ErrNoDataChannel = errors.New("no available data channel")
var ErrWSCDrop = errors.New("drop data when ws transport disabled")
var ErrWebRTCDisabled = errors.New("drop data when ws transport disabled")
