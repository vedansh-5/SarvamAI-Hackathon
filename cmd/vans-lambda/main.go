package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/vedansh/vans/internal/server"
	"github.com/vedansh/vans/internal/synth"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "vans-lambda:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(server.Region))
	if err != nil {
		return err
	}
	if _, err := server.LoadSettings(ctx, ssm.NewFromConfig(cfg)); err != nil {
		return err
	}
	models, described, err := server.ModelsFromEnv()
	if err != nil {
		return err
	}
	total, err := server.TotalFromEnv()
	if err != nil {
		return err
	}
	table := strings.TrimSpace(os.Getenv(server.EnvTable))
	if table == "" {
		return fmt.Errorf("%s is not set", server.EnvTable)
	}
	ledger, mail, send, err := server.OnAWS(ctx, table)
	if err != nil {
		return err
	}
	joining, err := server.JoiningFromEnv(ledger, send)
	if err != nil {
		return err
	}

	logger := log.New(os.Stderr, "", 0)
	synth.Notice = func(format string, args ...any) { logger.Printf("[model] "+format, args...) }
	logger.Printf("ready for testers in %s; %s", table, described)

	if joining == nil {
		logger.Printf("no %s set; signing up by email is closed", server.EnvJoinFrom)
	}

	releases := server.ReleasesFromEnv(cfg)
	if releases == nil {
		logger.Printf("no %s set; update checks answer that there is nothing new", server.EnvReleases)
	}

	lambda.Start(server.FunctionURL(&server.Server{Model: models, Ledger: ledger, Mail: mail, Total: total, Log: logger.Printf, Console: server.ConsoleFor(ledger), Releases: releases, Joining: joining}))
	return nil
}
