package logger

import (
	"fmt"
	"log/slog"

	"golang.zx2c4.com/wireguard/device"
)

func New(prepend string) *device.Logger {
	logger := slog.With("device", prepend)
	return &device.Logger{
		Verbosef: func(format string, args ...any) {
			msg := fmt.Sprintf(format, args...)
			logger.Debug(msg)
		},
		Errorf: func(format string, args ...any) {
			msg := fmt.Sprintf(format, args...)
			logger.Error(msg)
		},
	}
}
