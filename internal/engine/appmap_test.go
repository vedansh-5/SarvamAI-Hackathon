package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type memoryMap struct {
	edges map[string][]store.MapEdge
	tasks []store.Run
}

func (m *memoryMap) LearnEdge(_ context.Context, app string, e store.MapEdge) error {
	if m.edges == nil {
		m.edges = map[string][]store.MapEdge{}
	}
	m.edges[app] = append(m.edges[app], e)
	return nil
}

func (m *memoryMap) KnownEdges(_ context.Context, app string) ([]store.MapEdge, error) {
	return m.edges[app], nil
}

func (m *memoryMap) RecordTask(_ context.Context, r store.Run) error {
	m.tasks = append(m.tasks, r)
	return nil
}

func chatList() uiact.Tree {
	return uiact.Tree{App: "Chat", Window: "Chat", Elements: []uiact.Element{
		{Role: "AXButton", Title: "Suhani", Enabled: true, Position: []int{10, 10}, Size: []int{100, 30}, Path: []int{0}},
		{Role: "AXButton", Title: "New Chat", Enabled: true, Position: []int{10, 50}, Size: []int{100, 30}, Path: []int{1}},
	}}
}

func openChat() uiact.Tree {
	t := chatList()
	t.Elements = append(t.Elements,
		uiact.Element{Role: "AXTextArea", Title: "Compose message", Enabled: true, Position: []int{200, 400}, Size: []int{300, 30}, Path: []int{2}},
		uiact.Element{Role: "AXButton", Title: "Voice message", Enabled: true, Position: []int{520, 400}, Size: []int{30, 30}, Path: []int{3}},
		uiact.Element{Role: "AXStaticText", Title: "hello there, a message", Enabled: true, Position: []int{200, 100}, Size: []int{300, 20}, Path: []int{4}},
	)
	return t
}

type opensAChat struct{ fakeScreen }

func (o *opensAChat) Capture(context.Context, string) (uiact.Tree, error) {
	o.captures++
	if o.acts > 0 {
		return openChat(), nil
	}
	return chatList(), nil
}

func clickSuhani() uiact.Step {
	return uiact.Step{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Suhani"}, Describe: "Open the chat with Suhani"}
}

func TestARunNotesWhatAClickLedTo(t *testing.T) {
	m := &memoryMap{}
	e := &Engine{UIScreen: &opensAChat{}, Map: m, Tasks: m, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: clickSuhani()}, {Done: true, Why: "open"}},
	}}

	plan := &Plan{Drive: &DrivePlan{App: "Chat", Task: "open the chat with Suhani", Shape: "open the chat with suhani"}}
	if _, err := e.runDrive(context.Background(), plan); err != nil {
		t.Fatalf("runDrive: %v", err)
	}

	edges := m.edges["Chat"]
	if len(edges) != 1 || edges[0].Title != "Suhani" {
		t.Fatalf("edges = %+v, want the one click noted", edges)
	}
	leads := strings.Join(edges[0].Leads, " | ")
	if !strings.Contains(leads, `AXTextArea "Compose message"`) || !strings.Contains(leads, `AXButton "Voice message"`) {
		t.Errorf("leads = %s, want the controls that appeared", leads)
	}
	if strings.Contains(leads, "hello there") {
		t.Errorf("leads = %s; what somebody wrote is not part of the application's map", leads)
	}
	if len(m.tasks) != 1 || m.tasks[0].Calls != 2 || m.tasks[0].Steps != 1 || !m.tasks[0].Finished || m.tasks[0].Mode != ModeDo {
		t.Errorf("tasks = %+v, want the run noted with what it cost", m.tasks)
	}
}

