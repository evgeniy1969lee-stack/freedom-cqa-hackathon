package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/homepunks/datathong/internal/checklist"
	"github.com/homepunks/datathong/internal/config"
	"github.com/homepunks/datathong/internal/evaluator"
	"github.com/homepunks/datathong/internal/httpapi"
	"github.com/homepunks/datathong/internal/queue"
	"github.com/homepunks/datathong/internal/store"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	seed, err := checklist.Seed()
	if err != nil {
		return err
	}
	if err := st.EnsureSeedChecklist(ctx, seed); err != nil {
		return err
	}

	var ev evaluator.Evaluator
	mlName := "http"
	if cfg.MLURL == "" {
		log.Warn("CQA_ML_URL is empty: using the stub evaluator; results are placeholders")
		ev = evaluator.Stub{}
		mlName = "stub"
	} else {
		ev = evaluator.HTTP{URL: cfg.MLURL, Client: &http.Client{}}
	}
	worker := queue.New(st, ev, cfg.Workers, cfg.MLTimeout, log)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		worker.Run(ctx)
	}()

	srv := &http.Server{
		Addr: cfg.Addr,
		Handler: httpapi.New(st, httpapi.Options{
			MLName: mlName, DevURL: cfg.DevURL, StaticDir: cfg.StaticDir, Worker: worker, MaxAttempts: cfg.MaxAttempts,
		}, log),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	log.Info("listening", "addr", cfg.Addr, "db", cfg.DBPath, "dev_url_configured", cfg.DevURL != "",
		"ml_url_configured", cfg.MLURL != "", "workers", cfg.Workers, "ml_timeout", cfg.MLTimeout, "max_attempts", cfg.MaxAttempts)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-workerDone
	return nil
}
