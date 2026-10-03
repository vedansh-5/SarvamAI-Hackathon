package uiact

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const mostNudges = 200

var nudgePause = 30 * time.Millisecond

func AmountFor(el Element, text string) (float64, error) {
	text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), "%"))
	if text == "" {
		return 0, fmt.Errorf("it sets the %s %q without saying what to", el.Role, el.Title)
	}
	amount, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", ""), 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number, and the %s %q is set to a number", text, el.Role, el.Title)
	}
	if el.Ranged && (amount < el.Low || amount > el.High) {
		return 0, fmt.Errorf("the %s %q goes from %g to %g, and %g is outside that", el.Role, el.Title, el.Low, el.High, amount)
	}
	return amount, nil
}

func near(a, b, span float64) bool {
	tolerance := 0.5
	if span > 0 {
		tolerance = span / 200
	}
	return math.Abs(a-b) <= tolerance
}

func adjust(ctx context.Context, pid int, a Act) error {
	span := 0.0
	if a.Ranged {
		span = a.High - a.Low
	}

	if err := numberSetter(pid, a.Path, a.Amount); err == nil {
		if now, ok := numberReader(pid, a.Path); !ok || near(now, a.Amount, span) {
			return nil
		}
	}

	last, known := numberReader(pid, a.Path)
	if !known {
		return fmt.Errorf("uiact: that control took no value and shows none, so it cannot be set")
	}
	for i := 0; i < mostNudges; i++ {
		if near(last, a.Amount, span) {
			return nil
		}
		action := "AXIncrement"
		if last > a.Amount {
			action = "AXDecrement"
		}
		done, err := performer(pid, a.Path, action)
		if err != nil {
			return err
		}
		if !done {
			return fmt.Errorf("uiact: that control can be neither set nor stepped")
		}
		if !pause(ctx, nudgePause) {
			return ctx.Err()
		}
		now, ok := numberReader(pid, a.Path)
		if !ok {
			return fmt.Errorf("uiact: that control stopped showing its value")
		}
		if (last < a.Amount && now >= a.Amount) || (last > a.Amount && now <= a.Amount) {
			return nil
		}
		if now == last {
			return fmt.Errorf("uiact: that control stopped at %g and would not go to %g", now, a.Amount)
		}
		last = now
	}
	return fmt.Errorf("uiact: that control is at %g after %d steps and did not reach %g", last, mostNudges, a.Amount)
}
