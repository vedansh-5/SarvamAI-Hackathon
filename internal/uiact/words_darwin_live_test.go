//go:build darwin && cgo

package uiact

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLiveWordsLandOnTheirLabels(t *testing.T) {
	dir := os.Getenv("VANS_WORDS_BENCH")
	if dir == "" {
		t.Skip("set VANS_WORDS_BENCH to a folder of rendered windows (name.jpg) and their label boxes (name.json)")
	}
	pictures, _ := filepath.Glob(filepath.Join(dir, "*.jpg"))
	if len(pictures) == 0 {
		t.Fatalf("no pictures in %s", dir)
	}
	const width, height = 800.0, 400.0
	var labels, found, landed int
	var slowest time.Duration
	for _, picture := range pictures {
		data, err := os.ReadFile(picture)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(strings.TrimSuffix(picture, ".jpg") + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var want [][]any
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatalf("%s: %v", picture, err)
		}
		_, _ = ReadWords(data)
		started := time.Now()
		words, err := ReadWords(data)
		took := time.Since(started)
		slowest = max(slowest, took)
		if err != nil {
			t.Fatal(err)
		}
		var missed []string
		for _, w := range want {
			label := w[0].(string)
			x, y, wd, ht := w[1].(float64), w[2].(float64), w[3].(float64), w[4].(float64)
			labels++
			seen, inside := false, false
			for _, got := range words {
				if !SameLabel(got.Text, label) {
					continue
				}
				seen = true
				cx, cy := (got.X+got.W/2)*width, (got.Y+got.H/2)*height
				if cx >= x && cx <= x+wd && cy >= y && cy <= y+ht {
					inside = true
					break
				}
			}
			if seen {
				found++
			} else {
				missed = append(missed, label)
			}
			if inside {
				landed++
			} else if seen {
				missed = append(missed, "off: "+label)
			}
		}
		t.Logf("%s: %d labels, %d pieces in %s, missed %q", filepath.Base(picture), len(want), len(words), took.Round(time.Millisecond), missed)
	}
	t.Logf("found %d of %d labels (%.1f%%); %d of those landed inside their label (%.1f%%); slowest read %s",
		found, labels, 100*float64(found)/float64(labels), landed, 100*float64(landed)/float64(max(1, found)), slowest.Round(time.Millisecond))
	if float64(landed) < 0.99*float64(found) {
		t.Errorf("only %d of %d found labels landed inside them", landed, found)
	}
}
