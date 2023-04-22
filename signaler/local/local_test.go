package local

import (
	"context"
	"testing"

	"github.com/lainio/err2/assert"
	"github.com/lainio/err2/try"
	"github.com/pion/webrtc/v3"
	"github.com/shynome/wgortc/signaler"
)

func TestChannel(t *testing.T) {
	var hub = NewHub()
	s1, s2 := NewServer(), NewServer()
	hub.Register("s1", s1)
	hub.Register("s2", s2)

	offer := signaler.SDP{Type: webrtc.SDPTypeOffer}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ch := try.To1(s1.Accept())
		cancel()
		for session := range ch {
			offer := session.Description()
			assert.Equal(offer.Type, webrtc.SDPTypeOffer)
			session.Resolve(&signaler.SDP{Type: webrtc.SDPTypeAnswer})
		}
	}()
	<-ctx.Done() // wait unitl s1 start accept

	answer := try.To1(s2.Handshake("s1", offer))
	assert.Equal(answer.Type, webrtc.SDPTypeAnswer)

}
