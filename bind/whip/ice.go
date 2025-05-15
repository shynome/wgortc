package whip

import (
	"net/url"

	"github.com/pion/webrtc/v4"
)

func ParseICEServer(s string) webrtc.ICEServer {
	u, err := url.Parse(s)
	if err != nil || u.Opaque != "" {
		return webrtc.ICEServer{
			URLs: []string{s},
		}
	}
	link := u.Scheme + ":" + u.Host
	transport := u.Query().Get("transport")
	if transport != "" && (transport == "tcp" || transport == "udp") {
		link = link + "?transport=" + transport
	}
	ice := webrtc.ICEServer{
		URLs:           []string{link},
		CredentialType: webrtc.ICECredentialTypePassword,
	}
	if uinfo := u.User; uinfo != nil {
		ice.Username = uinfo.Username()
		ice.Credential, _ = uinfo.Password()
	}
	return ice
}
