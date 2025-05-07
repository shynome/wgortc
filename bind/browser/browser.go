package browser

import (
	"net/http"
	"net/url"
)

func SplitAuth(link string) (string, *url.Userinfo, error) {
	u, err := url.Parse(link)
	if err != nil || u == nil {
		return "", nil, err
	}
	user := u.User
	if user == nil {
		return link, nil, nil
	}
	u.User = nil
	return u.String(), user, nil
}

type BasicAuth url.Userinfo

func (b *BasicAuth) SetTo(req *http.Request) {
	if b == nil || req == nil {
		return
	}
	u := (*url.Userinfo)(b)
	user := u.Username()
	pass, _ := u.Password()
	req.SetBasicAuth(user, pass)
}
