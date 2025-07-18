package simple

import (
	"fmt"
	"testing"

	"github.com/shynome/err0/try"
)

func TestConfig(t *testing.T) {
	c := Config{
		Key:  "IDHBZNpXkYmavc3JhCvCA9bTh6fo2IfB1D/F6mE6xXg=",
		Peer: "ws://127.0.0.1:7788/link#UwJ8NDnTdT/XM1VC8wPF7iu0GMP3FK81qRPSQlHQ7jU=",
		Peers: []*Peer{
			{
				Pubkey:   "UwJ8NDnTdT/XM1VC8wPF7iu0GMP3FK81qRPSQlHQ7jU=",
				Endpoint: "ws://127.0.0.1:7788/link",
				Auto:     10,
				Allow:    "192.168.7.3/32",
				Allow6:   "fdd9:f800::2/128",
			},
		},
	}
	try.To(c.Normalize())
	t.Log(c.peers)
	ipc := c.IpcConfig()
	fmt.Println(ipc)
	t.Log(ipc)
}
