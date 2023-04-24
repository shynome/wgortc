package main

import (
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lainio/err2"
	"github.com/lainio/err2/try"
	"github.com/shynome/wgortc/signaler/local"
	"golang.zx2c4.com/wireguard/device"
)

var debugHub = local.NewHub()

func TestServer(t *testing.T) {
	if !debug {
		return
	}
	dev := startServer(debugHub)
	<-dev.Wait()
}

func TestClient(t *testing.T) {
	if !debug {
		return
	}
	dev, tnet := startClient(debugHub)
	defer dev.Close()
	httpGet(tnet)
}

func TestNet(t *testing.T) {
	hub := local.NewHub()
	dev := startServer(hub)
	defer dev.Close()
	dev2, tnet := startClient(hub)
	defer dev2.Close()
	httpGet(tnet)
}

func TestReconnect(t *testing.T) {
	hub := local.NewHub()

	dev := startServer(hub)
	dev2, tnet := startClient(hub)
	defer dev2.Close()
	httpGet(tnet)

	dev.Close()
	dev = startServer(hub)
	defer dev.Close()

	httpGet(tnet)

}

func TestDevClose(t *testing.T) {
	hub := local.NewHub()
	dev := startServer(hub)
	// time.Sleep(time.Second * 5)
	dev.Close()
}

var debug = false

func TestMain(m *testing.M) {
	debug = strings.HasSuffix(os.Args[0], "__debug_bin")
	if !debug {
		loglevel = device.LogLevelError
	}
	m.Run()
}

func BenchmarkNet(b *testing.B) {
	hub := local.NewHub()
	dev := startServer(hub)
	defer dev.Close()
	defer time.Sleep(time.Second) // todo fix
	dev2, tnet := startClient(hub)
	defer dev2.Close()

	client := http.Client{
		Transport: &http.Transport{
			DialContext: tnet.DialContext,
		},
	}
	try.To1(client.Get("http://192.168.4.29/"))

	var wg sync.WaitGroup
	wg.Add(b.N)
	for i := 0; i < b.N; i++ {
		go func() {
			defer wg.Done()
			defer err2.Catch()
			resp := try.To1(client.Get("http://192.168.4.29/"))
			_ = try.To1(io.ReadAll(resp.Body))
		}()
	}
	wg.Wait()
}
