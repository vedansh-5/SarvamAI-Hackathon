package synth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	KindContent      = "content"
	MostContentBytes = 32 << 10
)

type Content struct {
	Welcome   *Welcome      `json:"welcome,omitempty"`
	Abilities []AbilityText `json:"abilities,omitempty"`
}

type Welcome struct {
	Title string `json:"title,omitempty"`
	Lede  string `json:"lede,omitempty"`
	Keys  []Key  `json:"keys,omitempty"`
}

type Key struct {
	Before string `json:"before,omitempty"`
	Key    string `json:"key"`
	After  string `json:"after,omitempty"`
}

type AbilityText struct {
	ID       string   `json:"id"`
	Title    string   `json:"title,omitempty"`
	Says     string   `json:"says,omitempty"`
	Examples []string `json:"examples,omitempty"`
	Hidden   bool     `json:"hidden,omitempty"`
}

const (
	AbilityGuide    = "guide"
	AbilityDo       = "do"
	AbilityRead     = "read"
	AbilityPage     = "page"
	AbilityMarks    = "marks"
	AbilitySearch   = "search"
	AbilityOnText   = "text"
	AbilityFiles    = "files"
	AbilitySetting  = "setting"
	AbilityMemory   = "memory"
	AbilityRoutines = "routines"
	AbilityLater    = "later"
	AbilityLearning = "learning"
	AbilityVoice    = "voice"
)

func BuiltInWelcome() Welcome {
	return Welcome{
		Title: "VANS shows you how",
		Lede:  "Ask how to do something and VANS points at each step on your own screen while you do it. When you would rather not, it can do it for you, where you can watch.",
		Keys: []Key{
			{Key: "⌥Space", After: "opens VANS, from any app. So does the dot at the top of the screen."},
			{Before: "Hold", Key: "⌥Space", After: "and talk, then let go, to ask out loud."},
			{Key: "Esc", After: "stops what VANS is doing."},
			{Before: "Hold", Key: "Return", After: "to say yes when VANS asks before changing something."},
		},
	}
}

func BuiltInAbilities() []AbilityText {
	return []AbilityText{
		{ID: AbilityGuide, Title: "Show you how",
			Says:     "Ask how to do something in any app. VANS points at each step on your screen while you do it yourself.",
			Examples: []string{"how do I add a new layer", "show me how to export this as a PDF"}},
		{ID: AbilityDo, Title: "Do it for you",
			Says:     "Switch to Do it for me and VANS does the steps on screen where you can watch. It asks before anything that changes your work.",
			Examples: []string{"make a new note called Groceries"}},
		{ID: AbilityVoice, Title: "Ask out loud",
			Says: "Hold ⌥Space, say what you want and let go. Or press ⌥⇧Space and talk; VANS stops listening when you stop. During a guide, say \"next\" or \"stop\". Your voice stays on this Mac; only the words go on, as if typed."},
		{ID: AbilityRead, Title: "Read what is on screen",
			Says:     "Ask about the window in front: an error, a status, a long message.",
			Examples: []string{"what does this error say", "summarise this window"}},
		{ID: AbilityPage, Title: "Answer about the page you are on",
			Says:     "Questions about the web page in front of you, answered from the page itself.",
			Examples: []string{"what is this page about"}},
		{ID: AbilityMarks, Title: "Mark things on a page",
			Says:     "Highlights only the words that matter, in your colour. Ask to clear the highlighting to take the marks off.",
			Examples: []string{"highlight the ones that start with 23", `highlight "Pending"`}},
		{ID: AbilitySearch, Title: "Look things up",
			Says:     "Searches the web or reads a site, and answers with where it found it.",
			Examples: []string{"search for flights to Goa", "the top stories on news.ycombinator.com"}},
		{ID: AbilityOnText, Title: "Work on text",
			Says:     "Translate, shorten, correct or reply to text you have selected.",
			Examples: []string{"translate the selected text to Hindi", "make this shorter"}},
		{ID: AbilityFiles, Title: "Tidy files",
			Says:     "Moves, renames and sorts files in the folders you name, and can put them back.",
			Examples: []string{"move the PDFs on my Desktop into a folder called Papers"}},
		{ID: AbilitySetting, Title: "Change a setting",
			Says:     "Volume, mute, dark or light, and a few more, without opening System Settings.",
			Examples: []string{"turn on dark mode", "set the volume to 30%"}},
		{ID: AbilityMemory, Title: "Remember things",
			Says:     "Facts VANS uses when a task needs them. Ask what it remembers, or tell it to forget.",
			Examples: []string{"remember that my home city is Delhi", "what do you remember"}},
		{ID: AbilityRoutines, Title: "Save a routine",
			Says:     "Keep a request under a name and run it again by saying the name.",
			Examples: []string{"save that as routine morning report", "list my routines"}},
		{ID: AbilityLater, Title: "Do it later",
			Says:     "Runs a request at a time you say, once or every day.",
			Examples: []string{"every weekday at 9am run morning report", "in 20 minutes remind me to stretch"}},
		{ID: AbilityLearning, Title: "Learn what you do often",
			Says: "Notices what you do at the same moment each day and offers it in a line below the dot. Switch it off in Setup, under Learning."},
	}
}

