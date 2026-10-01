package uiact

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type Shot struct {
	JPEG       []byte
	X, Y, W, H int
}

var ErrCannotSee = errors.New("uiact: the window could not be pictured")

const CannotSeeFix = "VANS needs Screen Recording permission to look at a window: System Settings \u203A Privacy & Security \u203A Screen & System Audio Recording."

const longestShotSide = 1400

func WindowShot(ctx context.Context, r Runner, app string) (Shot, error) {
	pid, err := pidFor(ctx, r, app)
	if err != nil {
		return Shot{}, err
	}
	id, x, y, w, h, ok := windowOf(pid)
	if !ok {
		return Shot{}, ErrNoWindow
	}

	dir, err := os.MkdirTemp("", "vans-shot")
	if err != nil {
		return Shot{}, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "window.jpg")

	if out, err := exec.CommandContext(ctx, "screencapture", "-x", "-o", "-t", "jpg",
		fmt.Sprintf("-l%d", id), path).CombinedOutput(); err != nil {
		return Shot{}, fmt.Errorf("%w: %v (%s)", ErrCannotSee, err, truncate(string(out), 120))
	}
	_ = exec.CommandContext(ctx, "sips", "-Z", fmt.Sprint(longestShotSide), path).Run()

	data, err := os.ReadFile(path)
	if err != nil || len(data) < 1024 {
		return Shot{}, ErrCannotSee
	}
	return Shot{JPEG: data, X: x, Y: y, W: w, H: h}, nil
}

func (s System) Shot(ctx context.Context, app string) (Shot, error) {
	return WindowShot(ctx, s.Runner, app)
}
