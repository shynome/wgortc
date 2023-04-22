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
