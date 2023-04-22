package lens2

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/donovanhide/eventsource"
	"github.com/lainio/err2"
	"github.com/lainio/err2/try"
	impl "github.com/shynome/wgortc/signaler"
)

type Signaler struct {
	id       string
	signaler *signaler

	connectionStream *eventsource.Stream
}

var _ impl.Channel = (*Signaler)(nil)

func NewSignaler(id string, endpoint string) *Signaler {
	return &Signaler{
		id:       id,
		signaler: newSignaler(endpoint),
	}
}

func (b *Signaler) Handshake(endpoint string, offer impl.SDP) (answer *impl.SDP, err error) {

	body := try.To1(json.Marshal(offer))
	req := try.To1(b.signaler.newReq(http.MethodPost, endpoint, bytes.NewReader(body)))
	res := try.To1(b.signaler.doReq(req))

	try.To(json.NewDecoder(res.Body).Decode(&answer))

	return
}
func (b *Signaler) Accept() (ch <-chan impl.Session, err error) {
	defer err2.Handle(&err)
	offerCh := make(chan impl.Session)
	req := try.To1(b.signaler.newReq(http.MethodGet, b.id, http.NoBody))
	b.connectionStream = try.To1(eventsource.SubscribeWithRequest("", req))
	go func() {
		defer close(offerCh)
		for ev := range b.connectionStream.Events {
			go func(ev eventsource.Event) {
				sess := b.newSession(ev)
				if sess == nil {
					return
				}
				offerCh <- sess
			}(ev)
		}
	}()
	return offerCh, nil
}

func (s *Signaler) Close() (err error) {
	if stream := s.connectionStream; stream != nil {
		stream.Close()
	}
	return
}

type Session struct {
	root  *Signaler
	ev    eventsource.Event
	offer impl.SDP
}

var _ impl.Session = (*Session)(nil)

func (s *Signaler) newSession(ev eventsource.Event) *Session {
	var offer impl.SDP
	if err := json.Unmarshal([]byte(ev.Data()), &offer); err != nil {
		return nil
	}
	return &Session{
		root:  s,
		ev:    ev,
		offer: offer,
	}
}

func (s *Session) Description() (offer impl.SDP) { return s.offer }
func (s *Session) Resolve(answer *impl.SDP) (err error) {
	defer err2.Handle(&err)
	b := s.root
	body := try.To1(json.Marshal(answer))
	req := try.To1(b.signaler.newReq(http.MethodDelete, b.id, bytes.NewReader(body)))
	req.Header.Set("X-Event-Id", s.ev.Id())
	try.To1(b.signaler.doReq(req))
	return
}
func (Session) Reject(err error) { return }
