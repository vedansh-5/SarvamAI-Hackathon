package engine

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/synth"
)

const MaxWebSteps = 24

type PageOpener interface {
	Open(ctx context.Context, pageURL string) error
}

type ProfileOpener interface {
	OpenIn(ctx context.Context, pageURL, profile string) error
}

var (
	inANewProfile = regexp.MustCompile(`(?i)[\s,]*\b(in|using|with|on)\s+(a\s+|an\s+|the\s+)?(new|fresh|clean|separate|different|vans)\s+(chrome\s+)?(profile|browser)\b`)
	inAPrivateOne = regexp.MustCompile(`(?i)[\s,]*\b(in|using|with)\s+(a\s+|an\s+)?(incognito|private)(\s+(window|mode|tab|browser))?\b`)
	profilesNamed = map[string]string{"new": "a separate Chrome profile", "private": "a private window"}
)

func withoutTheProfile(instruction string) (string, string) {
	if inAPrivateOne.MatchString(instruction) {
		return strings.Trim(inAPrivateOne.ReplaceAllString(instruction, ""), " ,.;"), "private"
	}
	if inANewProfile.MatchString(instruction) {
		return strings.Trim(inANewProfile.ReplaceAllString(instruction, ""), " ,.;"), "new"
	}
	return instruction, ""
}

func (e *Engine) canDrivePages() bool {
	return e.Steps != nil && e.PageScreen != nil
}

func (e *Engine) onThePage() *Engine {
	c := *e
	c.native = e.nativeScreen()
	c.UIScreen = e.PageScreen
	c.onPage = true
	return &c
}

type browserChooser interface {
	Handles(app string) bool
}

func (e *Engine) isPageBrowser(ctx context.Context, app string) bool {
	if e.PageScreen == nil || strings.TrimSpace(app) == "" {
		return false
	}
	if many, ok := e.PageScreen.(browserChooser); ok {
		return many.Handles(app)
	}
	return strings.EqualFold(app, e.pageBrowser(ctx))
}

func (e *Engine) pageBrowser(ctx context.Context) string {
	if e.PageScreen == nil {
		return ""
	}
	name, _ := e.PageScreen.Frontmost(ctx)
	return name
}

var moreSites = map[string]string{
	"makemytrip":     "https://www.makemytrip.com",
	"goibibo":        "https://www.goibibo.com",
	"cleartrip":      "https://www.cleartrip.com",
	"irctc":          "https://www.irctc.co.in",
	"redbus":         "https://www.redbus.in",
	"bookmyshow":     "https://in.bookmyshow.com",
	"booking":        "https://www.booking.com",
	"airbnb":         "https://www.airbnb.com",
	"amazon":         "https://www.amazon.in",
	"flipkart":       "https://www.flipkart.com",
	"github":         "https://github.com",
	"reddit":         "https://www.reddit.com",
	"twitter":        "https://x.com",
	"x":              "https://x.com",
	"instagram":      "https://www.instagram.com",
	"facebook":       "https://www.facebook.com",
	"youtubestudio":  "https://studio.youtube.com",
	"googlecalendar": "https://calendar.google.com",
	"googledocs":     "https://docs.google.com",
	"googlemaps":     "https://maps.google.com",
}

var domainLike = regexp.MustCompile(`^[a-z0-9-]+(\.[a-z0-9-]+)+$`)

func siteAddress(name string) (string, bool) {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	trimmed = strings.TrimPrefix(strings.TrimPrefix(trimmed, "https://"), "http://")
	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" {
		return "", false
	}
	if domainLike.MatchString(trimmed) {
		return "https://" + trimmed, true
	}
	if target, ok := webVersionOf(trimmed); ok {
		return target, true
	}
	key := strings.NewReplacer(" ", "", "-", "", "_", "", ".", "").Replace(trimmed)
	target, ok := moreSites[key]
	return target, ok
}

