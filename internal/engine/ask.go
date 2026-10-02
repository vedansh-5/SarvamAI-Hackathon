package engine

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/screen"
)

// Ask is a free-text request: what the user typed, and where they want it.
type Ask struct {
	// Instruction is what the user asked for, in their own words.
	Instruction string

	// Destination names where the answer goes. Empty means the panel.
	Destination string

	// Context is what the user was looking at when they asked. Optional, and
	// deliberately not required: the runtime must work when the screen cannot
	// be read, which is the ordinary case before Screen Recording is granted.
	Context *screen.Context

	Mode string
}

const (
	ModeShow     = "show"
	ModeDo       = "do"
	ModeFlip     = "flip"
	ModeMustShow = "show!"
	ModeMustDo   = "do!"
)

var teaching = regexp.MustCompile(`(?i)^\s*(please\s+)?(` +
	`how\b|where\b|which (button|menu|setting|option)\b|` +
	`(can|could|would) you (show|teach|tell|guide|walk|help)\b|` +
	`(show|teach|tell|guide|walk|help|coach) (me|us)\b|` +
	`(explain|demonstrate)\b|` +
	`i (want|need|would like|'d like) to (learn|know|see|understand)\b|` +
	`what('s| is) the (way|best way|steps?)\b)`)

var teachingAnywhere = regexp.MustCompile(`(?i)\b(step by step|show me how|teach me|walk me through|how (do|can|should|would) (i|we)|how to)\b`)

var doingAnywhere = regexp.MustCompile(`(?i)\b(do (it|this|that)( for me)?|for me|on my behalf|go ahead and|yourself)\b`)

func askedToBeShown(instruction string) string {
	if said := strings.TrimSpace(teachingAnywhere.FindString(instruction)); said != "" {
		return said
	}
	return strings.TrimSpace(teaching.FindString(instruction))
}

func asksToBeShown(instruction string) bool {
	return askedToBeShown(instruction) != ""
}

func askedToHaveItDone(instruction string) string {
	if asksToBeShown(instruction) {
		return ""
	}
	return strings.TrimSpace(doingAnywhere.FindString(instruction))
}

func isTask(k route.Kind) bool {
	return k == route.KindAppAction || k == route.KindPageAction || k == route.KindAppGuide
}

func said(words string) string {
	return fmt.Sprintf("you said \u201C%s\u201D", strings.ToLower(words))
}

func (e *Engine) wantsShowing(in Ask, k route.Kind) (bool, string) {
	shown := askedToBeShown(in.Instruction)
	done := askedToHaveItDone(in.Instruction)

	auto := func() (bool, string) {
		switch {
		case shown != "":
			return true, said(shown)
		case k == route.KindAppGuide:
			return true, "it reads as a question"
		default:
			return false, "it reads as something to do"
		}
	}

	switch in.Mode {
	case ModeMustShow:
		return true, "you chose it"
	case ModeMustDo:
		return false, "you chose it"
	case ModeFlip:
		show, _ := auto()
		return !show, "you chose it"
	case ModeShow:
		if done != "" {
			return false, said(done)
		}
		if shown != "" {
			return true, said(shown)
		}
		return true, "the switch is on Show me how"
	case ModeDo:
		if shown != "" {
			return true, said(shown)
		}
		if done != "" {
			return false, said(done)
		}
		return false, "the switch is on Do it for me"
	default:
		return auto()
	}
}

// UnroutableError is the router declining, carried across the engine boundary
// so the caller can tell "I did not understand" from "I understood and it
// failed". They need entirely different things said to the user.
type UnroutableError struct {
	Instruction string
	Why         string

	// Can lists what VANS is able to do, so the answer to "I can't do that"
	// is followed by what it can. A refusal with no alternative is a dead end.
	Can []string
}

func (e *UnroutableError) Error() string {
	msg := fmt.Sprintf("engine: could not tell what %q is asking for", e.Instruction)
	if e.Why != "" {
		msg += ": " + e.Why
	}
	return msg
}

