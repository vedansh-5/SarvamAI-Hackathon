package engine

import "github.com/vedansh/vans/internal/route"

type Ability struct {
	Title    string   `json:"title"`
	Says     string   `json:"says"`
	Examples []string `json:"examples,omitempty"`
}

func (e *Engine) Abilities() []Ability {
	var out []Ability
	add := func(can bool, a Ability) {
		if can {
			out = append(out, a)
		}
	}

	add(e.canRun(route.KindAppGuide), Ability{
		Title:    "Show you how",
		Says:     "Ask how to do something in any app. VANS points at each step on your screen while you do it yourself.",
		Examples: []string{"how do I add a new layer", "show me how to export this as a PDF"},
	})
	add(e.canRun(route.KindAppAction), Ability{
		Title:    "Do it for you",
		Says:     "Switch to Do it for me and VANS does the steps on screen where you can watch. It asks before anything that changes your work.",
		Examples: []string{"make a new note called Groceries"},
	})
	add(e.canRun(route.KindScreenRead), Ability{
		Title:    "Read what is on screen",
		Says:     "Ask about the window in front: an error, a status, a long message.",
		Examples: []string{"what does this error say", "summarise this window"},
	})
	add(e.canRun(route.KindPageHere), Ability{
		Title:    "Answer about the page you are on",
		Says:     "Questions about the web page in front of you, answered from the page itself.",
		Examples: []string{"what is this page about"},
	})
	_, marks := e.pageAsker()
	add(marks || e.canRun(route.KindPageStyle), Ability{
		Title:    "Mark things on a page",
		Says:     "Highlights only the words that matter, in your colour. Esc on the page takes the marks off.",
		Examples: []string{"highlight the ones that start with 23", `highlight "Pending"`},
	})
	add(e.canRun(route.KindWebSearch), Ability{
		Title:    "Look things up",
		Says:     "Searches the web or reads a site, and answers with where it found it.",
		Examples: []string{"search for flights to Goa", "the top stories on news.ycombinator.com"},
	})
	add(e.canRun(route.KindText), Ability{
		Title:    "Work on text",
		Says:     "Translate, shorten, correct or reply to text you have selected.",
		Examples: []string{"translate the selected text to Hindi", "make this shorter"},
	})
	add(e.canRun(route.KindFiles), Ability{
		Title:    "Tidy files",
		Says:     "Moves, renames and sorts files in the folders you name, and can put them back.",
		Examples: []string{"move the PDFs on my Desktop into a folder called Papers"},
	})
	add(e.canRun(route.KindSetting), Ability{
		Title:    "Change a setting",
		Says:     "Volume, mute, dark or light, and a few more, without opening System Settings.",
		Examples: []string{"turn on dark mode", "set the volume to 30%"},
	})
	add(e.Memory != nil, Ability{
		Title:    "Remember things",
		Says:     "Facts VANS uses when a task needs them. Ask what it remembers, or tell it to forget.",
		Examples: []string{"remember that my home city is Delhi", "what do you remember"},
	})
	add(e.Routines != nil, Ability{
		Title:    "Save a routine",
		Says:     "Keep a request under a name and run it again by saying the name.",
		Examples: []string{"save that as routine morning report", "list my routines"},
	})
	add(e.Schedules != nil, Ability{
		Title:    "Do it later",
		Says:     "Runs a request at a time you say, once or every day.",
		Examples: []string{"every weekday at 9am run morning report", "in 20 minutes remind me to stretch"},
	})
	add(e.Clicks != nil, Ability{
		Title: "Learn what you do often",
		Says:  "Notices what you do at the same moment each day and offers it in a line below the dot. Switch it off in Setup, under Learning.",
	})
	return out
}
