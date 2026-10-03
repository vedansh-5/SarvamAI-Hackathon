package uiact

import (
	"strings"
	"testing"
)

func chatWindow() Tree {
	at := func(y int) ([]int, []int) { return []int{100, y}, []int{400, 20} }
	el := func(i int, role, title, value string, y int) Element {
		p, s := at(y)
		return Element{Role: role, Title: title, Value: value, Enabled: true, Position: p, Size: s, Path: []int{0, i}}
	}
	return Tree{App: "Chat", Window: "Joe", Elements: []Element{
		{Role: "AXMenuBarItem", Title: "File", Path: []int{MenuBarRoot, 0}, Position: []int{0, 0}, Size: []int{40, 20}},
		{Role: "AXWindow", Position: []int{0, 0}, Size: []int{800, 600}, Path: []int{0}},
		el(0, "AXStaticText", "", "Joe", 40),
		el(1, "AXStaticText", "", "Are we still on for Friday?", 80),
		el(2, "AXStaticText", "", "Are we still on for Friday?", 100),
		el(3, "AXStaticText", "", "Yes, 7 pm at the usual place", 140),
		el(4, "AXSecureTextField", "Passcode", "hunter2", 200),
		el(5, "AXCheckBox", "Mute", "1", 240),
		el(6, "AXTextArea", "Message", "see you then", 300),
		el(7, "AXStaticText", "", "an older message far above", -400),
	}}
}

func TestAWindowIsReadTopToBottomAsItsText(t *testing.T) {
	lines := chatWindow().TextLines(MostTextBytes)
	got := strings.Join(lines, " | ")
	want := "Joe | Are we still on for Friday? | Yes, 7 pm at the usual place | Mute | Message: see you then"
	if got != want {
		t.Errorf("read\n  %s\nwant\n  %s", got, want)
	}
}

func TestOnlyTheEndIsKeptWhenThereIsTooMuchText(t *testing.T) {
	lines := chatWindow().TextLines(60)
	if len(lines) == 0 || lines[len(lines)-1] != "Message: see you then" {
		t.Fatalf("lines = %q, want the newest text kept", lines)
	}
	for _, line := range lines {
		if line == "Joe" {
			t.Errorf("lines = %q, want the oldest text dropped first", lines)
		}
	}
}
