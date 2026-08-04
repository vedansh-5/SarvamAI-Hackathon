package appleevents

import (
	"context"
	"encoding/json"
	"fmt"
)

// Notes drives the macOS Notes application.
type Notes struct{ Runner Runner }

// Note is a note as read back from Notes.app.
type Note struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Plaintext string `json:"plaintext"`
}

// jsLiteral renders v as a JavaScript literal safe to embed in a script.
//
// encoding/json escapes quotes, backslashes, control characters, and
// U+2028/U+2029, so the result is always a valid JS literal. This is what
// keeps user content from breaking out of the generated script.
func jsLiteral(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("appleevents: encode literal: %w", err)
	}
	return string(b), nil
}

// Create makes a note whose body is the given HTML and returns its id.
//
// Per Notes.sdef the note class exposes body (HTML, writable) and name
// (writable, normally the first line of the body). id is read-only and is what
// read-back verification keys on.
func (n Notes) Create(ctx context.Context, bodyHTML string) (string, error) {
	body, err := jsLiteral(bodyHTML)
	if err != nil {
		return "", err
	}

	script := fmt.Sprintf(`
const app = Application('Notes');
const note = app.make({ new: 'note', withProperties: { body: %s } });
JSON.stringify({ id: note.id() });
`, body)

	out, err := n.Runner.Run(ctx, script)
	if err != nil {
		return "", err
	}

	var res struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return "", fmt.Errorf("appleevents: decode create result: %w (output %q)", err, out)
	}
	if res.ID == "" {
		return "", fmt.Errorf("appleevents: create returned no id (output %q)", out)
	}
	return res.ID, nil
}

// Get reads a note back by id.
func (n Notes) Get(ctx context.Context, id string) (Note, error) {
	lit, err := jsLiteral(id)
	if err != nil {
		return Note{}, err
	}

	script := fmt.Sprintf(`
const app = Application('Notes');
const note = app.notes.byId(%s);
JSON.stringify({
  id: note.id(),
  name: note.name(),
  plaintext: note.plaintext()
});
`, lit)

	out, err := n.Runner.Run(ctx, script)
	if err != nil {
		return Note{}, err
	}

	var note Note
	if err := json.Unmarshal(out, &note); err != nil {
		return Note{}, fmt.Errorf("appleevents: decode note: %w (output %q)", err, out)
	}
	return note, nil
}

// CreateVerified makes a note and reads it back.
//
// A write that is not read back is a write we only assume happened. The
// AppleEvents tier is the one place a side effect can actually be confirmed,
// so every write on it goes through here.
func (n Notes) CreateVerified(ctx context.Context, bodyHTML string) (Note, error) {
	id, err := n.Create(ctx, bodyHTML)
	if err != nil {
		return Note{}, err
	}
	return n.Get(ctx, id)
}
