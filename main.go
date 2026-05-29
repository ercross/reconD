// Package main is the entry point for the deployment-manager.
//
// Startup sequence:
//  1. Configure structured logging
//  2. Load and validate configuration
//  3. Construct shared dependencies (GitHub client)
//  4. For each environment: construct state manager and reconciler
//  5. Launch each reconciler in its own goroutine
//  6. Block on SIGTERM/SIGINT for graceful shutdown
//
// Shutdown sequence:
//  1. Cancel the root context
//  2. All reconcilers detect cancellation and exit their loops
//  3. Wait for all goroutines to finish
//  4. Exit 0
//
// The agent is designed to be managed by systemd with Restart=on-failure.
// It exits non-zero if configuration is invalid or state directories cannot
// be created (operator action required), but recovers from transient errors
// (network failures, Docker unavailable) via the reconcile loop's retry logic.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ercross/reconD/config"
	"github.com/ercross/reconD/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "agent error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// --- Flags ---
	configPath := flag.String("config", "missing config file", "path to agent config file")
	flag.Parse()
	if *configPath == "" {
		return fmt.Errorf("missing config path")
	}

	// --- Logging ---
	// Set up structured logging first so all subsequent errors are structured.
	log := logger.Setup()
	log.Info("Deployment manager starting", "config", *configPath)

	// --- Configuration ---
	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config error %q: %w", *configPath, err)
	}

	// --- Root context with signal cancellation ---
	// The context is cancelled on SIGTERM or SIGINT, which propagates to all
	// reconciler goroutines and their in-flight HTTP requests and exec calls.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	for service := range cfg.Services {
		service := service

	}
	manager := deployment_manager.New(cfg.Projects, log)
	if err := manager.Start(ctx); err != nil {
		return err
	}
	log.Info("agent shutdown complete")
	return nil
}
