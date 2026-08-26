package screen_test

import (
	"encoding/json"
	"testing"

	"github.com/vedansh/vans/internal/screen"
)

func TestContextValidation(t *testing.T) {
	ctx := screen.Context{
		AppName:     "Google Chrome",
		BundleID:    "com.google.Chrome",
		WindowTitle: "Hacker News",
		PID:         1234,
		Bounds: screen.Rect{
			X:      0,
			Y:      0,
			Width:  1920,
			Height: 1080,
		},
	}

	data, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("failed to marshal context: %v", err)
	}

	var decoded screen.Context
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal context: %v", err)
	}

	if decoded.AppName != "Google Chrome" || decoded.BundleID != "com.google.Chrome" || decoded.WindowTitle != "Hacker News" || decoded.PID != 1234 {
		t.Fatalf("decoded context mismatch: %+v", decoded)
	}
	if decoded.Bounds.Width != 1920 || decoded.Bounds.Height != 1080 {
		t.Fatalf("decoded bounds mismatch: %+v", decoded.Bounds)
	}
}
