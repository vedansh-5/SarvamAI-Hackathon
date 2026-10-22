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
	"strings"
	"syscall"
	"time"

	"github.com/vedansh/vans/internal/envfile"
	"github.com/vedansh/vans/internal/server"
	"github.com/vedansh/vans/internal/synth"
)

const EnvTesters = "VANS_TESTERS"

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
	models, described, err := server.ModelsFromEnv()
	if err != nil {
		return err
	}
	total, err := server.TotalFromEnv()
	if err != nil {
		return err
	}

	logger := log.New(os.Stderr, "[server] ", log.LstdFlags)
	synth.Notice = func(format string, args ...any) { logger.Printf("[model] "+format, args...) }

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &server.Server{Model: models, Total: total, Log: logger.Printf}
	who := ""
	if table := strings.TrimSpace(os.Getenv(server.EnvTable)); table != "" {
		ledger, mail, send, err := server.OnAWS(ctx, table)
		if err != nil {
			return err
		}
		if srv.Joining, err = server.JoiningFromEnv(ledger, send); err != nil {
			return err
		}
		srv.Ledger, srv.Mail = ledger, mail
		who = "testers in " + table
	} else {
		testers, err := server.ParseTesters(os.Getenv(EnvTesters))
		if err != nil {
			return err
		}
		if len(testers) == 0 {
			return fmt.Errorf("no testers: set %s, or %s to name:code:daily entries separated by ;", server.EnvTable, EnvTesters)
		}
		srv.Ledger = server.NewMemory(testers)
		who = fmt.Sprintf("%d tester(s)", len(testers))
	}

	srv.Console = server.ConsoleFor(srv.Ledger)
	if srv.Console != nil {
		logger.Printf("console at http://%s%s", addr, server.ConsolePath)
	}

	httpServer := &http.Server{Addr: addr, Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdown)
	}()

	logger.Printf("listening on %s for %s; %s", addr, who, described)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
