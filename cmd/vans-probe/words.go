package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/uiact"
)

type wordScore struct {
	app      string
	named    int
	read     int
	landed   int
	took     time.Duration
	misses   []string
	problems string
}

func measureWords(ctx context.Context, screen uiact.System, apps []string) error {
	if len(apps) == 0 {
		return fmt.Errorf("name the applications to measure; each is brought forward for a moment")
	}
	was, _ := screen.Frontmost(ctx)
	var scores []wordScore
	for _, app := range apps {
		scores = append(scores, measureOne(ctx, screen, app))
	}
	if was != "" {
		_ = screen.BringToFront(ctx, was)
	}

	fmt.Printf("%-24s %6s %6s %7s %8s  %s\n", "APPLICATION", "NAMED", "READ", "LANDED", "READ IN", "MISSED OR OFF")
	var named, read, landed int
	for _, s := range scores {
		if s.problems != "" {
			fmt.Printf("%-24s %s\n", clip(s.app, 24), s.problems)
			continue
		}
		named, read, landed = named+s.named, read+s.read, landed+s.landed
		fmt.Printf("%-24s %6d %6d %7d %8s  %s\n", clip(s.app, 24), s.named, s.read, s.landed,
			s.took.Round(time.Millisecond), clip(strings.Join(s.misses, ", "), 70))
	}
	if read > 0 {
		fmt.Printf("\nOf %d named controls, the words found %d (%.0f%%); of those, %d landed inside the right control (%.1f%%).\n",
			named, read, 100*float64(read)/float64(max(1, named)), landed, 100*float64(landed)/float64(read))
	}
	return nil
}

func measureOne(ctx context.Context, screen uiact.System, app string) wordScore {
	score := wordScore{app: app}
	tree, err := screen.Capture(ctx, app)
	if err != nil {
		if err := screen.BringToFront(ctx, app); err != nil {
			score.problems = err.Error()
			return score
		}
		time.Sleep(400 * time.Millisecond)
		if tree, err = screen.Capture(ctx, app); err != nil {
			score.problems = err.Error()
			return score
		}
	}
	shot, err := screen.Shot(ctx, app)
	if err != nil {
		score.problems = err.Error()
		return score
	}
	started := time.Now()
	words, err := uiact.ReadWords(shot.Full)
	score.took = time.Since(started)
	if err != nil {
		score.problems = err.Error()
		return score
	}

	inWindow := func(x, y int) bool {
		return x >= shot.X && y >= shot.Y && x < shot.X+shot.W && y < shot.Y+shot.H
	}
	for _, el := range tree.Elements {
		label := strings.TrimSpace(el.Label())
		if label == "" || len([]rune(label)) > 40 || uiact.InMenuBar(el) || len(el.Position) != 2 || len(el.Size) != 2 || el.Size[0] < 4 || el.Size[1] < 4 {
			continue
		}
		if !inWindow(el.Position[0]+el.Size[0]/2, el.Position[1]+el.Size[1]/2) {
			continue
		}
		score.named++
		found, inside := false, false
		for _, w := range words {
			if !uiact.SameLabel(w.Text, label) {
				continue
			}
			found = true
			cx := shot.X + int((w.X+w.W/2)*float64(shot.W))
			cy := shot.Y + int((w.Y+w.H/2)*float64(shot.H))
			if cx >= el.Position[0] && cx <= el.Position[0]+el.Size[0] && cy >= el.Position[1] && cy <= el.Position[1]+el.Size[1] {
				inside = true
				break
			}
		}
		switch {
		case inside:
			score.read++
			score.landed++
		case found:
			score.read++
			score.misses = append(score.misses, "off: "+label)
		default:
			score.misses = append(score.misses, label)
		}
	}
	return score
}
