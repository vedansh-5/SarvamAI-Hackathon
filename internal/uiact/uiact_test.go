package uiact

import (
	"errors"
	"strings"
	"testing"
)

func window() Tree {
	return Tree{
		App:    "Telegram",
		Window: "Telegram",
		Elements: []Element{
			{Role: "AXTextArea", Title: "Message", Enabled: true, Path: []int{0, 1}},
			{Role: "AXButton", Title: "Send", Enabled: true, Actions: []string{"AXPress"}, Path: []int{0, 2}},
			{Role: "AXButton", Title: "Open", Enabled: true, Actions: []string{"AXPress"}, Path: []int{1, 0}},
			{Role: "AXButton", Title: "Open", Enabled: true, Actions: []string{"AXPress"}, Path: []int{1, 1}},
			{Role: "AXStaticText", Title: "Delivered", Enabled: true, Path: []int{2}},
			{Role: "AXButton", Title: "Archive", Enabled: false, Actions: []string{"AXPress"}, Path: []int{3}},
		},
	}
}

func send() Program {
	return Program{
		App: "Telegram",
		Steps: []Step{
			{Do: "type", Target: Selector{Role: "AXTextArea", Title: "Message"}, Text: "on my way"},
			{Do: "click", Target: Selector{Role: "AXButton", Title: "Send"}},
		},
	}
}

func reasons(t *testing.T, err error) []string {
	t.Helper()
	var invalid *InvalidError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidError", err)
	}
	return invalid.Reasons
}

func mentions(rs []string, want string) bool {
	for _, r := range rs {
		if strings.Contains(r, want) {
			return true
		}
	}
	return false
}

func TestAValidProgramRenders(t *testing.T) {
	script, err := Render(send(), window())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(script, `typeInto([0,1], "AXTextArea", "Message", "on my way")`) {
		t.Errorf("typing step missing or wrong:\n%s", script)
	}
	if !strings.Contains(script, `click([0,2], "AXButton", "Send")`) {
		t.Errorf("click step missing or wrong:\n%s", script)
	}
}

func TestAmbiguityIsRefused(t *testing.T) {
	p := Program{App: "Telegram", Steps: []Step{
		{Do: "click", Target: Selector{Role: "AXButton", Title: "Open"}},
	}}
	rs := reasons(t, Validate(p, window()))
	if !mentions(rs, "does not say which one") {
		t.Errorf("reasons = %v", rs)
	}
}

func TestIndexResolvesAmbiguity(t *testing.T) {
	p := Program{App: "Telegram", Steps: []Step{
		{Do: "click", Target: Selector{Role: "AXButton", Title: "Open", Index: 2}},
	}}
	script, err := Render(p, window())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(script, `click([1,1]`) {
		t.Errorf("index picked the wrong element:\n%s", script)
	}
}

func TestTypingIntoSomethingThatCannotHoldTextIsRefused(t *testing.T) {
	p := Program{App: "Telegram", Steps: []Step{
		{Do: "type", Target: Selector{Role: "AXStaticText", Title: "Delivered"}, Text: "hello"},
	}}
	rs := reasons(t, Validate(p, window()))
	if !mentions(rs, "cannot hold typed text") {
		t.Errorf("reasons = %v", rs)
	}
}

func TestADisabledElementIsRefused(t *testing.T) {
	p := Program{App: "Telegram", Steps: []Step{
		{Do: "click", Target: Selector{Role: "AXButton", Title: "Archive"}},
	}}
	rs := reasons(t, Validate(p, window()))
	if !mentions(rs, "is disabled") {
		t.Errorf("reasons = %v", rs)
	}
}

func TestAnElementThatIsNotThereIsRefused(t *testing.T) {
	p := Program{App: "Telegram", Steps: []Step{
		{Do: "click", Target: Selector{Role: "AXButton", Title: "Publish"}},
	}}
	rs := reasons(t, Validate(p, window()))
	if !mentions(rs, "nothing on screen is a AXButton") {
		t.Errorf("reasons = %v", rs)
	}
}

