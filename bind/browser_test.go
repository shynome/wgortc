package bind_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/shynome/err0/try"
)

func TestBrowser(t *testing.T) {
	testBrowser(t, true)
	time.Sleep(3 * time.Second) // 确保上一个进程已退出
	testBrowser(t, false)
}

//go:generate go install github.com/agnivade/wasmbrowsertest@v0.11.0

func testBrowser(t *testing.T, nowsc bool) {
	buildTry(nowsc)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	testWasm := exec.CommandContext(ctx, "bash", "-c", "WASM_HEADLESS=off wasmbrowsertest browser_test.wasm")
	testWasm.Stdout = os.Stdout
	testWasm.Stderr = os.Stderr
	try.To(testWasm.Start())
	client := &http.Client{
		Transport: &http.Transport{DialContext: serverNet.DialContext},
		Timeout:   30 * time.Second,
	}

	func() {
		defer cancel()

		resp := try.To1(client.Get("http://192.168.7.2/"))
		body := try.To1(io.ReadAll(resp.Body))

		if body := string(body); body != "hello world!" {
			t.Error(body)
		}
	}()

	testWasm.Wait()
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
