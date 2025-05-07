package whip

import "fmt"

var (
	ErrPCConnectionClosed = fmt.Errorf("pc connection closed")
	ErrPCConnectionFailed = fmt.Errorf("pc connection failed")
)
