// Command reconD runs a deployment reconciliation agent for single-host
// container workloads.
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

	"github.com/ercross/reconD/config"
	"github.com/ercross/reconD/git_provider"
	"github.com/ercross/reconD/logger"
	"github.com/ercross/reconD/notifier"
	"github.com/ercross/reconD/reconciler"
	"github.com/ercross/reconD/state"
)

var (
	defaultConfigFile           = "/etc/reconD/config.yaml"
	defaultDeploymentsStateFile = "/var/lib/reconD/deployments_state.json"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "agent error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", defaultConfigFile, "path to agent config file")
	statePath := flag.String("state", defaultDeploymentsStateFile, "path to deployments state file")
	flag.Parse()
	if *configPath == "" {
		return fmt.Errorf("missing config path")
	}
	if *statePath == "" {
		return fmt.Errorf("missing deployments state path")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config error %q: %w", *configPath, err)
	}

	log := logger.Setup(cfg.Log)
	log.Info("deployment agent starting", "config", *configPath, "state", *statePath)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	stateMgr, err := state.NewManagerWithLocalFileSystem(*statePath)
	if err != nil {
		return fmt.Errorf("create deployments state manager: %w", err)
	}

	var wg sync.WaitGroup
	for _, workload := range cfg.Workloads {
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

		wg.Go(func() {
			r.StartPeriodicReconciliation(ctx)
		})
	}

	<-ctx.Done()
	wg.Wait()
	log.Info("agent shutdown complete")
	return nil
}

func gitProviderFor(workload config.Workload) (git_provider.GitProvider, error) {
	token := workload.GitProvider.Token
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	return git_provider.NewGithubClient(workload.GitProvider.Owner, workload.GitProvider.Repo, token)
}

func notifierFor(workload config.Workload, log *slog.Logger) notifier.Notifier {
	if workload.NotificationURL == "" {
		return notifier.Noop{}
	}
	return notifier.NewSlackNotifier(workload.NotificationURL, log)
}
