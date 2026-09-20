//go:build desktop

// Command alexander is the desktop shell: it runs the game server on a
// loopback port and presents it in a native webview window, closing the
// server when the window closes. Built with -tags desktop; the webview
// toolchain (WebKitGTK on Linux) is only required when this tag is set.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/webview/webview_go"

	"goGame/internal/content"
	"goGame/internal/paths"
	"goGame/internal/web"

	assets "goGame"
)

const (
	windowTitle = "Alexander — a card adventure"
	windowW     = 1280
	windowH     = 800
	saveMaxAge  = 30 * 24 * time.Hour
)

func main() {
	lib, err := content.Load(assets.FS())
	if err != nil {
		log.Fatal(err)
	}
	saves, err := paths.UserSavesDir()
	if err != nil {
		log.Fatalf("resolve user data dir: %v", err)
	}
	store := web.NewStore(lib, saves)
	if n := store.Sweep(saveMaxAge); n > 0 {
		log.Printf("swept %d stale save(s)", n)
	}
	srv := web.New(store)

	// Loopback with a kernel-chosen port: no collisions, no exposure.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		if err := srv.Serve(ln); err != nil {
			log.Printf("serve: %v", err)
		}
	}()

	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle(windowTitle)
	w.SetSize(windowW, windowH, webview.HintNone)
	w.Navigate(fmt.Sprintf("http://%s", ln.Addr().String()))
	w.Run()

	// Window closed: drain in-flight requests and exit.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
