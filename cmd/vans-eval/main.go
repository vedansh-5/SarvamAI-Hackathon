package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vedansh/vans/internal/envfile"
	"github.com/vedansh/vans/internal/synth"
)

type choiceCase struct {
	task    string
	inFront string
	want    string
}

var installed = []string{
	"System Settings", "Notes", "Calendar", "Mail", "Safari", "Finder", "Reminders", "Music",
	"Maps", "Photos", "Messages", "Screenshot", "Calculator", "Pages", "TextEdit", "Clock",
	"Preview", "Terminal", "App Store", "FaceTime",
}

var choices = []choiceCase{
	{"turn on dark mode", "Safari", "System Settings"},
	{"change my wallpaper", "Notes", "System Settings"},
	{"connect to a different wifi network", "Finder", "System Settings"},
	{"how much battery is left", "Mail", "System Settings"},
	{"write down my grocery list", "Safari", "Notes"},
	{"add a meeting tomorrow at 3pm", "Mail", "Calendar"},
	{"send an email to Riya about the report", "Notes", "Mail"},
	{"search the web for flights to Goa", "Finder", "Safari"},
	{"find the file called resume", "Safari", "Finder"},
	{"remind me to call mom at 6", "Notes", "Reminders"},
	{"play some music", "Safari", "Music"},
	{"get directions to the airport", "Notes", "Maps"},
	{"crop a photo I took yesterday", "Finder", "Photos"},
	{"text Arjun that I am running late", "Safari", "Messages"},
	{"work out 15 percent of 2400", "Notes", "Calculator"},
	{"set a timer for 10 minutes", "Safari", "Clock"},
	{"make this text bold", "Pages", "Pages"},
	{"change the font size here", "TextEdit", "TextEdit"},
	{"rename this file", "Finder", "Finder"},
	{"sign this PDF", "Preview", "Preview"},
}

type result struct {
	seat  string
	right int
	times []time.Duration
	wrong []string
	fails int
}

func main() {
	only := flag.String("only", "", "test only seats whose name contains this")
	gap := flag.Duration("gap", 8*time.Second, "pause between questions to one model, to stay under its per-minute limit")
	flag.Parse()
	envfile.Load(".env", "../.env")

	wait, _ := synth.WaitAndModeFromEnv()
	pool, err := synth.PoolFrom(os.Getenv, wait)
	if err != nil || pool == nil {
		fmt.Fprintln(os.Stderr, "vans-eval: set VANS_PROVIDERS and its settings first:", err)
		os.Exit(1)
	}

	var seats []synth.Seat
	for _, s := range pool.Seats {
		if s.Takes[synth.Quick] && (*only == "" || strings.Contains(s.Name(), *only)) {
			seats = append(seats, s)
		}
	}
	results := make([]result, len(seats))
	var wg sync.WaitGroup
	for i, s := range seats {
		wg.Add(1)
		go func(i int, s synth.Seat) {
			defer wg.Done()
			results[i] = score(s, *gap)
			fmt.Fprintf(os.Stderr, "done %s\n", s.Name())
		}(i, s)
	}
	wg.Wait()

	fmt.Printf("%-40s %8s %8s %8s %6s\n", "model", "right", "median", "slowest", "failed")
	for _, r := range results {
		fmt.Printf("%-40s %5d/%-2d %8s %8s %6d\n", r.seat, r.right, len(choices), median(r.times), slowest(r.times), r.fails)
	}
	for _, r := range results {
		for _, w := range r.wrong {
			fmt.Printf("  %s %s\n", r.seat, w)
		}
	}
}

func score(s synth.Seat, gap time.Duration) result {
	r := result{seat: s.Name()}
	for i, c := range choices {
		if i > 0 {
			time.Sleep(gap)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		started := time.Now()
		got, err := s.Client.WhichApp(ctx, synth.AppChoiceRequest{
			Task: c.task, InFront: c.inFront, Running: []string{c.inFront, "Finder"}, Installed: installed,
		})
		cancel()
		took := time.Since(started)
		if err != nil {
			r.fails++
			r.wrong = append(r.wrong, fmt.Sprintf("%q: failed (%v)", c.task, short(err)))
			continue
		}
		r.times = append(r.times, took)
		if strings.EqualFold(strings.TrimSpace(got.App), c.want) {
			r.right++
		} else {
			r.wrong = append(r.wrong, fmt.Sprintf("%q: said %q, want %q", c.task, got.App, c.want))
		}
	}
	return r
}

func short(err error) string {
	s := err.Error()
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

func median(ts []time.Duration) string {
	if len(ts) == 0 {
		return "-"
	}
	c := append([]time.Duration(nil), ts...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	return c[len(c)/2].Round(10 * time.Millisecond).String()
}

func slowest(ts []time.Duration) string {
	if len(ts) == 0 {
		return "-"
	}
	m := ts[0]
	for _, t := range ts {
		if t > m {
			m = t
		}
	}
	return m.Round(10 * time.Millisecond).String()
}
