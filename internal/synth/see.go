package synth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

const seeSystemPrompt = `You are shown a picture of one application window on a Mac. List the things in it a person could act on: buttons, text fields, links, tabs, menu items, checkboxes, and the rows or items of a list.

For each one give:

- kind: one of button, field, link, tab, menu, checkbox, item
- name: the words on it, exactly as shown. For an icon with no words, what a person would call it in one or two words ("Search", "Settings", "Close"). Never invent words that are not there for a control that has words.
- value: for a field, the text somebody has typed into it; empty otherwise. Grey hint text that says what the field is for is its name, not its value
- x0, y0, x1, y1: its box in the picture, each from 0 to 1000, where 0,0 is the top-left corner of the picture and 1000,1000 the bottom-right. The box should fit the control closely.

List only what is visible. Go top to bottom, left to right. At most 80, most useful first when there are more. Do not list plain text that cannot be acted on, and do not describe the picture.`

func seeSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"window": map[string]any{"type": "string"},
			"controls": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"kind":  map[string]any{"type": "string", "enum": []string{"button", "field", "link", "tab", "menu", "checkbox", "item"}},
						"name":  map[string]any{"type": "string"},
						"value": map[string]any{"type": "string"},
						"x0":    map[string]any{"type": "integer"},
						"y0":    map[string]any{"type": "integer"},
						"x1":    map[string]any{"type": "integer"},
						"y1":    map[string]any{"type": "integer"},
					},
					"required":             []string{"kind", "name", "x0", "y0", "x1", "y1"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"window", "controls"},
		"additionalProperties": false,
	}
}

type SeeRequest struct {
	App  string
	JPEG []byte
}

type Seen struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Value string `json:"value"`
	X0    int    `json:"x0"`
	Y0    int    `json:"y0"`
	X1    int    `json:"x1"`
	Y1    int    `json:"y1"`
}

type SeeAnswer struct {
	Window   string
	Controls []Seen
}

const MostSeen = 80

func (c *OpenAICompatible) SeeScreen(ctx context.Context, req SeeRequest) (SeeAnswer, error) {
	if c.BaseURL == "" || c.Model == "" {
		return SeeAnswer{}, fmt.Errorf("synth: BaseURL and Model are required")
	}
	if len(req.JPEG) == 0 {
		return SeeAnswer{}, fmt.Errorf("synth: no picture to look at")
	}

	system, err := c.systemFor(seeSystemPrompt, "controls", seeSchema)
	if err != nil {
		return SeeAnswer{}, err
	}

	raw, err := c.post(ctx, chatRequest{
		Model:     c.Model,
		MaxTokens: 6000,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{
				Role:    "user",
				Content: fmt.Sprintf("This is the front window of %s. List what can be acted on.", req.App),
				Image:   "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(req.JPEG),
			},
		},
		ResponseFormat: c.responseFormat("seen_controls", seeSchema, false),
	})
	if err != nil {
		return SeeAnswer{}, err
	}

	text := extractJSONObject(raw)
	if text == "" {
		return SeeAnswer{}, fmt.Errorf("synth: no JSON object in the response (got %q)", truncate(raw, 300))
	}
	var answer struct {
		Window   string `json:"window"`
		Controls []Seen `json:"controls"`
	}
	if err := json.Unmarshal([]byte(text), &answer); err != nil {
		return SeeAnswer{}, fmt.Errorf("synth: decode what was seen: %w (from %q)", err, truncate(text, 300))
	}

	out := SeeAnswer{Window: strings.TrimSpace(answer.Window)}
	for _, s := range answer.Controls {
		s.Name = strings.TrimSpace(s.Name)
		if s.Name == "" || s.X1 <= s.X0 || s.Y1 <= s.Y0 {
			continue
		}
		if s.X0 < 0 || s.Y0 < 0 || s.X1 > 1000 || s.Y1 > 1000 {
			continue
		}
		out.Controls = append(out.Controls, s)
		if len(out.Controls) == MostSeen {
			break
		}
	}
	return out, nil
}
