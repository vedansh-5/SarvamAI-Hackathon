// Command vans-engine is the runtime, spoken to over stdin and stdout.
//
// It is spawned by the desktop shell as a child process and speaks the
// newline-delimited JSON protocol in internal/rpc. It has no network listener
// and no command-line surface for performing work: the only way to reach a
// side effect is to be the process that started it.
//
// Run it by hand to exercise the protocol:
//
//	echo '{"id":1,"method":"preflight"}' | go run ./cmd/vans-engine
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/vedansh/vans/internal/adapter/appleevents"
	"github.com/vedansh/vans/internal/adapter/cdp"
	"github.com/vedansh/vans/internal/engine"
	"github.com/vedansh/vans/internal/extract"
	"github.com/vedansh/vans/internal/rpc"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
)

func main() {
	if err := run(); err != nil {
		// Diagnostics go to stderr. stdout carries the protocol and nothing
		// else — a stray line there desynchronizes the shell's decoder.
		fmt.Fprintf(os.Stderr, "vans-engine: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		hostPort = flag.String("cdp", "127.0.0.1:9222", "debugging endpoint of an already-running Chromium")
		dbPath   = flag.String("db", defaultDB(), "program cache")
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	browser := engine.NewChromiumBrowser(*hostPort)
	defer browser.Close()

	notes := appleevents.Notes{Runner: appleevents.Osascript{}}

	// A missing key is reported by preflight rather than refused at startup.
	// The shell needs to open and explain what to fix; exiting here would give
	// it nothing to show.
	synthesizer, synthErr := synth.FromEnv()
	if synthErr != nil {
		synthesizer = unconfigured{err: synthErr}
	}

	srv := &rpc.Server{
		Engine: &engine.Engine{
			Browser: browser,
			Store:   st,
			Synth:   synthesizer,
			Sink:    engine.NotesSink{Notes: notes},
			Model:   modelLabel(),
		},
		Probe: probe{
			hostPort: *hostPort,
			notes:    notes,
			synthErr: synthErr,
		},
		HostPort: *hostPort,
	}

	return srv.Serve(ctx, os.Stdin, os.Stdout)
}

// probe answers the shell's readiness questions.
type probe struct {
	hostPort string
	notes    appleevents.Notes
	synthErr error
}

func (p probe) LLMConfigured() error { return p.synthErr }

func (p probe) BrowserReachable(ctx context.Context) error {
	_, err := cdp.BrowserWSURL(ctx, p.hostPort)
	return err
}

// NotesAccessible sends a trivial Apple event to Notes.
//
// This launches Notes and can raise the automation prompt, which is why it is
// never part of the automatic preflight — the shell calls it only when the
// user presses the button asking for it, and the button says what it will do.
// Doing it at a moment the user chose is the whole argument for onboarding in
// a window rather than discovering the denial mid-task.
func (p probe) NotesAccessible(ctx context.Context) error {
	_, err := p.notes.Runner.Run(ctx, `Application('Notes').name()`)
	return err
}

// unconfigured stands in for a synthesizer that could not be built, so the
// process can start and explain itself.
type unconfigured struct{ err error }

func (u unconfigured) Synthesize(context.Context, synth.Request) (extract.Program, error) {
	return extract.Program{}, u.err
}

func modelLabel() string {
	if m := os.Getenv(synth.EnvModel); m != "" {
		return m
	}
	if os.Getenv(synth.EnvProvider) == "anthropic" {
		return synth.DefaultModel
	}
	return synth.DefaultOSModel
}

func defaultDB() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "vans.db"
	}
	return filepath.Join(home, ".vans", "programs.db")
}
