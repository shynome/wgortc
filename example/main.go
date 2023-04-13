package main

import (
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"

	"github.com/lainio/err2/try"
	"github.com/shynome/wgortc"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

func main() {
	runServer := flag.Bool("server", false, "")
	flag.Parse()
	if *runServer {
		dev := startServer()
		// <-dev.Wait()
		dev.Close()
		return
	} else {
		startClient()
	}
}

var signaler = "https://test:test@signaler.slive.fun"
var loglevel = device.LogLevelVerbose

func startServer() (dev *device.Device) {
	tdev, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("192.168.4.29")},
		[]netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("8.8.4.4")},
		1420,
	)
	try.To(err)
	var bind conn.Bind
	bind = wgortc.NewBind("server", signaler)
	// bind = conn.NewDefaultBind()
	dev = device.NewDevice(tdev, bind, device.NewLogger(loglevel, "server"))
	dev.IpcSet(`private_key=003ed5d73b55806c30de3f8a7bdab38af13539220533055e635690b8b87ad641
listen_port=0
public_key=f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c
allowed_ip=192.168.4.28/32
`)
	try.To(dev.Up())

	listener := try.To1(tnet.ListenTCP(&net.TCPAddr{Port: 80}))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		log.Printf("> %s - %s - %s", request.RemoteAddr, request.URL.String(), request.UserAgent())
		io.WriteString(writer, "Hello from userspace TCP!")
	})
	go func() {
		try.To(http.Serve(listener, mux))
	}()
	return
}

func startClient() (dev *device.Device, tnet *netstack.Net) {
	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("192.168.4.28")},
		[]netip.Addr{netip.MustParseAddr("8.8.8.8")},
		1420)
	try.To(err)
	bind := wgortc.NewBind("client", signaler)
	dev = device.NewDevice(tun, bind, device.NewLogger(loglevel, "client"))
	err = dev.IpcSet(`private_key=087ec6e14bbed210e7215cdc73468dfa23f080a1bfb8665b2fd809bd99d28379
public_key=c4c8e984c5322c8184c72265b92b250fdb63688705f504ba003c88f03393cf28
allowed_ip=0.0.0.0/0
endpoint=server
`)
	try.To(dev.Up())

	client := http.Client{
		Transport: &http.Transport{
			DialContext: tnet.DialContext,
		},
	}
	resp := try.To1(client.Get("http://192.168.4.29/"))
	body := try.To1(io.ReadAll(resp.Body))
	log.Println(string(body))
	return
}
