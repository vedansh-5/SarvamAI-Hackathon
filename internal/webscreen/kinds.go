package webscreen

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/vedansh/vans/internal/uiact"
)

var (
	SuggestWait  = 1500 * time.Millisecond
	OpenWait     = 500 * time.Millisecond
	CalendarWait = 800 * time.Millisecond
)

const (
	mostMonthsMoved = 14
	mostCounts      = 30
)

type answer struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
	More  bool   `json:"more"`
	Opens bool   `json:"opens"`
}

func (s *Screen) ask(ctx context.Context, a act) (answer, error) {
	arg, err := json.Marshal(a)
	if err != nil {
		return answer{}, err
	}
	var out answer
	if err := s.Page.AskFront(ctx, actOnPage+"("+string(arg)+")", &out); err != nil {
		return answer{}, err
	}
	return out, nil
}

func (s *Screen) idFor(ctx context.Context, sel uiact.Selector) (uiact.Tree, uiact.Element, int, error) {
	now, err := s.Capture(ctx, "")
	if err != nil {
		return uiact.Tree{}, uiact.Element{}, 0, err
	}
	el, why := now.Resolve(sel)
	if why != "" {
		return now, el, 0, &uiact.InvalidError{App: now.App, Reasons: []string{why}}
	}
	id, ok := idOf(el)
	if !ok {
		return now, el, 0, &uiact.InvalidError{App: now.App, Reasons: []string{fmt.Sprintf("the %s is not something on the page to act on", el.Role)}}
	}
	return now, el, id, nil
}

func (s *Screen) clickOn(ctx context.Context, t uiact.Tree, id int) error {
	if err := s.send(ctx, act{Verb: "click", ID: id}); err != nil {
		return err
	}
	if s.waitForAnAnswer(ctx, t) {
		return nil
	}
	return s.clickForReal(ctx, t, id)
}

func (s *Screen) openWith(ctx context.Context, t uiact.Tree, id int, opened func(uiact.Tree) bool) (uiact.Tree, bool, error) {
	if err := s.send(ctx, act{Verb: "click", ID: id}); err != nil {
		return t, false, err
	}
	for round := 0; round < 2; round++ {
		deadline := time.Now().Add(OpenWait)
		if round == 1 {
			deadline = time.Now().Add(CalendarWait)
		}
		for {
			now, err := s.Capture(ctx, "")
			if err != nil {
				return t, false, err
			}
			if opened(now) {
				return now, true, nil
			}
			if time.Now().After(deadline) {
				break
			}
			select {
			case <-ctx.Done():
				return t, false, ctx.Err()
			case <-time.After(AnswerPoll):
			}
		}
		if round == 0 {
			if s.RealClick == nil {
				break
			}
			if err := s.clickForReal(ctx, t, id); err != nil {
				return t, false, err
			}
		}
	}
	return t, false, nil
}

func (s *Screen) clickForReal(ctx context.Context, t uiact.Tree, id int) error {
	if s.RealClick == nil {
		return nil
	}
	was, ok := byID(t, id)
	if !ok {
		return nil
	}
	now, err := s.Capture(ctx, "")
	if err != nil {
		return err
	}
	at, ok := byID(now, id)
	if !ok || at.Role != was.Role || at.Title != was.Title {
		found := now.Find(uiact.Selector{Role: was.Role, Title: was.Title})
		if len(found) == 0 {
			return nil
		}
		at = found[0]
	}
	if len(at.Position) < 2 || len(at.Size) < 2 {
		return nil
	}
	if s.Raise != nil {
		if err := s.Raise(ctx, s.browser()); err != nil {
			return err
		}
	}
	s.logf("the page did not answer a scripted click on %q, so it is done with the real pointer", at.Title)
	s.RealClick(at.Position[0]+at.Size[0]/2, at.Position[1]+at.Size[1]/2)
	s.waitForAnAnswer(ctx, now)
	return nil
}

func byID(t uiact.Tree, id int) (uiact.Element, bool) {
	for _, el := range t.Elements {
		if got, ok := idOf(el); ok && got == id {
			return el, true
		}
	}
	return uiact.Element{}, false
}

func (s *Screen) typeWhereItOpens(ctx context.Context, t uiact.Tree, id int, el uiact.Element, text string) error {
	box := func(now uiact.Tree) (uiact.Element, int, bool) {
		for _, field := range now.Elements {
			into, ok := idOf(field)
			if ok && field.Focused && field.Typeable() && field.Role != "AXDateField" && field.Enabled && !reflect.DeepEqual(field.Position, el.Position) {
				return field, into, true
			}
		}
		return uiact.Element{}, 0, false
	}
	opened, ok, err := s.openWith(ctx, t, id, func(now uiact.Tree) bool { _, _, ok := box(now); return ok })
	if err != nil {
		return err
	}
	if ok {
		field, into, _ := box(opened)
		s.logf("%q opens a box of its own, so the text is typed there", el.Title)
		if err := s.send(ctx, act{Verb: "type", ID: into, Text: text}); err != nil {
			return err
		}
		s.waitForAnAnswer(ctx, opened)
		if field.Role == "AXComboBox" {
			s.pickSuggestion(ctx, opened, text)
		}
		return nil
	}
	return &uiact.InvalidError{App: t.App, Reasons: []string{fmt.Sprintf("the %s %q cannot be typed into, and clicking it opened nothing to type into", el.Role, el.Title)}}
}

