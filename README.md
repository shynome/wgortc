# wgortc (Wireguard Over Webrtc)

## How to Use

replace `conn.Bind` with this. more details see [example/main.go](./example/main.go)

```go
	// the signaler server is only for test
	bind := wgortc.NewBind("client", "https://test:test@signaler.slive.fun")
	dev = device.NewDevice(tun, bind, device.NewLogger(loglevel, "client"))
```

## Deps

- sinagler server source repo - <https://github.com/shynome/lens2>
