package synth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// AppChoiceRequest asks where a task is carried out.
type AppChoiceRequest struct {
	// Task is what the person asked, in their words.
	Task string

	// InFront is the application they were looking at when they asked. It is
	// context, not the answer — the whole point of asking is that the two are
	// often different.
	InFront string

	// Running is what has a window open right now, and Installed is everything
	// on the Mac. An application that is already open is the better answer when
	// either would do, because it costs the person nothing to get to.
	Running   []string
	Installed []string
}

// AppChoice is where the task is done.
type AppChoice struct {
	// App is the application, spelled as it is installed.
	App string

	// Why is one line for the person, used when they have to open it.
	Why string

	// Nowhere reports that no application on this Mac does this, which is a real
	// answer and better than naming something plausible.
	Nowhere bool
}

const whichAppSystemPrompt = `You are told what someone asked to do on their Mac, and you name the single application it is done in.

You are given the application they were looking at when they asked. Start from that one. When the request could be about it, it is: "how do I open settings", "where is search", "how do I export this" and "make the text bigger" asked while looking at an application are about that application, not about the Mac. Every application has its own settings, search and menus.

Name a different application only when the request is about something that lives elsewhere and says so: the wallpaper, Wi-Fi, the display, a named application, a kind of thing the one in front does not hold. People do ask "how do I change the wallpaper" while reading mail, and the wallpaper is not changed in a mail client.

You are also given what is open right now and what is installed. Rules:

- Name an application from the installed list, spelled exactly as it appears there. A name that is not on the list cannot be opened or read, so it is not an answer.
- Prefer one that is already open when either would do. It costs the person nothing to reach.
- The Mac's own appearance, wallpaper, network, sound, keyboard, displays, privacy and accounts are done in System Settings. The bare word "settings" or "preferences" is not that: with an application in front, it means that application's settings.
- Being asked this question is not a reason to send them somewhere else.
- If nothing installed does this, set nowhere and say why in one line. That is a good answer.

Give why as one plain line naming what the application is for in this context, as you would say it to the person while they open it. Not a justification of your choice.`

func whichAppSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"app":     map[string]any{"type": "string"},
			"why":     map[string]any{"type": "string"},
			"nowhere": map[string]any{"type": "boolean"},
		},
		"required":             []string{"app", "why", "nowhere"},
		"additionalProperties": false,
	}
}

// WhichApp names the application a task is carried out in.
//
// Its own call rather than a slot on the router, because the router decides what
// kind of thing was asked from the sentence alone, and this needs the machine:
// which applications exist, and which are open. The answer is small and the
// prompt is short, so it costs a fraction of a step.
func (c *OpenAICompatible) WhichApp(ctx context.Context, req AppChoiceRequest) (AppChoice, error) {
	if c.BaseURL == "" || c.Model == "" {
		return AppChoice{}, fmt.Errorf("synth: BaseURL and Model are required")
	}
	if strings.TrimSpace(req.Task) == "" {
		return AppChoice{}, fmt.Errorf("synth: no task to place")
	}

	system, err := c.systemFor(whichAppSystemPrompt, "app choice", whichAppSchema)
	if err != nil {
		return AppChoice{}, err
	}

	maxTokens := c.MaxTokens
	if maxTokens == 0 {
		maxTokens = int(DefaultMaxTokens)
	}

	raw, err := c.post(ctx, chatRequest{
		Model:     c.easy(),
		MaxTokens: maxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: BuildAppChoicePrompt(req)},
		},
		ResponseFormat: c.responseFormat("app_choice", whichAppSchema, false),
	})
	if err != nil {
		return AppChoice{}, err
	}

	text := extractJSONObject(raw)
	if text == "" {
		return AppChoice{}, fmt.Errorf("synth: no JSON object in the response (got %q)", truncate(raw, 300))
	}

	var answer struct {
		App     string `json:"app"`
		Why     string `json:"why"`
		Nowhere bool   `json:"nowhere"`
	}
	if err := json.Unmarshal([]byte(text), &answer); err != nil {
		return AppChoice{}, fmt.Errorf("synth: decode app choice: %w (from %q)", err, truncate(text, 300))
	}

	out := AppChoice{
		App:     strings.TrimSpace(answer.App),
		Why:     strings.TrimSpace(answer.Why),
		Nowhere: answer.Nowhere,
	}
	if !out.Nowhere && out.App == "" {
		return AppChoice{}, fmt.Errorf("synth: neither an application nor a reason there is none")
	}
	return out, nil
}

// MaxAppsListed bounds the installed list sent to the model. A Mac with three
// hundred applications would otherwise spend most of the prompt on names, and
// the ones that matter for a task like this are few and well known.
const MaxAppsListed = 120

func BuildAppChoicePrompt(req AppChoiceRequest) string {
	var b strings.Builder

	fmt.Fprintf(&b, "They asked: %q\n\n", req.Task)
	if front := strings.TrimSpace(req.InFront); front != "" {
		fmt.Fprintf(&b, "They were looking at: %s\n\n", front)
	}
	if len(req.Running) > 0 {
		fmt.Fprintf(&b, "Open right now:\n%s\n\n", bullets(req.Running, MaxAppsListed))
	}
	fmt.Fprintf(&b, "Installed:\n%s\n", bullets(req.Installed, MaxAppsListed))

	return b.String()
}

func bullets(names []string, limit int) string {
	if len(names) > limit {
		names = names[:limit]
	}
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "- %s\n", n)
	}
	return strings.TrimRight(b.String(), "\n")
}
