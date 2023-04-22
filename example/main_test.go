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
	"golang.zx2c4.com/wireguard/device"
)

func TestServer(t *testing.T) {
	if !debug {
		return
	}
	dev := startServer()
	<-dev.Wait()
}

func TestClient(t *testing.T) {
	if !debug {
		return
	}
	dev, _ := startClient()
	defer dev.Close()
}

func TestNet(t *testing.T) {
	dev := startServer()
	defer dev.Close()
	dev2, _ := startClient()
	defer dev2.Close()
}

func TestReconnect(t *testing.T) {
	dev := startServer()
	dev2, tnet := startClient()
	defer dev2.Close()

	dev.Close()
	dev = startServer()
	defer dev.Close()

	client := http.Client{Transport: &http.Transport{DialContext: tnet.DialContext}}
	resp := try.To1(client.Get("http://192.168.4.29/"))
	body := try.To1(io.ReadAll(resp.Body))
	t.Log(string(body))

}

func TestDevClose(t *testing.T) {
	dev := startServer()
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
	dev := startServer()
	defer dev.Close()
	defer time.Sleep(time.Second) // todo fix
	dev2, tnet := startClient()
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
