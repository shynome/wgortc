package bind

import (
	"net/http"

	"github.com/shynome/websocket"
)

const (
	WsStatusBase websocket.StatusCode = 3000

	WsStatusBadRequest         = WsStatusBase + http.StatusBadRequest
	WsStatusServiceUnavailable = WsStatusBase + http.StatusServiceUnavailable
	WsStatusNotAcceptable      = WsStatusBase + http.StatusNotAcceptable
	WsStatusTemporaryRedirect  = WsStatusBase + http.StatusTemporaryRedirect
	WsStatusClientClosed       = WsStatusBase + 499
)