// Ask routes an instruction and prepares whatever it turns out to be.
//
// Nothing here touches the world. Routing reads no state and extraction reads
// a page; the side effect, if the destination has one, waits for Commit.
func (e *Engine) Ask(ctx context.Context, in Ask) (*Plan, error) {
	if plan, handled, err := e.aboutRoutines(ctx, in.Instruction); handled {
		return plan, err
	}
	if plan, handled, err := e.aboutMemory(ctx, in.Instruction); handled {
		return plan, err
	}
	if saved, ok := e.routineFor(ctx, in.Instruction); ok {
		e.logf("that is the routine %q", strings.TrimSpace(in.Instruction))
		in.Instruction = saved
	}

	rest, rehearsing := withoutTheRehearsal(in.Instruction)
	if rehearsing {
		in.Instruction, in.Mode = rest, ModeMustDo
	}

	looking := e.lookingFor(ctx, in)
	if looking != "" {
		e.logf("the request points at what was in front, so that goes with it")
	}
	if known := rememberedNote(e.recall(ctx, in.Instruction)); known != "" {
		e.logf("something remembered bears on this, so it goes with the task")
		looking = strings.TrimSpace(looking + "\n\n" + known)
	}

	if rehearsing {
		return e.rehearse(withLooking(ctx, looking), in)
	}
	plan, err := e.ask(withLooking(ctx, looking), in)
	if plan != nil {
		if plan.Drive != nil {
			plan.Drive.Looking = looking
		}
		if plan.Guide != nil {
			plan.Guide.Looking = looking
		}
	}
	if err == nil && plan != nil {
		if notAllowed := e.allowedToAct(ctx, plan); notAllowed != nil {
			return nil, notAllowed
		}
	}
	if err != nil || plan == nil || plan.Route == nil || !isTask(plan.Route.Kind) || plan.Mode != "" {
		return plan, err
	}
	_, plan.ModeWhy = e.wantsShowing(in, plan.Route.Kind)
	plan.Mode = ModeDo
	if plan.Guide != nil {
		plan.Mode = ModeShow
	}
	return plan, nil
}

