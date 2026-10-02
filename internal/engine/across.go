package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vedansh/vans/internal/names"
	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/uiact"
)

func (e *Engine) nativeScreen() uiact.Screen {
	if e.native != nil {
		return e.native
	}
	return e.UIScreen
}

func (e *Engine) screenFor(ctx context.Context, app string) *Engine {
	c := *e
	c.native = e.nativeScreen()
	if e.canDrivePages() && e.isPageBrowser(ctx, app) {
		c.UIScreen = e.PageScreen
		c.onPage = true
		return &c
	}
	c.UIScreen = c.native
	c.onPage = false
	return &c
}

type keyPresser interface {
	PressKey(code int, flags uint64)
}

const escapeKey = 53

const strayChooser = "that opened a file chooser, which was closed again because clicking cannot work one; " +
	"use a choose step on this control with the file\u2019s full path, or ask the person which file"

func (e *Engine) closedAStrayChooser(ctx context.Context) bool {
	if !e.onPage || e.native == nil {
		return false
	}
	presser, ok := e.native.(keyPresser)
	if !ok {
		return false
	}
	seen, err := e.native.Capture(ctx, e.pageBrowser(ctx))
	if err != nil || !uiact.HasChooser(seen) {
		return false
	}
	presser.PressKey(escapeKey, 0)
	e.logf("a click opened a file chooser, so it was closed; a file is given with a choose step")
	return true
}

func (e *Engine) openApps() []string {
	if e.Apps == nil {
		return nil
	}
	on := *e
	on.UIScreen = e.nativeScreen()
	return on.runningApps(namesOf(e.Apps.Installed()))
}

func (e *Engine) appNamed(name string) string {
	name = strings.TrimSpace(name)
	if e.Apps == nil {
		return name
	}
	if app, err := e.Apps.FindInstalled(name); err == nil {
		return app.Name
	}
	return name
}

func (e *Engine) isOpen(app string) bool {
	on := *e
	on.UIScreen = e.nativeScreen()
	return on.isRunning(app)
}

func (e *Engine) whereToSwitch(ctx context.Context, step uiact.Step, from string, acting bool) (string, string) {
	name := e.appNamed(step.App)
	if name == "" {
		return "", "it switches application without saying which"
	}
	if names.Same(name, from) {
		return "", fmt.Sprintf("%s is already the application being worked in, so there is nothing to switch to", name)
	}
	if acting {
		if where, no := forbidden(e.rules(ctx), name); no {
			return "", fmt.Sprintf("your rules say VANS never acts in %s, so it cannot be switched to; say stuck", where)
		}
	}
	return name, ""
}

func expandHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return path
}

var statFile = os.Stat

func fileToChoose(path, said string) (string, string) {
	full := filepath.Clean(expandHome(path))
	if !filepath.IsAbs(full) {
		return "", fmt.Sprintf("%q is not a full path to a file; ask the person where the file is", path)
	}
	name := filepath.Base(full)
	if !strings.Contains(strings.ToLower(said), strings.ToLower(name)) {
		return "", fmt.Sprintf("the person never named a file called %q, and only a file they named may be chosen; ask which file they mean", name)
	}
	info, err := statFile(full)
	if err != nil {
		return "", fmt.Sprintf("there is no file at %s; ask the person where it is", full)
	}
	if info.IsDir() {
		return "", fmt.Sprintf("%s is a folder, and a file is what gets chosen; ask which file in it", full)
	}
	return full, ""
}

var appWordsToIgnore = map[string]bool{
	"home": true, "phone": true, "apps": true, "tips": true, "news": true, "games": true, "books": true,
	"siri": true, "clock": true, "music": true, "photos": true, "maps": true, "weather": true, "stocks": true,
	"preview": true, "journal": true, "passwords": true, "calendar": true, "contacts": true,
}

var wordEdges = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func (e *Engine) appsNamedIn(text string) []string {
	if e.Apps == nil {
		return nil
	}
	padded := " " + strings.Join(wordEdges.Split(strings.ToLower(text), -1), " ") + " "
	var out []string
	seen := map[string]bool{}
	for _, app := range e.Apps.Installed() {
		name := strings.ToLower(names.Clean(app.Name))
		if name == "" || appWordsToIgnore[name] || seen[name] {
			continue
		}
		plain := " " + strings.Join(wordEdges.Split(name, -1), " ") + " "
		if strings.Contains(padded, plain) {
			seen[name] = true
			out = append(out, app.Name)
		}
	}
	return out
}

func (e *Engine) spansApps(text string) bool {
	return len(e.appsNamedIn(text)) >= 2
}

func (e *Engine) acrossApps(ctx context.Context, in Ask, r route.Route, shape string) (*Plan, bool, error) {
	if !e.canDrive() {
		return nil, false, nil
	}
	named := e.appsNamedIn(in.Instruction)
	if len(named) < 2 {
		return nil, false, nil
	}

	start := ""
	if slot := e.appNamed(r.Slot("app")); slot != "" && e.isOpen(slot) {
		start = slot
	}
	for _, app := range named {
		if start == "" && e.isOpen(app) {
			start = app
		}
	}
	if start == "" {
		return nil, true, &uiact.NotRunningError{App: named[0]}
	}
	if e.isPageBrowser(ctx, start) {
		return nil, false, nil
	}

	e.logf("this goes across %s, so it is worked through on screen, starting in %s", strings.Join(named, " and "), start)
	plan, err := e.driveOn(ctx, start, strings.TrimSpace(in.Instruction), shape, r.Literals)
	return plan, true, err
}

const finderSelection = `tell application "Finder"
	set picked to selection as alias list
	set out to ""
	repeat with f in picked
		set out to out & POSIX path of f & linefeed
	end repeat
	return out
end tell`

const mostFilesNoted = 10

func (e *Engine) filesSelectedIn(ctx context.Context, at *screen.Context) []string {
	if at == nil || e.Runner == nil || !names.Same(at.AppName, "Finder") {
		return nil
	}
	out, err := e.Runner.Run(ctx, finderSelection)
	if err != nil {
		e.logf("could not read what is selected in Finder: %v", err)
		return nil
	}
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" && len(files) < mostFilesNoted {
			files = append(files, line)
		}
	}
	return files
}

func filesNote(files []string) string {
	quoted := make([]string, len(files))
	for i, f := range files {
		quoted[i] = fmt.Sprintf("%q", f)
	}
	return "When they asked, they were looking at Finder with these files selected: " + strings.Join(quoted, ", ") +
		". Words like \"this file\", \"these\" and \"it\" in the task mean those files."
}
