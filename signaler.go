package wgortc

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lainio/err2/try"
)

type signaler struct {
	endpoint *url.URL
	client   *http.Client
}

func newSignaler(endpoint string) *signaler {
	u := try.To1(url.Parse(endpoint))
	return &signaler{
		endpoint: u,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *signaler) newReq(method string, topic string, body io.Reader) (req *http.Request, err error) {
	if req, err = http.NewRequest(method, s.endpoint.String(), body); err != nil {
		return
	}
	q := req.URL.Query()
	q.Set("t", topic)
	req.URL.RawQuery = q.Encode()
	u := s.endpoint.User
	pass, _ := u.Password()
	req.SetBasicAuth(u.Username(), pass)
	return
}

func (p *signaler) doReq(req *http.Request) (res *http.Response, err error) {
	res, err = p.client.Do(req)
	if err != nil {
		return
	}
	if strings.HasPrefix(res.Status, "2") {
		return
	}
	var errText []byte
	if errText, err = io.ReadAll(res.Body); err != nil {
		return
	}
	err = fmt.Errorf("server err. status: %s. content: %s", res.Status, errText)
	return
}
