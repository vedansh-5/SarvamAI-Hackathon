package synth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/fileops"
)

type FilesRequest struct {
	Ask      string
	Folders  []fileops.Folder
	Failures []string
}

type FilesAnswer struct {
	Ops      []fileops.Op
	Why      string
	Question string
}

const filesSystemPrompt = `You are given what someone asked to be done with files on their Mac, and the contents of the folders they named: every item's name, whether it is a folder, its size and the day it was last changed. You write the list of operations that does it.

The operations:

- move: from an item's full path to a new full path in another folder
- rename: from an item's full path to a new name in the same folder, given as a full path
- copy: from a file's full path to a new full path
- trash: from an item's full path into the Bin
- mkdir: make a folder at the full path given as to

Rules that decide whether your list is usable:

- from is always the full path of an item you were shown, or of something an earlier operation in your list made. Nothing else exists as far as you are concerned.
- to is always a full path. The folder it is in must be one you were shown, a subfolder you were shown, or one you make earlier in the list with mkdir.
- Nothing is ever overwritten: to must not be the path of something that is already there.
- Nothing is deleted for good. trash is the only way to remove something, and only when the person asked for it to be removed, deleted or binned.
- Do what was asked and nothing else. Leave every item the request does not cover exactly where it is.
- Keep a file's extension when renaming it unless the request is to change it.
- When asked to sort or tidy with no names given, choose plain folder names yourself, such as Images, Documents, Archives and Installers, and leave folders that are already there where they are: deciding that is what was asked for.
- Ask only when the request turns on something only the person knows, such as which of two items with similar names they mean. Then give no operations and put the question in ask.
- If nothing you were shown fits the request, give no operations and say so in why.

Give why as one plain line saying what will happen, as you would say it to the person. You are told today's date, for requests about what is old or recent.`

func filesSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ops": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"op":   map[string]any{"type": "string", "enum": []string{fileops.Move, fileops.Rename, fileops.Copy, fileops.Trash, fileops.Mkdir}},
						"from": map[string]any{"type": "string"},
						"to":   map[string]any{"type": "string"},
					},
					"required":             []string{"op"},
					"additionalProperties": false,
				},
			},
			"why": map[string]any{"type": "string"},
			"ask": map[string]any{"type": "string"},
		},
		"required":             []string{"ops", "why"},
		"additionalProperties": false,
	}
}

func size(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func BuildFilesPrompt(req FilesRequest) string {
	var b strings.Builder
	b.WriteString("THEY ASKED\n")
	b.WriteString(strings.TrimSpace(req.Ask))
	fmt.Fprintf(&b, "\n\nTODAY IS %s\n", today())

	for _, folder := range req.Folders {
		fmt.Fprintf(&b, "\nFOLDER %s\n", folder.Path)
		if len(folder.Entries) == 0 {
			b.WriteString("  (empty)\n")
		}
		for _, e := range folder.Entries {
			if e.Folder {
				fmt.Fprintf(&b, "  [folder] %s\n", e.Name)
				continue
			}
			fmt.Fprintf(&b, "  %s    %s    %s\n", e.Name, size(e.Size), e.Modified.Format("2006-01-02"))
		}
		if folder.LeftOut > 0 {
			fmt.Fprintf(&b, "  (and %d more that are not shown and cannot be touched)\n", folder.LeftOut)
		}
	}

	if len(req.Failures) > 0 {
		b.WriteString("\nYOUR LAST LIST WAS REFUSED\n")
		for _, f := range req.Failures {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
		b.WriteString("Fix that and give the whole list again.\n")
	}
	return b.String()
}

const MostTokensForFiles = 12000

func (c *OpenAICompatible) WriteFileOps(ctx context.Context, req FilesRequest) (FilesAnswer, error) {
	if c.BaseURL == "" || c.Model == "" {
		return FilesAnswer{}, fmt.Errorf("synth: BaseURL and Model are required")
	}
	if strings.TrimSpace(req.Ask) == "" {
		return FilesAnswer{}, fmt.Errorf("synth: nothing was asked of the files")
	}

	system, err := c.systemFor(filesSystemPrompt, "files", filesSchema)
	if err != nil {
		return FilesAnswer{}, err
	}
	maxTokens := c.MaxTokens
	if maxTokens == 0 {
		maxTokens = MostTokensForFiles
	}

	raw, err := c.post(ctx, chatRequest{
		Model:     c.Model,
		MaxTokens: maxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: BuildFilesPrompt(req)},
		},
		ResponseFormat: c.responseFormat("file_operations", filesSchema, false),
	})
	if err != nil {
		return FilesAnswer{}, err
	}

	body := extractJSONObject(raw)
	if body == "" {
		return FilesAnswer{}, fmt.Errorf("synth: no JSON object in the response (got %q)", truncate(raw, 300))
	}
	var answer struct {
		Ops []fileops.Op `json:"ops"`
		Why string       `json:"why"`
		Ask string       `json:"ask"`
	}
	if err := json.Unmarshal([]byte(body), &answer); err != nil {
		return FilesAnswer{}, fmt.Errorf("synth: decode file operations: %w (from %q)", err, truncate(body, 300))
	}

	for i, op := range answer.Ops {
		if op.Do == fileops.Mkdir && strings.TrimSpace(op.To) == "" && strings.TrimSpace(op.From) != "" {
			answer.Ops[i].To, answer.Ops[i].From = op.From, ""
		}
	}

	out := FilesAnswer{Ops: answer.Ops, Why: strings.TrimSpace(answer.Why), Question: strings.TrimSpace(answer.Ask)}
	if len(out.Ops) == 0 && out.Question == "" {
		if q := questionIn(out.Why); q != nil {
			out.Question = q.Ask
		}
	}
	return out, nil
}