func TestThePlannerIsToldWhatIsKnownAboutTheScreenInFront(t *testing.T) {
	m := &memoryMap{edges: map[string][]store.MapEdge{"Chat": {
		{Role: "AXButton", Title: "Suhani", Leads: []string{`AXTextArea "Compose message"`, `AXButton "Voice message"`}},
		{Role: "AXTextArea", Title: "Compose message", Leads: []string{`AXButton "Send"`}},
		{Role: "AXButton", Title: "Settings", Leads: []string{`AXButton "Account"`}},
	}}}
	planner := &sequencedPlanner{plans: [][]uiact.Step{{clickSuhani()}}}
	e := &Engine{UIScreen: &opensAChat{}, Map: m, Plans: planner, Steps: &countingSteps{}}

	if _, err := e.driveOn(context.Background(), "Chat", "send hi to Suhani", "send hi to suhani", nil); err != nil {
		t.Fatalf("driveOn: %v", err)
	}
	if len(planner.requests) != 1 {
		t.Fatalf("planned %d time(s)", len(planner.requests))
	}
	known := strings.Join(planner.requests[0].Known, "\n")
	if !strings.Contains(known, `on screen now: AXButton "Suhani" leads to`) {
		t.Errorf("known = %q, want what the chat button leads to", known)
	}
	if !strings.Contains(known, `one step further: AXTextArea "Compose message" leads to: AXButton "Send"`) {
		t.Errorf("known = %q, want the screen after that as well", known)
	}
	if strings.Contains(known, "Settings") {
		t.Errorf("known = %q; a control that is neither here nor one step away is noise", known)
	}
}

func TestNothingIsNotedWithoutAMap(t *testing.T) {
	e := &Engine{UIScreen: &opensAChat{}, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: clickSuhani()}, {Done: true}},
	}}
	plan := &Plan{Drive: &DrivePlan{App: "Chat", Task: "open the chat with Suhani"}}
	if _, err := e.runDrive(context.Background(), plan); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
}

func manyChats(names ...string) uiact.Tree {
	t := uiact.Tree{App: "Chat", Window: "Chat", Elements: []uiact.Element{
		{Role: "AXGroup", Title: "List of chats", Enabled: true, Position: []int{0, 0}, Size: []int{300, 600}, Path: []int{0}},
	}}
	for i, n := range names {
		t.Elements = append(t.Elements, uiact.Element{
			Role: "AXButton", Title: n, Enabled: true,
			Position: []int{10, 10 + 40*i}, Size: []int{200, 30}, Path: []int{0, i},
		})
	}
	t.Elements = append(t.Elements, uiact.Element{Role: "AXButton", Title: "New Chat", Enabled: true, Position: []int{400, 10}, Size: []int{30, 30}, Path: []int{1}})
	return t
}

func TestOneChatTeachesTheMapAboutEveryChat(t *testing.T) {
	m := &memoryMap{}
	e := &Engine{Map: m}

	before := manyChats("Suhani", "Joe", "Mum")
	after := manyChats("Suhani", "Joe", "Mum")
	after.Elements = append(after.Elements,
		uiact.Element{Role: "AXTextArea", Title: "Compose message", Enabled: true, Path: []int{2}},
		uiact.Element{Role: "AXButton", Title: "Start voice call with Suhani", Enabled: true, Path: []int{3}},
	)
	e.learnFrom(context.Background(), before, after, clickSuhani())

	var general *store.MapEdge
	for i, edge := range m.edges["Chat"] {
		if strings.HasPrefix(edge.Title, anyInPrefix) {
			general = &m.edges["Chat"][i]
		}
	}
	if general == nil {
		t.Fatalf("edges = %+v, want one that speaks for every item in the list", m.edges["Chat"])
	}
	leads := strings.Join(general.Leads, " | ")
	if !strings.Contains(leads, "Compose message") || strings.Contains(leads, "Suhani") {
		t.Errorf("general leads = %s, want what any chat leads to and nothing naming this one", leads)
	}

	known := strings.Join(e.knownAbout(context.Background(), manyChats("Joe", "Mum", "Dad")), "\n")
	if !strings.Contains(known, `any AXButton in "List of chats" leads to`) || !strings.Contains(known, "Compose message") {
		t.Errorf("known = %q, want the list's rule offered on a screen where Suhani is not even listed", known)
	}
}

func TestAControlThatStandsAloneTeachesNothingGeneral(t *testing.T) {
	m := &memoryMap{}
	e := &Engine{Map: m}
	before := manyChats("Suhani", "Joe", "Mum")
	after := manyChats("Suhani", "Joe", "Mum")
	after.Elements = append(after.Elements, uiact.Element{Role: "AXTextField", Title: "Search name or number", Enabled: true, Path: []int{5}})

	e.learnFrom(context.Background(), before, after, uiact.Step{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "New Chat"}})
	for _, edge := range m.edges["Chat"] {
		if strings.HasPrefix(edge.Title, anyInPrefix) {
			t.Errorf("a lone button taught a rule for a list: %+v", edge)
		}
	}
}
