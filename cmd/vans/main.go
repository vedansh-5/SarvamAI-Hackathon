// Command vans runs one extraction task end to end.
//
// This is a development harness, not the product surface. D-007 settled that
// V1 ships as a GUI, and "CLI-first" is on the rejected list — the permissions
// story and the confirmation gate both want a window. What this exists for is
// running the real pipeline against a real browser and a real Notes.app before
// that window exists, so the engine is exercised by something other than its
// own tests.
//
// It launches nothing. Chrome must already be running with a debugging port
// open, and Notes must already have been granted automation access.
//
//	go run ./cmd/vans \
//	  -url https://news.ycombinator.com \
//	  -intent "the title, link and score of each story"
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vedansh/vans/internal/adapter/appleevents"
	"github.com/vedansh/vans/internal/engine"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "\nvans: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		pageURL  = flag.String("url", "", "page to extract from (required)")
		intent   = flag.String("intent", "", "what to extract, in plain language (required)")
		title    = flag.String("title", "", "note title (defaults to the intent and site)")
		hostPort = flag.String("cdp", "127.0.0.1:9222", "debugging endpoint of an already-running Chromium")
		dbPath   = flag.String("db", defaultDB(), "program cache")
		dryRun   = flag.Bool("dry-run", false, "extract and print, never offer to write")
		verbose  = flag.Bool("v", false, "log what the runtime is doing")
		timeout  = flag.Duration("timeout", 3*time.Minute, "overall deadline")
	)
	flag.Parse()

	if *pageURL == "" || *intent == "" {
		flag.Usage()
		return errors.New("-url and -intent are required")
	}

	// Ctrl-C has to reach the engine, not just kill the process — a cancelled
	// context is the difference between stopping cleanly before a write and
	// stopping halfway through one.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	synthesizer, err := synth.FromEnv()
	if err != nil {
		return fmt.Errorf("%w\n\nSet %s to an OpenRouter key, or %s=anthropic with ANTHROPIC_API_KEY",
			err, synth.EnvAPIKey, synth.EnvProvider)
	}
	counted := synth.NewCounter(synthesizer)

	st, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	browser := engine.NewChromiumBrowser(*hostPort)
	defer browser.Close()

	eng := &engine.Engine{
		Browser: browser,
		Store:   st,
		Synth:   counted,
		Sinks:   engine.DefaultSinks(engine.NotesSink{Notes: appleevents.Notes{Runner: appleevents.Osascript{}}}),
		Model:   modelLabel(),
	}
	if *verbose {
		eng.Log = func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "  · "+format+"\n", args...)
		}
	}

	started := time.Now()
	plan, err := eng.Prepare(ctx, engine.Task{URL: *pageURL, Intent: *intent, Title: *title})
	if err != nil {
		return explain(err)
	}

	doc := engine.Document{Title: planTitle(plan, *title), Fields: plan.Fields, Rows: plan.Rows}

	fmt.Println()
	fmt.Print(doc.Preview(10))
	fmt.Printf("\n%d rows in %s — %s, %d model call(s)\n",
		len(plan.Rows), time.Since(started).Round(time.Millisecond),
		source(plan), plan.SynthesisCalls)

	if *dryRun {
		return nil
	}

	// The gate. Nothing above this line touched anything outside the browser.
	ok, err := confirm(fmt.Sprintf("\nWrite these %d rows to Notes?", len(plan.Rows)))
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println("Cancelled. Nothing was written.")
		return nil
	}

	receipt, err := eng.Commit(ctx, plan)
	if err != nil {
		return err
	}

	fmt.Printf("Wrote %q to Notes (id %s)", receipt.Title, receipt.ID)
	if receipt.Verified {
		fmt.Print(" — read back and confirmed")
	}
	fmt.Println()
	return nil
}

// explain turns rung 3 into something a person can act on.
func explain(err error) error {
	var unresolved *engine.UnresolvedError
	if !errors.As(err, &unresolved) {
		return err
	}

	fmt.Fprintf(os.Stderr, "\nCould not extract from %s\n", unresolved.PageURL)
	fmt.Fprintf(os.Stderr, "page signature: %s\n", unresolved.Signature)

	for i, a := range unresolved.Attempts {
		fmt.Fprintf(os.Stderr, "\nattempt %d — %s, root %q\n", i+1, a.Source, a.Program.Root)
		for _, r := range a.Reasons {
			fmt.Fprintf(os.Stderr, "  · %s\n", r)
		}
	}

	if len(unresolved.Candidates) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d repeating regions were found on the page:\n", len(unresolved.Candidates))
		for _, c := range unresolved.Candidates {
			fmt.Fprintf(os.Stderr, "  · %s (%d records)\n", c.Selector, c.RecordCount)
		}
		fmt.Fprintln(os.Stderr, "\nTry rewording -intent to describe one of these.")
	}
	return errors.New("extraction failed")
}

func confirm(prompt string) (bool, error) {
	fmt.Printf("%s [y/N] ", prompt)

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		// A closed stdin is not consent. Defaulting to yes here would make
		// every piped invocation write silently.
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func source(p *engine.Plan) string {
	if p.FromCache {
		return "from cache"
	}
	return "freshly synthesized"
}

func planTitle(p *engine.Plan, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	return fmt.Sprintf("%s — %s", p.Task.Intent, p.PageURL)
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
