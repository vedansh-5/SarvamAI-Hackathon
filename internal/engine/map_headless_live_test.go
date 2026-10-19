//go:build integration

package engine

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/vedansh/vans/internal/headless"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/webscreen"
)

func liveModel(t *testing.T) (StepWriter, PlanWriter) {
	t.Helper()
	if os.Getenv("VANS_LIVE_MODEL") == "" {
		t.Skip("set VANS_LIVE_MODEL=1, with the model in the environment, to spend model calls on this")
	}
	backend, err := synth.FromEnv()
	if err != nil {
		t.Skipf("no model configured: %v", err)
	}
	steps, ok := backend.(StepWriter)
	plans, planned := backend.(PlanWriter)
	if !ok || !planned {
		t.Skip("the configured model cannot write steps and plans")
	}
	return steps, plans
}

type appState struct {
	Dark   bool `json:"dark"`
	Large  bool `json:"large"`
	Motion bool `json:"motion"`
}

func runInTheApp(t *testing.T, c *headless.Chrome, site string, m *memoryMap, task string, steps StepWriter, plans PlanWriter) (store.Run, appState) {
	t.Helper()
	if err := c.OpenInFront(c.Ctx, site+"/index.html"); err != nil {
		t.Fatal(err)
	}
	screen := &webscreen.Screen{Page: c, Browser: "Google Chrome", RealDrag: c.Drag}
	tree, err := screen.Capture(c.Ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	said := &lines{}
	e := &Engine{UIScreen: screen, Steps: steps, Plans: plans, Map: m, Tasks: m, Log: said.logf}
	before := len(m.tasks)
	plan, err := e.driveOn(c.Ctx, tree.App, task, task, nil)
	if err != nil {
		t.Fatalf("%s: %v", task, err)
	}
	_, err = e.runDrive(c.Ctx, plan)
	t.Logf("%s:\n%s", task, said.all())
	if err != nil {
		t.Fatalf("%s: %v", task, err)
	}
	if len(m.tasks) != before+1 {
		t.Fatalf("%s: the run was not written down", task)
	}
	var state appState
	if err := json.Unmarshal([]byte(c.Eval("JSON.stringify(state)")), &state); err != nil {
		t.Fatal(err)
	}
	return m.tasks[len(m.tasks)-1], state
}

func TestLiveTheMapSavesCallsOnTheNextTaskInTheSameApp(t *testing.T) {
	steps, plans := liveModel(t)
	c := headless.Open(t)
	site, _ := headless.Serve(t, "testdata/app")

	learnt := &memoryMap{}
	first, state := runInTheApp(t, c, site, learnt, "turn on dark mode", steps, plans)
	if !state.Dark {
		t.Fatalf("dark mode is not on after the first run")
	}
	if len(learnt.edges) == 0 {
		t.Fatal("the first run taught the map nothing")
	}

	withMap, state := runInTheApp(t, c, site, learnt, "turn on large text", steps, plans)
	if !state.Large {
		t.Fatalf("large text is not on with the map")
	}
	without, state := runInTheApp(t, c, site, &memoryMap{}, "turn on large text", steps, plans)
	if !state.Large {
		t.Fatalf("large text is not on without the map")
	}

	t.Logf("calls: first run %d; the next task with the map %d, without it %d", first.Calls, withMap.Calls, without.Calls)
	if withMap.Calls >= without.Calls {
		t.Errorf("the map saved nothing: %d call(s) with it, %d without", withMap.Calls, without.Calls)
	}
}
