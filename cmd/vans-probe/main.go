package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/adapter/appleevents"
	"github.com/vedansh/vans/internal/names"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/uiact"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: vans-probe [application ...]")
		fmt.Fprintln(os.Stderr, "Reads each application's front window and says how much of it VANS can use.")
		fmt.Fprintln(os.Stderr, "With no names, every application with a window on screen is read.")
		fmt.Fprintln(os.Stderr, "Each one is brought forward for a moment, because macOS only lists the windows of the application in front.")
	}
	tasks := flag.Bool("tasks", false, "list the most recent tasks and how many model calls each took, instead of reading applications")
	flag.Parse()

	ctx := context.Background()
	if *tasks {
		if err := listTasks(ctx); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	screen := uiact.System{Runner: appleevents.Osascript{}}

	apps := flag.Args()
	if len(apps) == 0 {
		for _, name := range screen.Running() {
			if skip(name) {
				continue
			}
			apps = append(apps, name)
		}
	}
	if len(apps) == 0 {
		fmt.Fprintln(os.Stderr, "no applications with a window on screen")
		os.Exit(1)
	}

	was, _ := screen.Frontmost(ctx)
	named := len(flag.Args()) > 0
	skipped := 0
	var found []uiact.Assessment
	for _, app := range apps {
		if err := screen.BringToFront(ctx, app); err != nil {
			if !named {
				skipped++
				continue
			}
			found = append(found, uiact.Assessment{App: app, Verdict: uiact.Blind, Why: err.Error()})
			continue
		}
		time.Sleep(350 * time.Millisecond)
		tree, err := screen.Capture(ctx, app)
		if err != nil {
			if !named {
				skipped++
				continue
			}
			found = append(found, uiact.Assessment{App: app, Verdict: uiact.Blind, Why: err.Error()})
			continue
		}
		a := uiact.Assess(tree)
		a.App = app
		found = append(found, a)
	}
	if was != "" {
		_ = screen.BringToFront(ctx, was)
	}

	order := map[string]int{uiact.Ready: 0, uiact.Partly: 1, uiact.Blind: 2}
	sort.SliceStable(found, func(i, j int) bool { return order[found[i].Verdict] < order[found[j].Verdict] })

	fmt.Printf("%-28s %-7s %8s %6s %5s  %s\n", "APPLICATION", "VERDICT", "CONTROLS", "NAMED", "TYPE", "WHY")
	for _, a := range found {
		fmt.Printf("%-28s %-7s %8d %6d %5d  %s\n", clip(a.App, 28), a.Verdict, a.Controls, a.Named, a.Typeable, a.Why)
	}
	if skipped > 0 {
		fmt.Printf("\n%d background helper(s) with no window were left out.\n", skipped)
	}
}

var notApps = []string{"vans", "Window Server", "Dock", "Control Centre", "Control Center", "Notification Centre",
	"Notification Center", "Spotlight", "SystemUIServer", "TextInputMenuAgent", "loginwindow", "Wallpaper", "WindowManager"}

func skip(name string) bool {
	for _, n := range notApps {
		if names.Same(name, n) {
			return true
		}
	}
	return false
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func listTasks(ctx context.Context) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	st, err := store.Open(filepath.Join(home, ".vans", "programs.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	runs, err := st.RecentTasks(ctx, 30)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		fmt.Println("no tasks recorded yet")
		return nil
	}
	fmt.Printf("%-12s %-5s %-22s %5s %8s %5s %8s  %s\n", "WHEN", "MODE", "WHERE", "STEPS", "FROM MEM", "CALLS", "TOKENS", "TASK")
	for _, r := range runs {
		fmt.Printf("%-12s %-5s %-22s %5d %8d %5d %8d  %s\n",
			r.At.Format("Jan 2 15:04"), r.Mode, clip(r.App, 22), r.Steps, r.Replayed, r.Calls, r.TokensIn+r.TokensOut, clip(r.Shape, 60))
		if r.Failed != "" {
			fmt.Printf("%-12s       failed: %s\n", "", clip(r.Failed, 140))
		}
	}
	return nil
}
