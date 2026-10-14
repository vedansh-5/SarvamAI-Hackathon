package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vedansh/vans/internal/envfile"
	"github.com/vedansh/vans/internal/server"
	"github.com/vedansh/vans/internal/synth"
)

const (
	EnvTesters = "VANS_TESTERS"
	EnvTotal   = "VANS_TOTAL_DAILY"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "where to listen")
	flag.Parse()
	envfile.Load(".env", "../.env")

	if err := run(*addr); err != nil {
		fmt.Fprintln(os.Stderr, "vans-server:", err)
		os.Exit(1)
	}
}

func run(addr string) error {
	base, err := modelFromEnv()
	if err != nil {
		return err
	}
	testers, err := server.ParseTesters(os.Getenv(EnvTesters))
	if err != nil {
		return err
	}
	if len(testers) == 0 {
		return fmt.Errorf("no testers: set %s to name:code:daily entries separated by ;", EnvTesters)
	}
	total := 0
	if spelled := strings.TrimSpace(os.Getenv(EnvTotal)); spelled != "" {
		if total, err = strconv.Atoi(spelled); err != nil || total < 0 {
			return fmt.Errorf("%s must be a whole number", EnvTotal)
		}
	}

	logger := log.New(os.Stderr, "[server] ", log.LstdFlags)
	synth.Notice = func(format string, args ...any) { logger.Printf("[model] "+format, args...) }

	srv := &server.Server{
		Model: func(used *synth.Usage) synth.Model {
			c := *base
			c.Usage = used
			return &c
		},
		Ledger: server.NewMemory(testers),
		Total:  total,
		Log:    logger.Printf,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	httpServer := &http.Server{Addr: addr, Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdown)
	}()

	backup := "none"
	if base.Backup != nil {
		backup = base.Backup.Model
	}
	logger.Printf("listening on %s for %d tester(s); model %s, backup %s", addr, len(testers), base.Model, backup)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func modelFromEnv() (*synth.OpenAICompatible, error) {
	key := strings.TrimSpace(os.Getenv(synth.EnvAPIKey))
	if key == "" {
		return nil, fmt.Errorf("%s is not set", synth.EnvAPIKey)
	}
	wait, mode := synth.WaitAndModeFromEnv()
	return synth.FromSettings(synth.WithBackupFrom(os.Getenv, synth.Settings{
		APIKey:   key,
		BaseURL:  os.Getenv(synth.EnvBaseURL),
		Model:    os.Getenv(synth.EnvModel),
		Fast:     os.Getenv(synth.EnvFastModel),
		Fallback: os.Getenv(synth.EnvFallbackModel),
	}), wait, mode), nil
}
