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
	WsStatusPermanentRedirect  = WsStatusBase + http.StatusPermanentRedirect
	WsStatusClientClosed       = WsStatusBase + 499
)

type PeerEndpiontRedirected interface {
	EndpiontRedirected(link string, lc int)
}
