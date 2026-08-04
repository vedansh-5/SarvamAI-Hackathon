// Package appleevents executes Apple Events against scriptable macOS
// applications by generating JXA and running it through osascript.
//
// JXA is used rather than raw AppleEvents because it takes and returns JSON,
// which removes AppleScript record parsing from the picture. The cost is a
// process spawn per call (~50-100ms).
package appleevents

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runner executes a JXA script and returns its trimmed stdout.
type Runner interface {
	Run(ctx context.Context, script string) ([]byte, error)
}

// Osascript runs JXA through /usr/bin/osascript.
//
// The script is fed on stdin rather than passed with -e, which keeps shell and
// argument quoting out of the picture entirely.
type Osascript struct {
	Path    string        // defaults to /usr/bin/osascript
	Timeout time.Duration // defaults to 30s
}

func (o Osascript) Run(ctx context.Context, script string) ([]byte, error) {
	path := o.Path
	if path == "" {
		path = "/usr/bin/osascript"
	}
	timeout := o.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "-l", "JavaScript")
	cmd.Stdin = bytes.NewBufferString(script)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("appleevents: timed out after %s", timeout)
		}
		return nil, &ScriptError{Err: err, Stderr: strings.TrimSpace(stderr.String())}
	}

	return bytes.TrimSpace(stdout.Bytes()), nil
}

// ScriptError carries osascript's stderr, which holds the actual reason.
type ScriptError struct {
	Err    error
	Stderr string
}

func (e *ScriptError) Error() string {
	if e.Stderr != "" {
		return "appleevents: " + e.Stderr
	}
	return "appleevents: " + e.Err.Error()
}

func (e *ScriptError) Unwrap() error {
	return e.Err
}

// IsPermissionDenied reports whether macOS refused Automation access. This
// needs a user grant in System Settings, not a retry — the failure ladder must
// not treat it as transient.
func IsPermissionDenied(err error) bool {
	var se *ScriptError
	if !errors.As(err, &se) {
		return false
	}
	return strings.Contains(se.Stderr, "-1743") ||
		strings.Contains(se.Stderr, "Not authorized") ||
		strings.Contains(se.Stderr, "not allowed assistive access")
}
