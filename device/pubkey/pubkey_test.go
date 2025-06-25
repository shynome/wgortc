package pubkey_test

import (
	"encoding/hex"
	"testing"

	"github.com/shynome/err0/try"
	"github.com/shynome/wgortc/device/pubkey"
	"golang.zx2c4.com/wireguard/device"
)

func TestUnpack(t *testing.T) {
	sk := try.To1(hex.DecodeString("e0a292790317a9f4f449157f7fb2e43db5a3a728c24ba21f070afb818da7af43"))
	pk := try.To1(hex.DecodeString("7391b59d4d3a223acde9e5a956aebe9a962754a7d54ec8bda3e81fcd8f85ba2e"))
	initiator := try.To1(pubkey.Initiator(device.NoisePrivateKey(sk), device.NoisePublicKey(pk)))
	{
		sk := try.To1(hex.DecodeString("2031c164da5791899abdcdc9842bc203d6d387a7e8d887c1d43fc5ea613ac578"))
		pk := try.To1(pubkey.Unpack(device.NoisePrivateKey(sk), initiator))
		pkStr := hex.EncodeToString(pk[:])
		if pkStr != "53027c3439d3753fd7335542f303c5ee2bb418c3f714af35a913d24251d0ee35" {
			t.Errorf("want 53027c3439d3753fd7335542f303c5ee2bb418c3f714af35a913d24251d0ee35, but got %s", pkStr)
			return
		}
		t.Log(pkStr)
	}
}

// key: 4KKSeQMXqfT0SRV/f7LkPbWjpyjCS6IfBwr7gY2nr0M=
// key(hex): e0a292790317a9f4f449157f7fb2e43db5a3a728c24ba21f070afb818da7af43
// pubkey: UwJ8NDnTdT/XM1VC8wPF7iu0GMP3FK81qRPSQlHQ7jU=
// pubkey(hex): 53027c3439d3753fd7335542f303c5ee2bb418c3f714af35a913d24251d0ee35
var p1cfg = `private_key=e0a292790317a9f4f449157f7fb2e43db5a3a728c24ba21f070afb818da7af43
listen_port=7777
public_key=7391b59d4d3a223acde9e5a956aebe9a962754a7d54ec8bda3e81fcd8f85ba2e
allowed_ip=192.168.7.2/32`

// key: IDHBZNpXkYmavc3JhCvCA9bTh6fo2IfB1D/F6mE6xXg=
// key(hex): 2031c164da5791899abdcdc9842bc203d6d387a7e8d887c1d43fc5ea613ac578
// pubkey: c5G1nU06IjrN6eWpVq6+mpYnVKfVTsi9o+gfzY+Fui4=
// pubkey(hex): 7391b59d4d3a223acde9e5a956aebe9a962754a7d54ec8bda3e81fcd8f85ba2e
var p2cfg = `private_key=2031c164da5791899abdcdc9842bc203d6d387a7e8d887c1d43fc5ea613ac578
public_key=53027c3439d3753fd7335542f303c5ee2bb418c3f714af35a913d24251d0ee35
endpoint=ws://127.0.0.1:7788
allowed_ip=192.168.7.1/32`
