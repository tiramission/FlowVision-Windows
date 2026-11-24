//go:build debug

package main

// Test-only control endpoint used by the local end-to-end checks. It is
// compiled only with `go build -tags debug` and is never part of a release
// build. The server listens on loopback and answers Eval requests so the
// harness can drive the page and read its DOM back.

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"time"
)

func init() {
	go func() {
		for i := 0; i < 900; i++ {
			if mainWin != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if mainWin == nil {
			return
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/eval", func(w http.ResponseWriter, r *http.Request) {
			code := r.URL.Query().Get("code")
			v, err := mainWin.Page().Eval(code)
			w.Header().Set("Content-Type", "application/json")
			out := map[string]any{"result": v}
			if err != nil {
				out["error"] = err.Error()
			}
			_ = json.NewEncoder(w).Encode(out)
		})
		mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("ok"))
		})
		ln, err := net.Listen("tcp", "127.0.0.1:47831")
		if err != nil {
			_, _ = os.Stderr.WriteString("debug server: " + err.Error() + "\n")
			return
		}
		_ = http.Serve(ln, mux)
	}()
}
