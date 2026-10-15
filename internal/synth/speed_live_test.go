//go:build integration

package synth

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/uiact"
)

func postmanSized(n int) uiact.Tree {
	t := uiact.Tree{App: "Postman", Window: "Postman"}
	add := func(role, title string) {
		i := len(t.Elements)
		t.Elements = append(t.Elements, uiact.Element{
			Role: role, Title: title, Enabled: true,
			Position: []int{20 + (i%8)*120, 40 + (i/8)*18}, Size: []int{110, 16}, Path: []int{0, i},
		})
	}
	add("AXButton", "Environments")
	add("AXButton", "Collections")
	add("AXComboBox", "Environment")
	add("AXRow", "Variable")
	add("AXButton", "Create")
	for i := len(t.Elements); i < n; i++ {
		switch i % 4 {
		case 0:
			add("AXButton", fmt.Sprintf("Request %d", i))
		case 1:
			add("AXStaticText", fmt.Sprintf("GET /api/items/%d", i))
		case 2:
			add("AXRow", fmt.Sprintf("Header %d", i))
		default:
			add("AXLink", fmt.Sprintf("Docs %d", i))
		}
	}
	return t
}

func TestSpeedOfPlanAndStepLive(t *testing.T) {
	c, model := liveClient(t)
	c.Model = model
	task := `The person is looking at this window and asked: "how do I set the baseUrl"`
	for _, n := range []int{100, 376} {
		tree := postmanSized(n)
		for round := 0; round < 3; round++ {
			start := time.Now()
			_, err := c.WritePlan(context.Background(), PlanRequest{Task: task, Tree: tree, Most: 12})
			t.Logf("%d controls, plan: %s %v", n, time.Since(start).Round(10*time.Millisecond), err)
			time.Sleep(5 * time.Second)
			start = time.Now()
			_, err = c.WriteStep(context.Background(), StepRequest{Task: task, Tree: tree})
			t.Logf("%d controls, step: %s %v", n, time.Since(start).Round(10*time.Millisecond), err)
			time.Sleep(5 * time.Second)
		}
	}
}
