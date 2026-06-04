// Command sidecar is the Go process spawned by the VS Code extension.
// It speaks JSON-RPC 2.0 over stdio. All model traffic flows through the
// hard-locked Foundry client (see internal/foundry/lock.go).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/patmeh1/foundry-copilot/core/internal/config"
	"github.com/patmeh1/foundry-copilot/core/internal/foundry"
	"github.com/patmeh1/foundry-copilot/core/internal/logx"
	"github.com/patmeh1/foundry-copilot/core/internal/rpc"
)

// Version is overridden at build time with -ldflags "-X main.Version=...".
var Version = "0.1.0-dev"

func main() {
	logLevel := flag.String("log-level", "info", "log level: debug|info|warn|error")
	flag.Parse()

	log := logx.New(*logLevel)
	log.Info("sidecar starting", "version", Version, "pid", os.Getpid())

	cfg, err := config.Load()
	if err != nil {
		log.Error("config load failed", "err", err)
		os.Exit(2)
	}
	log.Debug("config loaded", "endpoint_set", cfg.Endpoint != "")

	ctx, cancel := signalCtx()
	defer cancel()

	srv := &rpc.Server{Log: log, Version: Version}

	// If an endpoint is already configured, try to construct the client now
	// so the first chat call doesn't have a one-time auth penalty.
	if cfg.Endpoint != "" {
		if err := foundry.ValidateEndpoint(cfg.Endpoint); err != nil {
			log.Error("startup endpoint rejected by hard lock", "err", err)
		} else if cred, err := foundry.NewCredential(); err != nil {
			log.Warn("credential build failed (will retry on demand)", "err", err)
		} else if client, err := foundry.NewClient(cfg.Endpoint, cred); err != nil {
			log.Warn("foundry client build failed (will retry on demand)", "err", err)
		} else {
			srv.Foundry = client
			log.Info("foundry client ready", "endpoint", cfg.Endpoint)
		}
	}

	if err := srv.Run(ctx, os.Stdin, os.Stdout); err != nil {
		log.Error("rpc loop exited with error", "err", err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	log.Info("sidecar shutdown clean")
}

func signalCtx() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		cancel()
	}()
	return ctx, cancel
}
