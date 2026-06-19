package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/ercross/reconD/internal/config"
	git_provider2 "github.com/ercross/reconD/internal/git_provider"
	"github.com/ercross/reconD/internal/logger"
	notifier2 "github.com/ercross/reconD/internal/notifier"
	"github.com/ercross/reconD/internal/reconciler"
	"github.com/ercross/reconD/internal/state"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "agent error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "path to agent config file")
	flag.Parse()
	if *configPath == "" {
		return fmt.Errorf("missing config path")
	}

	log := logger.Setup()
	log.Info("deployment agent starting", "config", *configPath)

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config error %q: %w", *configPath, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	var wg sync.WaitGroup
	for _, workload := range cfg.Workloads {
		workload := workload

		stateMgr, err := state.NewManagerWithLocalFileSystem(workload.StateDir)
		if err != nil {
			return fmt.Errorf("create state manager for workload %q: %w", workload.Name, err)
		}

		gp, err := gitProviderFor(workload)
		if err != nil {
			return fmt.Errorf("create git provider for workload %q: %w", workload.Name, err)
		}

		r := reconciler.New(
			workload,
			gp,
			stateMgr,
			notifierFor(workload, log),
			log,
		)

		wg.Add(1)
		go func() {
			defer wg.Done()
			r.StartPeriodicReconciliation(ctx)
		}()
	}

	<-ctx.Done()
	wg.Wait()
	log.Info("agent shutdown complete")
	return nil
}

func gitProviderFor(workload config.Workload) (git_provider2.GitProvider, error) {
	token := workload.GitProvider.Token
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	return git_provider2.NewGithubClient(workload.GitProvider.Owner, workload.GitProvider.Repo, token)
}

func notifierFor(workload config.Workload, log *slog.Logger) notifier2.Notifier {
	if workload.NotificationURL == "" {
		return notifier2.Noop{}
	}
	return notifier2.NewSlackNotifier(workload.NotificationURL, log)
}
