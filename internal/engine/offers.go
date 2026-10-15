package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/store"
)

const (
	ByTheClock    = "tick"
	MacOpened     = "opened"
	AppForward    = "front"
	yesesToTrust  = 3
	skipsToStop   = 3
	quietAfter    = 3
	snooze        = 15 * time.Minute
	stillDueFor   = 10
	quietKey      = "offers.quiet_on"
	dismissedKey  = "offers.dismissed"
	greetedKey    = "offers.greeted_on"
	OfferRun      = "run"
	OfferAlways   = "always"
	OfferRan      = "ran"
	OfferCancel   = "cancel"
	OfferSkip     = "skip"
	OfferLater    = "later"
	OfferIgnored  = "ignored"
	OfferGreeting = "greeting"
)

type Cause struct {
	Kind string
	App  string
}

type Offer struct {
	Key       string   `json:"key"`
	Line      string   `json:"line"`
	When      string   `json:"when,omitempty"`
	Always    bool     `json:"always,omitempty"`
	Countdown bool     `json:"countdown,omitempty"`
	Morning   bool     `json:"morning,omitempty"`
	Usual     []string `json:"usual,omitempty"`
}

type OfferAnswer struct {
	Instruction string `json:"instruction,omitempty"`
	Open        string `json:"open,omitempty"`
}

func (e *Engine) keepTimed(ctx context.Context, t Timed) error {
	trigger, err := json.Marshal(t.Trigger)
	if err != nil {
		return err
	}
	action, err := json.Marshal(t.Action)
	if err != nil {
		return err
	}
	return e.Clicks.KeepTimed(ctx, store.TimedRow{Key: t.Key, Trigger: string(trigger), Action: string(action), Days: t.Days})
}

func timedFrom(r store.TimedRow) (Timed, bool) {
	t := Timed{Key: r.Key, Days: r.Days}
	if json.Unmarshal([]byte(r.Trigger), &t.Trigger) != nil || json.Unmarshal([]byte(r.Action), &t.Action) != nil {
		return Timed{}, false
	}
	return t, true
}

func (e *Engine) quietToday(ctx context.Context, now time.Time) bool {
	v, _ := e.Clicks.Setting(ctx, quietKey)
	return v == dayOf(now)
}

func doneToday(today []store.Click, action store.Click) bool {
	for _, c := range today {
		if actionKey(c) == actionKey(action) {
			return true
		}
	}
	return false
}

func firstTimeToday(today []store.Click, app string) bool {
	n := 0
	for _, c := range today {
		if c.Role == store.RoleFront && c.App == app {
			n++
		}
	}
	return n <= 1
}

func due(t Timed, cause Cause, now time.Time, today []store.Click) bool {
	switch t.Trigger.Kind {
	case AtATime:
		if cause.Kind != ByTheClock || (t.Trigger.Weekdays && !isWeekday(dayOf(now))) {
			return false
		}
		m := minuteOf(now)
		return m >= t.Trigger.Minute && m <= t.Trigger.Minute+stillDueFor
	case WhenOpened:
		return cause.Kind == MacOpened
	case AfterAnApp:
		return cause.Kind == AppForward && cause.App == t.Trigger.App && firstTimeToday(today, cause.App)
	}
	return false
}

