package server

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/vedansh/vans/internal/synth"
)

const EnvTotal = "VANS_TOTAL_DAILY"

func ModelsFromEnv() (func(used *synth.Usage) synth.Model, string, error) {
	wait, mode := synth.WaitAndModeFromEnv()
	pool, err := synth.PoolFrom(os.Getenv, wait)
	if err != nil {
		return nil, "", err
	}
	if pool != nil {
		var seats []string
		for _, s := range pool.Seats {
			seats = append(seats, s.Name())
		}
		return pool.With, "models " + strings.Join(seats, ", "), nil
	}

	key := strings.TrimSpace(os.Getenv(synth.EnvAPIKey))
	if key == "" {
		return nil, "", fmt.Errorf("set %s, or %s and its providers", synth.EnvAPIKey, synth.EnvProviders)
	}
	base := synth.FromSettings(synth.WithBackupFrom(os.Getenv, synth.Settings{
		APIKey:   key,
		BaseURL:  os.Getenv(synth.EnvBaseURL),
		Model:    os.Getenv(synth.EnvModel),
		Fast:     os.Getenv(synth.EnvFastModel),
		Fallback: os.Getenv(synth.EnvFallbackModel),
	}), wait, mode)
	backup := "none"
	if base.Backup != nil {
		backup = base.Backup.Model
	}
	return func(used *synth.Usage) synth.Model {
		c := *base
		c.Usage = used
		return &c
	}, fmt.Sprintf("model %s, backup %s", base.Model, backup), nil
}

func TotalFromEnv() (int, error) {
	spelled := strings.TrimSpace(os.Getenv(EnvTotal))
	if spelled == "" {
		return 0, nil
	}
	total, err := strconv.Atoi(spelled)
	if err != nil || total < 0 {
		return 0, fmt.Errorf("%s must be a whole number", EnvTotal)
	}
	return total, nil
}
