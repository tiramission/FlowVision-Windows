package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"testing"
	"time"
)

// TestServeManualWebDAV serves a folder over WebDAV for a manual run of the
// app. Set FLOWVISION_DAV_ROOT and FLOWVISION_DAV_PORT.
func TestServeManualWebDAV(t *testing.T) {
	root := os.Getenv("FLOWVISION_DAV_ROOT")
	if root == "" {
		t.Skip("FLOWVISION_DAV_ROOT not set")
	}
	port := os.Getenv("FLOWVISION_DAV_PORT")
	if port == "" {
		port = "5005"
	}
	srv := &http.Server{Addr: "127.0.0.1:" + port, Handler: davHandler(root)}
	go func() { _ = srv.ListenAndServe() }()
	fmt.Printf("webdav serving %s on http://127.0.0.1:%s\n", root, port)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	select {
	case <-stop:
	case <-time.After(30 * time.Minute):
	}
	_ = srv.Shutdown(context.Background())
}
