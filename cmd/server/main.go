package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/wesleynepo/led-panel/internal/server"
	"github.com/wesleynepo/led-panel/panel"
)

func main() {
	port := "8765"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}
	if d := os.Getenv("LED_PANEL_DIR"); d != "" {
		os.Chdir(d)
	}

	link := panel.New(".uploads") // our own uploads: previews + slot keys
	app := server.New(link, server.Config{
		CacheDir: ".cache",
		AppDoc:   expandHome(getenv("LOY_DOC", "~/Library/Containers/com.yskd.loywf/Data/Library/Pandora/apps/__UNI__F2139F6/doc")),
	})
	srv := &http.Server{Addr: "127.0.0.1:" + port, Handler: app.Handler()}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		if link.Connected() {
			link.Disconnect()
		}
		srv.Close()
	}()
	fmt.Println("http://127.0.0.1:" + port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func expandHome(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, rest)
	}
	return p
}
