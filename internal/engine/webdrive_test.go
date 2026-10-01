package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type pageFake struct {
	host       string
	moves      int
	acts       int
	captures   int
	opened     []string
	hostOnOpen string
	typedAfter int
}

func (p *pageFake) Capture(context.Context, string) (uiact.Tree, error) {
	p.captures++
	p.moves++
	tree := editorTree()
	tree.App = p.host
	tree.Window = p.host
	if p.typedAfter > 0 && p.captures > p.typedAfter {
		tree.Elements[0].Value = "hello"
	}
	tree.Elements = append(tree.Elements, uiact.Element{
		Role: "AXStaticText", Title: fmt.Sprintf("change %d", p.moves), Enabled: true,
		Position: []int{0, 0}, Size: []int{10, 10}, Path: []int{0, p.moves + 1},
	})
	return tree, nil
}

func (p *pageFake) Act(_ context.Context, program uiact.Program, t uiact.Tree) error {
	p.acts++
	return uiact.Validate(program, t)
}

func (p *pageFake) Frontmost(context.Context) (string, error)  { return "Google Chrome", nil }
func (p *pageFake) BringToFront(context.Context, string) error { return nil }
func (p *pageFake) Type(context.Context, string) error         { return nil }

func (p *pageFake) Open(_ context.Context, u string) error {
	p.opened = append(p.opened, u)
	if p.hostOnOpen != "" {
		p.host = p.hostOnOpen
	}
	return nil
}

type untouchable struct{ t *testing.T }

func (u untouchable) Capture(context.Context, string) (uiact.Tree, error) {
	u.t.Error("a page task read the desktop window")
	return uiact.Tree{}, nil
}
func (u untouchable) Act(context.Context, uiact.Program, uiact.Tree) error {
	u.t.Error("a page task acted on the desktop window")
	return nil
}
func (u untouchable) Frontmost(context.Context) (string, error)  { return "Google Chrome", nil }
func (u untouchable) BringToFront(context.Context, string) error { return nil }
func (u untouchable) Type(context.Context, string) error         { return nil }

func TestSitesAreFoundInWhatWasAsked(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"book a flight on makemytrip to goa", "https://www.makemytrip.com"},
		{"book a flight on Make My Trip", "https://www.makemytrip.com"},
		{"how do I upload a video on youtube", "https://www.youtube.com"},
		{"open an issue on github.com", "https://github.com"},
		{"find shoes on shop.example.com.", "https://shop.example.com"},
		{"how do I change the wallpaper", ""},
		{"multiply x by two", ""},
	} {
		_, got, _ := siteIn(c.text)
		if got != c.want {
			t.Errorf("siteIn(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

func TestAPageTaskIsWorkedThroughOnThePageNotTheDesktop(t *testing.T) {
	page := &pageFake{host: "example.com"}
	e := &Engine{
		UIScreen: untouchable{t}, PageScreen: page,
		Steps: &scriptedSteps{answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true, Why: "typed"}}},
	}

	plan, err := e.driveOnPage(context.Background(), "say hello", "say hello", nil, "")
	if err != nil {
		t.Fatalf("driveOnPage: %v", err)
	}
	if !plan.Drive.Web || plan.Drive.Browser != "Google Chrome" || plan.Drive.App != "example.com" {
		t.Errorf("plan = %+v, want a page task on example.com in Chrome", plan.Drive)
	}

	receipt, err := e.runDrive(context.Background(), plan)
	if err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if page.acts != 1 || !receipt.Verified {
		t.Errorf("acted %d time(s), verified=%v; want the one step, seen to land", page.acts, receipt.Verified)
	}
}

func TestASiteThatIsNotInFrontIsOpenedOnlyAfterConfirming(t *testing.T) {
	page := &pageFake{host: "news.example.org", hostOnOpen: "makemytrip.com"}
	planner := &fakePlanner{steps: []uiact.Step{typeIntoMessage()}}
	e := &Engine{
		UIScreen: untouchable{t}, PageScreen: page, Plans: planner,
		Steps: &countingSteps{},
	}

	plan, err := e.driveOnPage(context.Background(), "say hello", "say hello", nil, "makemytrip")
	if err != nil {
		t.Fatalf("driveOnPage: %v", err)
	}
	if len(page.opened) != 0 || planner.calls != 0 {
		t.Errorf("opened %v and planned %d time(s) before anyone confirmed", page.opened, planner.calls)
	}
	if plan.Drive.Open != "https://www.makemytrip.com" || !strings.Contains(plan.Drive.Preview, "Open makemytrip.com") {
		t.Errorf("plan = %+v, want opening the site named as the first thing it does", plan.Drive)
	}

	if _, err := e.runDrive(context.Background(), plan); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(page.opened) != 1 || planner.calls != 2 || page.acts != 1 {
		t.Errorf("opened %v, planned %d, acted %d; want one tab, the plan and its check, and one step", page.opened, planner.calls, page.acts)
	}
}

func TestASiteNobodyKnowsTheAddressOfIsNotGuessed(t *testing.T) {
	page := &pageFake{host: "news.example.org"}
	e := &Engine{PageScreen: page, Steps: &countingSteps{}}

	_, err := e.driveOnPage(context.Background(), "buy socks", "buy socks", nil, "sockbarn")
	if err == nil || !strings.Contains(err.Error(), "open it in Google Chrome") {
		t.Errorf("err = %v, want to be told to open it rather than a guessed address", err)
	}
	if len(page.opened) != 0 {
		t.Errorf("opened %v", page.opened)
	}
}

func TestTheCoachPointsAtControlsOnAPage(t *testing.T) {
	quickGuide(t)
	page := &pageFake{host: "example.com", typedAfter: 4}
	e := &Engine{
		UIScreen: untouchable{t}, PageScreen: page,
		Steps: &scriptedSteps{answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}}},
	}

	plan, err := e.planGuideFor(context.Background(), "how do I say hello", "how do i say hello", nil,
		&screen.Context{AppName: "Google Chrome"})
	if err != nil {
		t.Fatalf("planGuideFor: %v", err)
	}
	if !plan.Guide.Web || plan.Guide.Browser != "Google Chrome" || plan.Guide.App != "example.com" {
		t.Fatalf("guide = %+v, want a page guide on example.com", plan.Guide)
	}

	var shown []GuideStep
	if _, err := e.RunGuide(context.Background(), plan, func(s GuideStep) {
		shown = append(shown, s)
	}, nil); err != nil {
		t.Fatalf("RunGuide: %v", err)
	}
	if len(shown) == 0 || shown[0].W != 80 || !strings.Contains(shown[0].Label, "Message") {
		t.Errorf("shown = %+v, want the page's control pointed at", shown)
	}
	if page.acts != 0 {
		t.Errorf("acted %d time(s); the coach only shows", page.acts)
	}
}