func folded(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

var (
	leadingCode = regexp.MustCompile(`^([A-Z0-9]{2,4})\s+(.+)$`)
	firstPart   = regexp.MustCompile(`^[^,(\-–—|]+`)
)

func fits(typed, option string) int {
	code, body := "", strings.TrimSpace(option)
	if m := leadingCode.FindStringSubmatch(body); m != nil {
		code, body = m[1], m[2]
	}
	t, o, head := folded(typed), folded(body), folded(firstPart.FindString(body))
	switch {
	case t == "" || o == "":
		return 0
	case o == t || head == t:
		return 5
	case strings.HasPrefix(o, t+" "):
		return 4
	case strings.HasPrefix(o, t):
		return 3
	case code != "" && strings.EqualFold(code, strings.TrimSpace(typed)):
		return 2
	case strings.Contains(" "+o, " "+t):
		return 1
	}
	return 0
}

func (s *Screen) pickSuggestion(ctx context.Context, before uiact.Tree, text string) {
	there := map[string]int{}
	for _, el := range before.Elements {
		if el.Role == "AXMenuItem" {
			there[el.Title]++
		}
	}
	deadline := time.Now().Add(SuggestWait)
	for {
		now, err := s.Capture(ctx, "")
		if err != nil {
			return
		}
		best, at := 0, -1
		seen := map[string]int{}
		for i, el := range now.Elements {
			if _, ok := idOf(el); !ok || el.Role != "AXMenuItem" || !el.Enabled {
				continue
			}
			seen[el.Title]++
			if seen[el.Title] <= there[el.Title] {
				continue
			}
			if score := fits(text, el.Title); score > best {
				best, at = score, i
			}
		}
		if at >= 0 {
			s.logf("picked %q from the suggestions for %q", now.Elements[at].Title, text)
			id, _ := idOf(now.Elements[at])
			_ = s.clickOn(ctx, now, id)
			return
		}
		if time.Now().After(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(AnswerPoll):
		}
	}
}

var (
	isoDay     = regexp.MustCompile(`\b(\d{4})-(\d{1,2})-(\d{1,2})\b`)
	slashedDay = regexp.MustCompile(`\b(\d{1,2})[/.](\d{1,2})[/.](\d{2,4})\b`)
	dateBits   = regexp.MustCompile(`[A-Za-z]+|\d+`)
	yearGiven  = regexp.MustCompile(`\b\d{4}\b|\d[/.]\d{1,2}[/.]\d{2}`)
	nextMonth  = regexp.MustCompile(`(?i)^(next( month)?|›|»|→|>|⟩|❯)$|next month`)
	lastMonth  = regexp.MustCompile(`(?i)^(prev(ious)?( month)?|‹|«|←|<|⟨|❮)$|previous month`)
)

var months = map[string]time.Month{
	"jan": time.January, "feb": time.February, "mar": time.March, "apr": time.April,
	"may": time.May, "jun": time.June, "jul": time.July, "aug": time.August,
	"sep": time.September, "oct": time.October, "nov": time.November, "dec": time.December,
}

func monthIn(word string) (time.Month, bool) {
	w := strings.ToLower(word)
	if len(w) < 3 {
		return 0, false
	}
	m, ok := months[w[:3]]
	if !ok {
		return 0, false
	}
	full := strings.ToLower(m.String())
	if len(w) > 3 && !strings.HasPrefix(full, w) && w != "sept" {
		return 0, false
	}
	return m, true
}

func dateIn(text string, year int) (time.Time, bool) {
	day := func(y, m, d int) (time.Time, bool) {
		if y < 100 {
			y += 2000
		}
		at := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
		if at.Month() != time.Month(m) || at.Day() != d {
			return time.Time{}, false
		}
		return at, true
	}
	if m := isoDay.FindStringSubmatch(text); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		return day(y, mo, d)
	}
	if m := slashedDay.FindStringSubmatch(text); m != nil {
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		y, _ := strconv.Atoi(m[3])
		return day(y, mo, d)
	}
	var mo time.Month
	d, y := 0, 0
	bits := dateBits.FindAllString(text, -1)
	for i, bit := range bits {
		if bit[0] >= '0' && bit[0] <= '9' {
			n, _ := strconv.Atoi(bit)
			switch {
			case len(bit) == 4 && n >= 1900 && n < 2200:
				y = n
			case len(bit) == 2 && d != 0 && mo != 0 && y == 0 && i > 0 && !strings.ContainsAny(bits[i-1], "0123456789"):
				y = n
			case len(bit) <= 2 && n >= 1 && n <= 31 && d == 0:
				d = n
			}
			continue
		}
		if m, ok := monthIn(bit); ok && mo == 0 {
			mo = m
		}
	}
	if mo == 0 || d == 0 {
		return time.Time{}, false
	}
	if y == 0 {
		y = year
	}
	return day(y, int(mo), d)
}

