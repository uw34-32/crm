package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"crm/pkg/config"
	"crm/pkg/server"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// Restore default signal handling once shutdown starts, so a second signal
	// force-quits a stuck drain.
	context.AfterFunc(ctx, stop)

	srv, err := server.New(ctx, cfg)
	if err != nil {
		return err
	}
	return srv.Run(ctx)
}
