package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/lainio/err2/try"
	"github.com/shynome/wgortc"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/ipc"
	"golang.zx2c4.com/wireguard/tun"
)

func main() {
	tdevn := flag.String("tun", "wgortc", "tun name")
	id := flag.String("id", "", "webrtc id")
	signaler := flag.String("signaler", "https://test:test@signaler.slive.fun/", "signaler server")
	loglevel := flag.Int("log", 0, "log level. slient:0 error:1 verbose:2")
	flag.Parse()

	if *id == "" {
		fmt.Fprintln(os.Stderr, "id is required")
		os.Exit(1)
		return
	}

	tdev := try.To1(tun.CreateTUN(*tdevn, device.DefaultMTU))
	bind := wgortc.NewBind(*id, *signaler)
	logger := device.NewLogger(*loglevel, *id)
	dev := device.NewDevice(tdev, bind, logger)

	f := try.To1(ipc.UAPIOpen(*tdevn))
	uapi := try.To1(ipc.UAPIListen(*tdevn, f))

	fmt.Printf("wgortc: node %s is running on tun %s\n", *id, *tdevn)
	for {
		conn := try.To1(uapi.Accept())
		go dev.IpcHandle(conn)
	}
}