func wantedDate(text string, today time.Time) (time.Time, bool) {
	at, ok := dateIn(text, today.Year())
	if !ok || yearGiven.MatchString(text) {
		return at, ok
	}
	if at.Before(time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)) {
		at = at.AddDate(1, 0, 0)
	}
	return at, true
}

type dayCell struct {
	at time.Time
	id int
	el uiact.Element
}

func calendarIn(t uiact.Tree, year int) []dayCell {
	var out []dayCell
	for _, el := range t.Elements {
		id, ok := idOf(el)
		if !ok || el.Role == "AXDateField" || el.Typeable() {
			continue
		}
		if at, ok := dateIn(el.Title, year); ok {
			out = append(out, dayCell{at: at, id: id, el: el})
		}
	}
	return out
}

func moverIn(t uiact.Tree, onward bool) (int, bool) {
	look := lastMonth
	if onward {
		look = nextMonth
	}
	for _, el := range t.Elements {
		if id, ok := idOf(el); ok && el.Enabled && (el.Role == "AXButton" || el.Role == "AXLink") && look.MatchString(strings.TrimSpace(el.Title)) {
			return id, true
		}
	}
	return 0, false
}

func (s *Screen) pickDate(ctx context.Context, t uiact.Tree, step uiact.Step) error {
	want, ok := wantedDate(step.Text, time.Now())
	if !ok {
		return &uiact.InvalidError{App: t.App, Reasons: []string{fmt.Sprintf("%q is not a date; give it as 24 Oct 2026", step.Text)}}
	}
	t, el, id, err := s.idFor(ctx, step.Target)
	if err != nil {
		return err
	}
	ans, err := s.ask(ctx, act{Verb: "date", ID: id, Text: want.Format("2006-01-02")})
	if err != nil {
		return err
	}
	if !ans.OK {
		return &uiact.InvalidError{App: t.App, Reasons: []string{ans.Error}}
	}
	if !ans.Opens {
		s.waitForAnAnswer(ctx, t)
		return nil
	}
	if _, _, err := s.openWith(ctx, t, id, func(now uiact.Tree) bool { return len(calendarIn(now, want.Year())) > 0 }); err != nil {
		return err
	}

	shown := want.Format("2 Jan 2006")
	deadline := time.Now().Add(CalendarWait)
	for moved := 0; moved <= mostMonthsMoved; {
		now, err := s.Capture(ctx, "")
		if err != nil {
			return err
		}
		cells := calendarIn(now, want.Year())
		if len(cells) == 0 {
			if time.Now().After(deadline) {
				return &uiact.InvalidError{App: t.App, Reasons: []string{fmt.Sprintf("clicking %q brought up no calendar to pick %s from", el.Title, shown)}}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(AnswerPoll):
			}
			continue
		}
		first, last := cells[0].at, cells[0].at
		for _, c := range cells {
			if c.at.Equal(want) {
				if !c.el.Enabled {
					return &uiact.InvalidError{App: t.App, Reasons: []string{fmt.Sprintf("%s cannot be picked in this calendar; it is greyed out", shown)}}
				}
				s.logf("picked %s in the calendar", shown)
				return s.clickOn(ctx, now, c.id)
			}
			if c.at.Before(first) {
				first = c.at
			}
			if c.at.After(last) {
				last = c.at
			}
		}
		onward := want.After(last)
		if !onward && !want.Before(first) {
			return &uiact.InvalidError{App: t.App, Reasons: []string{fmt.Sprintf("the calendar shows no day for %s", shown)}}
		}
		mover, ok := moverIn(now, onward)
		if !ok {
			return &uiact.InvalidError{App: t.App, Reasons: []string{fmt.Sprintf("the calendar does not go to %s", shown)}}
		}
		if err := s.clickOn(ctx, now, mover); err != nil {
			return err
		}
		moved++
		deadline = time.Now().Add(CalendarWait)
	}
	return &uiact.InvalidError{App: t.App, Reasons: []string{fmt.Sprintf("%s is more than %d months away in this calendar", shown, mostMonthsMoved)}}
}

func (s *Screen) setCount(ctx context.Context, step uiact.Step) error {
	for pressed := 0; pressed < mostCounts; pressed++ {
		now, _, id, err := s.idFor(ctx, step.Target)
		if err != nil {
			return err
		}
		ans, err := s.ask(ctx, act{Verb: "set", ID: id, Text: step.Text})
		if err != nil {
			return err
		}
		if !ans.OK {
			return &uiact.InvalidError{App: now.App, Reasons: []string{ans.Error}}
		}
		if !ans.More {
			return nil
		}
		s.waitForAnAnswer(ctx, now)
	}
	return &uiact.InvalidError{App: step.App, Reasons: []string{fmt.Sprintf("%q did not reach %s after %d presses", step.Target.Title, step.Text, mostCounts)}}
}
