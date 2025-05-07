package bind

import "context"

const magicStr = "wgortc"

func ref[T any](v T) *T {
	return &v
}

func iter[T any](ctx context.Context, ch <-chan T, fn func(c T) error) {
	for {
		select {
		case <-ctx.Done():
			return
		case c, ok := <-ch:
			if ok {
				fn(c)
			}
		}
	}
}