func siteIn(text string) (name, address string, ok bool) {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '.' && r != '-'
	})
	for i := range words {
		words[i] = strings.Trim(words[i], ".-")
	}
	for span := 3; span >= 1; span-- {
		for i := 0; i+span <= len(words); i++ {
			candidate := strings.Join(words[i:i+span], " ")
			if span == 1 && len(candidate) < 2 {
				continue
			}
			if span == 1 && candidate == "x" {
				continue
			}
			if address, ok := siteAddress(candidate); ok {
				return candidate, address, true
			}
		}
	}
	return "", "", false
}

func (e *Engine) driveOnPage(ctx context.Context, task, shape string, literals map[string]string, site string) (*Plan, error) {
	return e.driveOnPageIn(ctx, task, shape, literals, site, "")
}

func (e *Engine) driveOnPageIn(ctx context.Context, task, shape string, literals map[string]string, site, profile string) (*Plan, error) {
	if !e.canDrivePages() {
		return nil, fmt.Errorf("engine: this build cannot work through a task on a page")
	}
	web := e.onThePage()
	browser := e.pageBrowser(ctx)

	tree, err := e.PageScreen.Capture(ctx, browser)
	if err != nil {
		return nil, err
	}

	if profile != "" {
		if _, ok := e.PageScreen.(ProfileOpener); !ok {
			return nil, fmt.Errorf("engine: this build cannot open another browser profile")
		}
		target, known := siteAddress(site)
		if !known {
			if site != "" {
				return nil, fmt.Errorf("engine: the address of %s is not known; say it as an address, like example.com", site)
			}
			target = "https://" + tree.App
		}
		return &Plan{
			Task:        Task{Intent: task, Title: hostOf(target)},
			Destination: browser,
			PageURL:     target,
			Drive: &DrivePlan{
				App: hostOf(target), Task: task, Shape: shape, Literals: literals,
				Web: true, Browser: browser, Open: target, Profile: profile,
				Preview: fmt.Sprintf(
					"Open %s in %s, then work through this on the page, one step at a time:\n\n  %s\n\n"+
						"Up to %d steps. Nothing there is signed in unless you have signed in to that profile yourself.",
					hostOf(target), profilesNamed[profile], task, MaxWebSteps),
			},
		}, nil
	}

	if site != "" && !pageBelongsTo("https://"+tree.App, site) {
		target, known := siteAddress(site)
		if !known {
			return nil, fmt.Errorf("engine: %s is not the page in front; open it in %s, then ask again", site, browser)
		}
		if _, ok := e.PageScreen.(PageOpener); !ok {
			return nil, fmt.Errorf("engine: this build cannot open %s", hostOf(target))
		}
		return &Plan{
			Task:        Task{Intent: task, Title: hostOf(target)},
			Destination: browser,
			PageURL:     target,
			Drive: &DrivePlan{
				App: hostOf(target), Task: task, Shape: shape, Literals: literals,
				Web: true, Browser: browser, Open: target,
				Preview: fmt.Sprintf(
					"Open %s in a new tab in %s, then work through this on the page, one step at a time:\n\n  %s\n\n"+
						"Up to %d steps. Every click and every keystroke happens where you can watch it.",
					hostOf(target), browser, task, MaxWebSteps),
			},
		}, nil
	}

	plan, err := web.driveOn(ctx, browser, task, shape, literals)
	if err != nil {
		return nil, err
	}
	plan.Drive.Web = true
	plan.Drive.Browser = browser
	plan.Destination = browser
	return plan, nil
}