func BuiltInContent() Content {
	w := BuiltInWelcome()
	return Content{Welcome: &w, Abilities: BuiltInAbilities()}
}

func ParseContent(raw string) (Content, error) {
	var c Content
	if strings.TrimSpace(raw) == "" {
		return c, nil
	}
	if len(raw) > MostContentBytes {
		return c, fmt.Errorf("the text is %d bytes; at most %d fit", len(raw), MostContentBytes)
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Content{}, fmt.Errorf("it is not readable: %w", err)
	}
	if dec.More() {
		return Content{}, fmt.Errorf("it has something after the closing brace")
	}
	known := map[string]bool{}
	for _, a := range BuiltInAbilities() {
		known[a.ID] = true
	}
	seen := map[string]bool{}
	for _, a := range c.Abilities {
		if !known[a.ID] {
			return Content{}, fmt.Errorf("%q is not an ability VANS has; the ids are the ones in the starting text", a.ID)
		}
		if seen[a.ID] {
			return Content{}, fmt.Errorf("%q is listed twice", a.ID)
		}
		seen[a.ID] = true
	}
	if c.Welcome != nil {
		for _, k := range c.Welcome.Keys {
			if strings.TrimSpace(k.Key) == "" {
				return Content{}, fmt.Errorf("every line under keys needs a key")
			}
		}
	}
	return c, nil
}

func (c Content) Spelled() string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(c)
	return strings.TrimSpace(b.String())
}

func (c Content) WelcomeOver(base Welcome) Welcome {
	if c.Welcome == nil {
		return base
	}
	if c.Welcome.Title != "" {
		base.Title = c.Welcome.Title
	}
	if c.Welcome.Lede != "" {
		base.Lede = c.Welcome.Lede
	}
	if len(c.Welcome.Keys) > 0 {
		base.Keys = c.Welcome.Keys
	}
	return base
}

func (c Content) AbilitiesOver(base []AbilityText) []AbilityText {
	byID := map[string]AbilityText{}
	for _, a := range c.Abilities {
		byID[a.ID] = a
	}
	merged := make(map[string]AbilityText, len(base))
	for _, a := range base {
		if o, ok := byID[a.ID]; ok {
			if o.Hidden {
				continue
			}
			if o.Title != "" {
				a.Title = o.Title
			}
			if o.Says != "" {
				a.Says = o.Says
			}
			if len(o.Examples) > 0 {
				a.Examples = o.Examples
			}
		}
		merged[a.ID] = a
	}
	var out []AbilityText
	for _, o := range c.Abilities {
		if a, ok := merged[o.ID]; ok {
			out = append(out, a)
			delete(merged, o.ID)
		}
	}
	for _, a := range base {
		if m, ok := merged[a.ID]; ok {
			out = append(out, m)
		}
	}
	return out
}