func TestUnknownVerbAndKeyAreRefused(t *testing.T) {
	p := Program{App: "Telegram", Steps: []Step{
		{Do: "drag", Target: Selector{Role: "AXButton", Title: "Send"}},
		{Do: "press", Target: Selector{Role: "AXTextArea", Title: "Message"}, Key: "F13"},
	}}
	rs := reasons(t, Validate(p, window()))
	if !mentions(rs, "not something VANS does") || !mentions(rs, "not a key VANS presses") {
		t.Errorf("reasons = %v", rs)
	}
}

func TestTooManyStepsAreRefused(t *testing.T) {
	p := Program{App: "Telegram"}
	for i := 0; i < MaxSteps+1; i++ {
		p.Steps = append(p.Steps, Step{Do: "click", Target: Selector{Role: "AXButton", Title: "Send"}})
	}
	rs := reasons(t, Validate(p, window()))
	if !mentions(rs, "is the most a single request may take") {
		t.Errorf("reasons = %v", rs)
	}
}

func TestTextCannotCloseItsOwnLiteral(t *testing.T) {
	p := Program{App: "Telegram", Steps: []Step{
		{Do: "type", Target: Selector{Role: "AXTextArea", Title: "Message"},
			Text: `"); proc.quit(); ("`},
	}}
	script, err := Render(p, window())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(script, "proc.quit();") && !strings.Contains(script, `\"`) {
		t.Errorf("text escaped its literal:\n%s", script)
	}
}

func TestBakedCatchesAValueWrittenIn(t *testing.T) {
	p := send()
	got := Baked(p, map[string]string{"s0": "on my way"})
	if len(got) != 1 || got[0] != "on my way" {
		t.Errorf("Baked = %v", got)
	}
}

func TestFillReplacesTextAndTitle(t *testing.T) {
	p := Program{App: "Telegram", Steps: []Step{
		{Do: "type", Target: Selector{Role: "AXTextArea", Title: "{s1}"}, Text: "{s0}"},
	}}
	got := p.Fill(func(v string) string {
		switch v {
		case "{s0}":
			return "hello"
		case "{s1}":
			return "Message"
		}
		return v
	})
	if got.Steps[0].Text != "hello" || got.Steps[0].Target.Title != "Message" {
		t.Errorf("Fill = %+v", got.Steps[0])
	}
}

func TestTreeTextNumbersDuplicates(t *testing.T) {
	got := window().Text()
	if !strings.Contains(got, "(index 1 of 2)") || !strings.Contains(got, "(index 2 of 2)") {
		t.Errorf("duplicates not distinguishable:\n%s", got)
	}
	if !strings.Contains(got, "[disabled]") {
		t.Errorf("disabled element not marked:\n%s", got)
	}
}

// The claim that makes this tier cacheable: the program names a role and a
// title, so it still finds its control when the window has been rearranged
// and every path has changed.
func TestAProgramFollowsItsControlsWhenTheyMove(t *testing.T) {
	p := send()
	if err := Validate(p, window()); err != nil {
		t.Fatalf("the program did not fit the window it was written for: %v", err)
	}

	moved := window()
	for i := range moved.Elements {
		moved.Elements[i].Path = append([]int{4}, moved.Elements[i].Path...)
	}

	script, err := Render(p, moved)
	if err != nil {
		t.Fatalf("Render against the rearranged window: %v", err)
	}
	if !strings.Contains(script, `typeInto([4,0,1]`) {
		t.Errorf("typing did not follow its control:\n%s", script)
	}
	if !strings.Contains(script, `click([4,0,2]`) {
		t.Errorf("the click did not follow its control:\n%s", script)
	}
}

// A control that is gone is refused rather than resolved to whatever now sits
// at the remembered position.
func TestAProgramIsRefusedWhenItsControlIsGone(t *testing.T) {
	gone := window()
	gone.Elements = gone.Elements[1:]

	if err := Validate(send(), gone); err == nil {
		t.Fatal("a program whose control disappeared was accepted")
	}
}