func (e *Engine) ask(ctx context.Context, in Ask) (*Plan, error) {
	profile := ""
	in.Instruction, profile = withoutTheProfile(in.Instruction)
	if profile != "" {
		e.logf("asked for %s", profilesNamed[profile])
	}
	if strings.TrimSpace(in.Instruction) == "" {
		return nil, fmt.Errorf("engine: no instruction")
	}
	if e.Router == nil {
		return nil, fmt.Errorf("engine: no router configured")
	}

	res, err := e.Router.Route(ctx, in.Instruction)
	if err != nil {
		var undecided *route.UndecidedError
		if errors.As(err, &undecided) {
			return nil, &UnroutableError{
				Instruction: in.Instruction,
				Why:         undecided.Why,
				Can:         e.capabilities(),
			}
		}
		return nil, err
	}

	if e.History != nil {
		if err := e.History.RecordInstruction(ctx, in.Instruction, string(res.Route.Kind)); err != nil {
			e.logf("could not remember the instruction: %v", err)
		}
	}

	res.Route = e.aimedAtWhatIsInFront(ctx, res.Route, in.Context)

	if e.selectedTextIsMeant(ctx, in, res.Route.Kind) {
		e.logf("text is selected and the request points at it, so this works on that text")
		res.Route.Kind = route.KindText
	}

	if kind := res.Route.Kind; isTask(kind) {
		show, why := e.wantsShowing(in, kind)
		decided := func(plan *Plan) *Plan {
			plan.Mode, plan.ModeWhy = ModeDo, why
			if show {
				plan.Mode = ModeShow
			}
			return plan
		}
		switch {
		case show && kind != route.KindAppGuide && e.canGuide():
			e.logf("asked to be shown, so this is coached rather than done")
			plan, err := e.showInsteadOfDoing(ctx, in, res)
			if err != nil {
				return nil, err
			}
			r := res.Route
			r.Kind = route.KindAppGuide
			plan.Route = &r
			plan.DecisionCalls = res.DecisionCalls
			return decided(plan), nil

		case !show && kind == route.KindAppGuide && e.canDrive():
			e.logf("asked to have it done, so this is done rather than coached")
			plan, err := e.doInsteadOfShowing(ctx, in, res)
			if err != nil {
				return nil, err
			}
			r := res.Route
			r.Kind = route.KindAppAction
			plan.Route = &r
			plan.DecisionCalls = res.DecisionCalls
			return decided(plan), nil
		}
	}

	switch res.Route.Kind {
	case route.KindText:
		if !e.canWorkOnText() {
			break
		}
		plan, err := e.workOnText(ctx, in)
		if err != nil {
			return nil, err
		}
		r := res.Route
		plan.Route = &r
		plan.DecisionCalls = res.DecisionCalls
		return plan, nil

	case route.KindWebExtract:
		plan, err := e.Prepare(ctx, Task{
			URL:         res.Route.Slot("url"),
			Intent:      res.Route.Slot("intent"),
			Title:       res.Route.Slot("title"),
			Destination: in.Destination,
		})
		if plan != nil {
			r := res.Route
			plan.Route = &r
			plan.DecisionCalls = res.DecisionCalls
		}
		return plan, err

	case route.KindWebSearch:
		query := res.Route.Slot("query")
		target, err := searchURL(res.Route.Slot("engine"), query)
		if err != nil {
			return nil, err
		}

		// The intent is the runtime's, not the user's: they asked a question
		// and said nothing about what a result looks like. From here it is an
		// ordinary extraction, so the results page is signed and cached like
		// any other — and a search engine's results page is about as stable a
		// shape as the web has.
		plan, err := e.Prepare(ctx, Task{
			URL:         target,
			Intent:      searchIntent,
			Title:       searchTitle(query),
			Destination: in.Destination,
		})
		if plan != nil {
			r := res.Route
			plan.Route = &r
			plan.DecisionCalls = res.DecisionCalls

			// The question is the user's, not the runtime's rewriting of it.
			// searchIntent describes what a results page looks like; it is not
			// what anybody asked.
			e.answerFor(ctx, in.Instruction, plan)
		}
		return plan, err

	case route.KindPageHere:
		plan, err := e.readWhatIsInFront(ctx, res.Route.Slot("intent"), in.Context)
		if err != nil {
			return nil, err
		}
		r := res.Route
		plan.Route = &r
		plan.DecisionCalls = res.DecisionCalls
		plan.Destination = in.Destination
		e.answerFor(ctx, in.Instruction, plan)
		return plan, nil

	case route.KindAppGuide:
		// The user's own words, not the slot: routing reduces "where is the
		// search bar" to "the search bar", and a noun is not a task anyone can
		// work out the next step of.
		plan, err := e.planGuideFor(ctx, in.Instruction, res.Shape, res.Route.Literals, in.Context)
		if err != nil {
			return nil, err
		}
		r := res.Route
		plan.Route = &r
		plan.DecisionCalls = res.DecisionCalls
		return plan, nil

	case route.KindPageStyle:
		plan, calls, err := e.stylePage(ctx, res.Route)
		if err != nil {
			return nil, err
		}
		r := res.Route
		plan.Route = &r
		plan.DecisionCalls = res.DecisionCalls
		plan.SynthesisCalls = calls
		return plan, nil

	case route.KindPageAction:
		if e.canDrivePages() {
			site := strings.TrimSpace(res.Route.Slot("site"))
			plan, err := e.driveOnPageIn(ctx, theTask(res.Route), res.Shape, res.Route.Literals, site, profile)
			if err == nil {
				r := res.Route
				plan.Route = &r
				plan.DecisionCalls = res.DecisionCalls
				return plan, nil
			}
			if site != "" || profile != "" {
				return nil, err
			}
			e.logf("could not work through the page a step at a time (%v); trying it in one go", err)
		}
		plan, calls, err := e.actOnPage(ctx, res.Route, res.Shape, "")
		if err != nil {
			return nil, err
		}
		r := res.Route
		plan.Route = &r
		plan.DecisionCalls = res.DecisionCalls
		plan.SynthesisCalls = calls
		return plan, nil

	case route.KindAppAction:
		r := res.Route
		if strings.TrimSpace(r.Slot("app")) == "" {
			if front, err := e.appInQuestion(ctx, in.Context); err == nil && front != "" {
				if r.Slots == nil {
					r.Slots = make(map[string]string)
				}
				r.Slots["app"] = front
			}
		}
		if e.canAct() {
			plan, calls, err := e.actInApplication(ctx, r, res.Shape)
			if err != nil {
				return nil, err
			}
			plan.Route = &r
			plan.DecisionCalls = res.DecisionCalls
			plan.SynthesisCalls = calls
			return plan, nil
		}
		if e.canActOnUI() {
			plan, calls, err := e.actOnUI(ctx, r, res.Shape, r.Slot("app"))
			if err != nil {
				return nil, err
			}
			r := res.Route
			plan.Route = &r
			plan.DecisionCalls = res.DecisionCalls
			plan.SynthesisCalls = calls
			return plan, nil
		}
	}

	// A kind the router can choose but this build cannot run. Said plainly
	// rather than failing somewhere downstream: the router is deliberately
	// ahead of the adapters, so this is an expected state and not a bug to be
	// discovered from a stack trace.
	return nil, &UnroutableError{
		Instruction: in.Instruction,
		Why: fmt.Sprintf(
			"that is a %s task, which VANS can recognise but cannot carry out yet",
			res.Route.Kind),
		Can: e.capabilities(),
	}
}