func (e *Engine) OffersFor(ctx context.Context, cause Cause, now time.Time) ([]Offer, error) {
	if e.Clicks == nil {
		return nil, nil
	}
	if on, err := e.Clicks.Watching(ctx); err != nil || !on || e.quietToday(ctx, now) {
		return nil, err
	}
	rows, err := e.Clicks.TimedRows(ctx)
	if err != nil {
		return nil, err
	}
	today, err := e.Clicks.Clicks(ctx, time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()))
	if err != nil {
		return nil, err
	}

	var offers []Offer
	if cause.Kind == MacOpened {
		if greeted, _ := e.Clicks.Setting(ctx, greetedKey); greeted != dayOf(now) && len(rows) > 0 {
			if err := e.Clicks.SetSetting(ctx, greetedKey, dayOf(now)); err != nil {
				return nil, err
			}
			morning := Offer{Key: OfferGreeting, Line: "Good morning", Morning: true}
			for _, r := range rows {
				if t, ok := timedFrom(r); ok && t.Trigger.Kind != WhenOpened && (!t.Trigger.Weekdays || isWeekday(dayOf(now))) {
					morning.Usual = append(morning.Usual, t.Doing()+" "+t.When())
				}
			}
			if len(morning.Usual) > 0 {
				offers = append(offers, morning)
			}
		}
	}

	for _, r := range rows {
		t, ok := timedFrom(r)
		if !ok {
			continue
		}
		snoozed := !r.LaterUntil.IsZero()
		switch {
		case snoozed && now.Before(r.LaterUntil):
			continue
		case snoozed:
			if cause.Kind != ByTheClock {
				continue
			}
		case r.OfferedOn == dayOf(now) || !due(t, cause, now, today):
			continue
		}
		if doneToday(today, t.Action) {
			continue
		}
		r.OfferedOn, r.LaterUntil = dayOf(now), time.Time{}
		if err := e.Clicks.PutTimed(ctx, r); err != nil {
			return nil, err
		}
		offers = append(offers, Offer{
			Key:       r.Key,
			Line:      t.Line(),
			When:      t.When(),
			Always:    !r.Always && r.Yes >= yesesToTrust,
			Countdown: r.Always,
		})
	}
	return offers, nil
}

func (e *Engine) dismissed(ctx context.Context, now time.Time) error {
	v, _ := e.Clicks.Setting(ctx, dismissedKey)
	n, _ := strconv.Atoi(v)
	n++
	if n >= quietAfter {
		n = 0
		if err := e.Clicks.SetSetting(ctx, quietKey, dayOf(now)); err != nil {
			return err
		}
	}
	return e.Clicks.SetSetting(ctx, dismissedKey, strconv.Itoa(n))
}

func (e *Engine) AnswerOffer(ctx context.Context, key, answer string, now time.Time) (OfferAnswer, error) {
	if e.Clicks == nil {
		return OfferAnswer{}, fmt.Errorf("engine: this build does not watch")
	}
	if key == OfferGreeting {
		if answer == OfferIgnored || answer == OfferSkip {
			return OfferAnswer{}, e.dismissed(ctx, now)
		}
		return OfferAnswer{}, e.Clicks.SetSetting(ctx, dismissedKey, "0")
	}
	r, found, err := e.Clicks.TimedRow(ctx, key)
	if err != nil {
		return OfferAnswer{}, err
	}
	if !found {
		return OfferAnswer{}, fmt.Errorf("engine: that suggestion has been cleared")
	}
	t, ok := timedFrom(r)
	if !ok {
		return OfferAnswer{}, fmt.Errorf("engine: that suggestion could not be read")
	}

	runs := false
	switch answer {
	case OfferRun, OfferRan:
		r.Yes, r.Skips, runs = r.Yes+1, 0, true
	case OfferAlways:
		r.Yes, r.Skips, r.Always, runs = r.Yes+1, 0, true, true
	case OfferCancel:
		r.Always = false
	case OfferSkip:
		r.Skips++
		r.Stopped = r.Skips >= skipsToStop
		if err := e.dismissed(ctx, now); err != nil {
			return OfferAnswer{}, err
		}
	case OfferIgnored:
		if err := e.dismissed(ctx, now); err != nil {
			return OfferAnswer{}, err
		}
	case OfferLater:
		r.LaterUntil = now.Add(snooze)
	default:
		return OfferAnswer{}, fmt.Errorf("engine: %q is not an answer to a suggestion", answer)
	}
	if runs {
		if err := e.Clicks.SetSetting(ctx, dismissedKey, "0"); err != nil {
			return OfferAnswer{}, err
		}
	}
	if err := e.Clicks.PutTimed(ctx, r); err != nil {
		return OfferAnswer{}, err
	}
	if !runs {
		return OfferAnswer{}, nil
	}
	if t.Action.Role == store.RoleFront {
		return OfferAnswer{Open: t.Action.App}, nil
	}
	return OfferAnswer{Instruction: t.Instruction()}, nil
}

func (o Offer) String() string {
	if o.Morning {
		return o.Line + ": " + strings.Join(o.Usual, ", ")
	}
	return o.Line
}
