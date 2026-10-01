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
	"strings"
	"syscall"

	"github.com/vedansh/vans/internal/adapter/appleevents"
	"github.com/vedansh/vans/internal/adapter/cdp"
	"github.com/vedansh/vans/internal/capability"
	"github.com/vedansh/vans/internal/engine"
	"github.com/vedansh/vans/internal/extract"
	"github.com/vedansh/vans/internal/eyes"
	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/rpc"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
	"github.com/vedansh/vans/internal/webscreen"
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
	loadEnvFile()
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

	debugged := engine.NewChromiumBrowser(*hostPort)
	defer debugged.Close()

	// The browser the user is already in comes first. The debugging port
	// reaches only a second Chrome on a throwaway profile, which is signed in
	// to nothing — the wrong browser for most of what anyone asks.
	live := &appleevents.Chrome{Runner: appleevents.Osascript{}, Remember: st}
	browser := &engine.LiveFirst{
		Live:     live,
		Debugged: debugged,
	}

	desktop := uiact.System{Runner: appleevents.Osascript{}}
	page := &webscreen.Screen{
		Page:    live,
		Browser: "Google Chrome",
		Raise:   desktop.BringToFront,
	}

	notes := appleevents.Notes{Runner: appleevents.Osascript{}}

	// A missing key is reported by preflight rather than refused at startup.
	// The shell needs to open and explain what to fix; exiting here would give
	// it nothing to show.
	synthesizer, synthErr := synth.FromEnv()
	if synthErr != nil {
		synthesizer = unconfigured{err: synthErr}
	}

	// The router shares the store, so a routing decision and an extraction
	// program for the same request live in one file and are backed up, moved
	// or deleted together.
	//
	// Nil when the backend cannot also decide: routing is a different call
	// shape from synthesis, and a backend that implements one need not
	// implement the other. Ask reports the absence rather than the engine
	// panicking on a nil router halfway through a request.
	var router *route.Router
	if decider, ok := synthesizer.(route.Decider); ok {
		router = &route.Router{Decider: decider, Cache: st, Model: modelLabel()}
		router.Log = func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "[route] "+format+"\n", args...)
		}
	}

	// Acting inside applications needs four things and takes them together:
	// the catalogue of what is installed, a backend that can write a program
	// against one, somewhere to cache it, and something to run it. A backend
	// that cannot write actions leaves Writer nil, and the engine declines
	// app.act with what it can do instead — rather than accepting the request
	// and failing partway through it.
	var writer engine.ActionWriter
	if w, ok := synthesizer.(engine.ActionWriter); ok {
		writer = w
	}

	var webWriter engine.WebActionWriter
	if w, ok := synthesizer.(engine.WebActionWriter); ok {
		webWriter = w
	}

	var uiWriter engine.UIActionWriter
	if w, ok := synthesizer.(engine.UIActionWriter); ok {
		uiWriter = w
	}

	var plans engine.PlanWriter
	if w, ok := synthesizer.(engine.PlanWriter); ok {
		plans = w
	}

	var steps engine.StepWriter
	if w, ok := synthesizer.(engine.StepWriter); ok {
		steps = w
	}

	var styler engine.StyleWriter
	if w, ok := synthesizer.(engine.StyleWriter); ok {
		styler = w
	}

	var answers engine.Answerer
	if a, ok := synthesizer.(engine.Answerer); ok {
		answers = a
	}

	var chooser engine.AppChooser
	if c, ok := synthesizer.(engine.AppChooser); ok {
		chooser = c
	}

	srv := &rpc.Server{
		Engine: &engine.Engine{
			Browser: browser,
			Store:   st,
			Synth:   synthesizer,
			Router:  router,
			Sinks:   engine.DefaultSinks(engine.NotesSink{Notes: notes}),
			Model:   modelLabel(),

			Answers:    answers,
			Apps:       capability.New(),
			Writer:     writer,
			WebWriter:  webWriter,
			Actions:    st,
			Runner:     appleevents.Osascript{},
			UIWriter:   uiWriter,
			UIActions:  st,
			Steps:      steps,
			Plans:      plans,
			Styler:     styler,
			History:    st,
			Chooser:    chooser,
			Deeds:      st,
			Recorded:   st,
			Traces:     st,
			Map:        st,
			Tasks:      st,
			UIScreen:   desktop,
			PageScreen: page,
			Eyes:       looking(synthesizer, desktop),
		},
		Deeds: st,
		Probe: probe{
			hostPort: *hostPort,
			notes:    notes,
			synthErr: synthErr,
			live:     &appleevents.Chrome{Runner: appleevents.Osascript{}},
		},
		HostPort: *hostPort,

		// The same lines the window shows, kept where they can be read after
		// the fact. stdout is the protocol and nothing else.
		Trace: os.Stderr,
	}

	return srv.Serve(ctx, os.Stdin, os.Stdout)
}

// probe answers the shell's readiness questions.
type probe struct {
	hostPort string
	notes    appleevents.Notes
	synthErr error
	live     *appleevents.Chrome
}

func (p probe) LLMConfigured() error { return p.synthErr }

// AccessibilityGranted asks whether the trust is in place, which does not
// prompt for it. Nothing here asks for the permission: a window opening is not
// a reason to put a system dialog in front of somebody (D-032).
func (p probe) AccessibilityGranted() error {
	if uiact.Trusted() {
		return nil
	}
	return uiact.ErrNoAssistiveAccess
}

// BrowserReachable prefers the browser the user is actually in, and only
// falls back to reporting on the debugged one. A ready live browser needs no
// setup at all, so saying "start Chrome with these flags" when it is right
// there would send someone to do work they do not need to do.
func (p probe) BrowserReachable(ctx context.Context) error {
	live := p.live.Reachable(ctx)
	if live == nil {
		return nil
	}
	if _, err := cdp.BrowserWSURL(ctx, p.hostPort); err == nil {
		return nil
	}
	return live
}

// BrowserFix is the instruction for whatever BrowserReachable reported, when
// there is one that is easier than launching a second browser.
func (p probe) BrowserFix(err error) string { return appleevents.Fix(err) }

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

func loadEnvFile() {
	paths := []string{".env", "../.env"}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				v = strings.Trim(v, `"'`)
				if os.Getenv(k) == "" {
					_ = os.Setenv(k, v)
				}
			}
		}
		break
	}
}

func looking(model any, desktop uiact.System) uiact.Screen {
	reader, ok := model.(eyes.Reader)
	if !ok {
		return nil
	}
	return &eyes.Screen{
		Camera: desktop,
		Reader: reader,
		Raise:  desktop.BringToFront,
		Log: func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "[eyes] "+format+"\n", args...)
		},
	}
}