func TestTheCoachAsksForTheSiteWhenItIsNotInFront(t *testing.T) {
	quickGuide(t)
	page := &pageFake{host: "news.example.org"}
	e := &Engine{
		UIScreen: untouchable{t}, PageScreen: page,
		Steps: &scriptedSteps{answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}}},
	}

	plan, err := e.planGuideFor(context.Background(), "how do I upload a video on youtube", "", nil,
		&screen.Context{AppName: "Google Chrome"})
	if err != nil {
		t.Fatalf("planGuideFor: %v", err)
	}
	if plan.Guide.Site != "https://www.youtube.com" {
		t.Fatalf("site = %q, want youtube named as where to go", plan.Guide.Site)
	}

	var shown []GuideStep
	_, err = e.RunGuide(context.Background(), plan, func(s GuideStep) {
		shown = append(shown, s)
		if s.Do == "open" {
			page.host = "youtube.com"
			page.typedAfter = page.captures + 4
		}
	}, nil)
	if err != nil {
		t.Fatalf("RunGuide: %v", err)
	}
	if len(shown) < 2 || shown[0].Do != "open" || !strings.Contains(shown[0].Label, "youtube.com") {
		t.Fatalf("shown = %+v, want going to the site as step 1", shown)
	}
	if last := shown[len(shown)-1]; last.Number != 2 {
		t.Errorf("the control was step %d, want 2, after going to the site", last.Number)
	}
}

func TestWithoutAPageScreenABrowserStillCannotBeCoached(t *testing.T) {
	e := &Engine{UIScreen: &fakeScreen{tree: editorTree()}, Steps: &scriptedSteps{}}
	_, err := e.planGuideFor(context.Background(), "how do I sign in", "", nil, &screen.Context{AppName: "Google Chrome"})
	if err == nil {
		t.Fatal("coached a browser with no way to read its page")
	}
}

type profilePage struct {
	pageFake
	openedIn []string
}

func (p *profilePage) OpenIn(_ context.Context, u, profile string) error {
	p.openedIn = append(p.openedIn, profile+" "+u)
	p.host = "makemytrip.com"
	return nil
}

func TestTheProfileIsTakenOutOfTheSentence(t *testing.T) {
	for _, c := range []struct{ in, rest, profile string }{
		{"book a flight on makemytrip in a new profile", "book a flight on makemytrip", "new"},
		{"search for shoes on amazon using a separate chrome profile", "search for shoes on amazon", "new"},
		{"In a clean browser, look up flights on makemytrip", "look up flights on makemytrip", "new"},
		{"look up flights on makemytrip in incognito", "look up flights on makemytrip", "private"},
		{"open youtube in a private window and search for lofi", "open youtube and search for lofi", "private"},
		{"book a flight on makemytrip", "book a flight on makemytrip", ""},
		{"change my profile picture on github", "change my profile picture on github", ""},
		{"edit my new profile on linkedin", "edit my new profile on linkedin", ""},
	} {
		rest, profile := withoutTheProfile(c.in)
		if profile != c.profile || (c.profile != "" && rest != c.rest) {
			t.Errorf("%q -> %q, %q; want %q, %q", c.in, rest, profile, c.rest, c.profile)
		}
	}
}

func TestATaskInANewProfileOpensThereAndOnlyAfterConfirming(t *testing.T) {
	page := &profilePage{pageFake: pageFake{host: "makemytrip.com"}}
	planner := &fakePlanner{steps: []uiact.Step{typeIntoMessage()}}
	e := &Engine{UIScreen: untouchable{t}, PageScreen: page, Plans: planner, Steps: &countingSteps{}}

	plan, err := e.driveOnPageIn(context.Background(), "say hello", "say hello", nil, "makemytrip", "new")
	if err != nil {
		t.Fatalf("driveOnPageIn: %v", err)
	}
	if plan.Drive.Profile != "new" || plan.Drive.Open == "" || !strings.Contains(plan.Drive.Preview, "separate Chrome profile") {
		t.Fatalf("plan = %+v, want it to say it opens in a separate profile, even with the site already in front", plan.Drive)
	}
	if len(page.openedIn) != 0 {
		t.Errorf("opened %v before anyone confirmed", page.openedIn)
	}

	if _, err := e.runDrive(context.Background(), plan); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(page.openedIn) != 1 || page.openedIn[0] != "new https://www.makemytrip.com" || len(page.opened) != 0 {
		t.Errorf("opened in %v and plainly %v; want one window, in the new profile", page.openedIn, page.opened)
	}
}
