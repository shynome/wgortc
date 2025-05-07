package bind_test

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	_ "github.com/agnivade/wasmbrowsertest/filesys"
	"github.com/shynome/err0/try"
)

func TestBrowser(t *testing.T) {
	testBrowser(t, true)
	time.Sleep(3 * time.Second) // 确保上一个进程已退出
	testBrowser(t, false)
}

func testBrowser(t *testing.T, nowsc bool) {
	buildTry(nowsc)
	testWasm := exec.Command("bash", "-c", "WASM_HEADLESS=off go run github.com/agnivade/wasmbrowsertest browser_test.wasm")
	testWasm.Stdout = os.Stdout
	testWasm.Stderr = os.Stderr
	try.To(testWasm.Start())
	client := &http.Client{
		Transport: &http.Transport{DialContext: serverNet.DialContext},
		Timeout:   30 * time.Second,
	}
	resp := try.To1(client.Get("http://192.168.7.2/browser"))
	body := try.To1(io.ReadAll(resp.Body))

	if body := string(body); body != "hello world!" {
		t.Error(body)
	}

	go client.Get("http://192.168.7.2/exit")
	try.To(testWasm.Wait())
}

func buildTry(nowsc bool) {
	s := "false"
	if nowsc {
		s = "true"
	}
	c := fmt.Sprintf(`GOOS=js GOARCH=wasm go build -ldflags="-X 'main.NOWSC=%s'" -o browser_test.wasm ./internal/wasm_test/`, s)
	build := exec.Command("bash", "-c", c)
	build.Stderr = os.Stderr
	try.To(build.Run())
}
