package endpoint

import (
	"context"
	"errors"
	"time"

	"github.com/pion/webrtc/v3"
)

func refVal[T any](v T) *T { return &v }

var ErrDataChannelClosed = errors.New("DataChannel state is closed")

func WaitDC(dc *webrtc.DataChannel, timeout time.Duration) (err error) {
	switch dc.ReadyState() {
	case webrtc.DataChannelStateOpen:
		return
	case webrtc.DataChannelStateClosing:
		fallthrough
	case webrtc.DataChannelStateClosed:
		return ErrDataChannelClosed
	case webrtc.DataChannelStateConnecting:
		break
	}

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ctx, cancelWith := context.WithCancelCause(ctx)

	dc.OnOpen(func() {
		cancelWith(nil)
	})
	dc.OnError(func(err error) {
		cancelWith(err)
	})

	<-ctx.Done()

	switch err = context.Cause(ctx); err {
	case context.Canceled:
		return nil
	}

	return
}
