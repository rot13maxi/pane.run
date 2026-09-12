package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/agent-surface/agent-surface/internal/server"
	"github.com/agent-surface/agent-surface/internal/store"
)

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	data := flag.String("data", "./surface-data/surfaces.json", "metadata store path")
	baseURL := flag.String("base-url", "http://localhost:8080", "public base URL")
	flag.Parse()

	logger := log.New(os.Stderr, "paned: ", log.LstdFlags)
	st, err := store.Open(*data)
	if err != nil {
		logger.Fatalf("open store: %v", err)
	}
	srv := &http.Server{
		Addr:              *listen,
		Handler:           server.New(st, *baseURL, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	logger.Printf("listening on %s", *listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("serve: %v", err)
	}
}