// capabilities describes what this build can actually run, which is not the
// same as what can be routed — and not the same across builds either. Asked of
// the engine rather than of a constant, so the list a user is shown after
// "I did not catch that" cannot offer something this particular runtime was
// not assembled to do.
func (e *Engine) capabilities() []string {
	var out []string
	for _, spec := range route.Specs {
		if !e.canRun(spec.Kind) {
			continue
		}
		out = append(out, spec.Describe)
	}
	return out
}

func (e *Engine) canRun(k route.Kind) bool {
	switch k {
	case route.KindWebExtract, route.KindWebSearch, route.KindPageHere:
		return true
	case route.KindText:
		return e.canWorkOnText()
	case route.KindPageAction:
		_, ok := e.pageDriver()
		return (ok && e.WebWriter != nil) || e.canDrivePages()
	case route.KindPageStyle:
		return e.canStyle()
	case route.KindAppGuide:
		return e.canGuide()
	case route.KindAppAction:
		return e.canAct() || e.canActOnUI()
	default:
		return false
	}
}

// canAct reports whether every piece acting in an application needs is here.
// All four together or none: a build with some of them would decline halfway
// through a request rather than before starting one.
func (e *Engine) canAct() bool {
	return e.Apps != nil && e.Writer != nil && e.Actions != nil && e.Runner != nil
}

// aimedAtWhatIsInFront sends a page action to the application in front when that
// application is not a browser.
//
// The router decides "this is about a web page" from the words alone, and "type
// hello" reads identically whether the person is looking at a web page or at
// TextEdit. The page tier then acted on whatever tab the browser had open — seen
// live with TextEdit in front, when VANS asked a model how to type into a LinkedIn
// profile nobody was looking at, and reported that it could not.
//
// Only the one request is redirected. The stored decision stays a page action,
// which is the right answer the next time a browser is what is in front.
func (e *Engine) aimedAtWhatIsInFront(ctx context.Context, r route.Route, lookingAt *screen.Context) route.Route {
	if r.Kind != route.KindPageAction || e.UIScreen == nil {
		return r
	}
	if strings.TrimSpace(r.Slot("site")) != "" {
		return r
	}

	front, err := e.appInQuestion(ctx, lookingAt)
	if err != nil || strings.TrimSpace(front) == "" || isBrowser(front) {
		return r
	}

	e.logf("%s is in front, not a browser, so this is done there rather than on a page", front)

	slots := make(map[string]string, len(r.Slots)+1)
	for k, v := range r.Slots {
		slots[k] = v
	}
	slots["app"] = front

	r.Kind = route.KindAppAction
	r.Slots = slots
	return r
}
