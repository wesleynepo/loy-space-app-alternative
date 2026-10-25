package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/wesleynepo/led-panel/internal/server"
	"github.com/wesleynepo/led-panel/internal/store"
	"github.com/wesleynepo/led-panel/panel"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	port := "8765"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}
	if d := os.Getenv("LED_PANEL_DIR"); d != "" {
		_ = os.Chdir(d)
	}

	link := panel.New()
	app := server.New(link, store.New(".uploads"))
	srv := &http.Server{Addr: "127.0.0.1:" + port, Handler: app.Handler()}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		if link.Connected() {
			_ = link.Disconnect()
		}
		srv.Close()
	}()
	go func() {
		if err := link.Connect(); err != nil {
			link.Note("startup connect failed: %v", err)
		}
	}()
	fmt.Println("http://127.0.0.1:" + port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