func (e *Engine) openThePage(ctx context.Context, d *DrivePlan) error {
	opener, ok := e.UIScreen.(PageOpener)
	if !ok {
		return fmt.Errorf("engine: this build cannot open %s", d.App)
	}
	e.logf("opening %s", d.Open)
	if d.Profile != "" {
		in, ok := e.UIScreen.(ProfileOpener)
		if !ok {
			return fmt.Errorf("engine: this build cannot open another browser profile")
		}
		if err := in.OpenIn(ctx, d.Open, d.Profile); err != nil {
			return err
		}
	} else if err := opener.Open(ctx, d.Open); err != nil {
		return err
	}

	tree, err := e.UIScreen.Capture(ctx, d.Browser)
	if err != nil {
		return err
	}
	if len(tree.Elements) == 0 {
		return stopped(d.App, "engine: %s opened with nothing on it to work with", d.App)
	}
	d.App = tree.App

	d.Replay = e.stepsFromLastTime(ctx, tree.App, d.Shape, d.Literals)
	d.Kept = len(d.Replay) > 0
	if !d.Kept {
		var planned synth.PlanAnswer
		planned, d.PlanCalls = e.planOn(ctx, synth.PlanRequest{Task: d.Task, Tree: tree, Most: MaxWebSteps})
		d.Replay, d.Whole = planned.Steps, planned.Whole
	}
	return nil
}

var siteWait = 90 * time.Second

func (e *Engine) waitForTheSite(ctx context.Context, g *GuidePlan, number int, show func(GuideStep), advance <-chan Advance) (bool, error) {
	there := func() bool {
		tree, err := e.UIScreen.Capture(ctx, g.Browser)
		return err == nil && pageBelongsTo("https://"+tree.App, hostOf(g.Site))
	}
	if there() {
		return false, nil
	}

	label := fmt.Sprintf("Go to %s in %s", hostOf(g.Site), g.Browser)
	for attempt := 1; attempt <= timesToRepeatAStep; attempt++ {
		show(GuideStep{Number: number, Label: label, Do: "open", Detail: g.Site})

		deadline := time.Now().Add(siteWait)
		for time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-advance:
			case <-time.After(guidePoll * 2):
			}
			if there() {
				e.logf("%s is in front; carrying on", hostOf(g.Site))
				return true, nil
			}
		}
	}
	return false, stopped(g.Browser, "engine: VANS stopped waiting for %s to be opened. Open it in %s, then ask again.", hostOf(g.Site), g.Browser)
}

func siteFor(task, hint string) (string, bool) {
	if address, ok := siteAddress(hint); ok {
		return address, true
	}
	_, address, ok := siteIn(task)
	return address, ok
}

func (e *Engine) planWebGuide(ctx context.Context, task, shape string, literals map[string]string, browser string, mustOpen bool, why, site string) (*Plan, error) {
	g := &GuidePlan{
		App: browser, Browser: browser, Task: task, Web: true,
		Shape: shape, Literals: literals, MustOpen: mustOpen, Why: why,
	}
	where := browser

	address, named := siteFor(task, site)
	if !mustOpen {
		tree, err := e.PageScreen.Capture(ctx, browser)
		if err != nil {
			return nil, err
		}
		g.App = tree.App
		where = tree.App
		if named && !pageBelongsTo("https://"+tree.App, hostOf(address)) {
			g.Site = address
			where = hostOf(address)
		}
	} else if named {
		g.Site = address
		where = hostOf(address)
	}

	g.Preview = fmt.Sprintf("Show you how to do this on %s, a step at a time:\n\n  %s", where, task)
	return &Plan{
		Task:        Task{Intent: task, Title: where},
		Destination: browser,
		Guide:       g,
	}, nil
}

var quotedText = regexp.MustCompile(`\{(s\d+)\}`)

func theTask(r route.Route) string {
	return withItsTarget(theAction(r), r.Slot("target"))
}

func withItsTarget(action, target string) string {
	action, target = strings.TrimSpace(action), strings.TrimSpace(target)
	if target == "" || action == "" || strings.Contains(strings.ToLower(action), strings.ToLower(target)) {
		return action
	}
	return action + " (on: " + target + ")"
}

func theAction(r route.Route) string {
	template := r.Template("action")
	if template == "" || !quotedText.MatchString(template) {
		return r.Slot("action")
	}
	template = route.InTheirCase(template, r.Instruction)
	said := quotedText.ReplaceAllStringFunc(template, func(m string) string {
		value, ok := r.Literals[quotedText.FindStringSubmatch(m)[1]]
		if !ok {
			return m
		}
		return `"` + value + `"`
	})
	return route.Fill(said, r.Literals)
}
