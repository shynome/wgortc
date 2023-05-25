package local

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shynome/wgortc/signaler"
)

type Server struct {
	ch  chan signaler.Session
	hub *Hub
}

func NewServer() *Server {
	return &Server{}
}

var _ signaler.Channel = (*Server)(nil)

func (s *Server) Handshake(endpoint string, offer signaler.SDP) (answer *signaler.SDP, err error) {
	if s.hub == nil {
		return nil, fmt.Errorf("server need register to a local hub")
	}
	remote := s.hub.Find(endpoint)
	if remote == nil {
		return nil, fmt.Errorf("server is not found. ep: %s", endpoint)
	}
	if remote.ch == nil {
		return nil, fmt.Errorf("server is not ready accept")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session := NewSession(ctx, offer)
	remote.ch <- session
	return session.Result()
}

type Session struct {
	context.Context
	reject context.CancelCauseFunc

	offer signaler.SDP

	answer *signaler.SDP
}

var _ signaler.Session = (*Session)(nil)

func NewSession(ctx context.Context, sdp signaler.SDP) *Session {
	ctx, reject := context.WithCancelCause(ctx)
	return &Session{
		Context: ctx,
		reject:  reject,

		offer: sdp,
	}
}

func (sess *Session) Description() signaler.SDP { return sess.offer }
func (sess *Session) Reject(err error) {
	sess.reject(err)
}
func (sess *Session) Resolve(answer *signaler.SDP) (err error) {
	defer sess.reject(nil)
	sess.answer = answer
	return
}
func (sess *Session) Result() (answer *signaler.SDP, err error) {
	<-sess.Done()
	switch err = context.Cause(sess); err {
	case context.Canceled:
		return sess.answer, nil
	}
	return
}

func (s *Server) Accept() (ch <-chan signaler.Session, err error) {
	if s.ch != nil {
		return s.ch, nil
	}
	s.ch = make(chan signaler.Session)
	ch = s.ch
	return
}

func (s *Server) Close() (err error) {
	if ch := s.ch; ch != nil {
		s.ch = nil
		close(ch)
	}
	return
}

type Hub struct {
	pool  map[string]*Server
	poolL *sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		pool:  make(map[string]*Server),
		poolL: &sync.RWMutex{},
	}
}

func (hub *Hub) Register(endpoint string, server *Server) {
	if endpoint == "" || server == nil {
		return
	}
	hub.poolL.Lock()
	defer hub.poolL.Unlock()
	server.hub = hub
	hub.pool[endpoint] = server
}

func (hub *Hub) Find(endpoint string) *Server {
	hub.poolL.Lock()
	defer hub.poolL.Unlock()
	server, ok := hub.pool[endpoint]
	if ok {
		return server
	}
	return nil
}
