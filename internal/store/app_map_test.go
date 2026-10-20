package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestTheMapKeepsWhatAControlLedToAndCountsHowOften(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "vans.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	edge := MapEdge{Role: "AXButton", Title: "New Chat", Leads: []string{`AXTextField "Search"`}}
	if err := s.LearnEdge(ctx, "WhatsApp", edge); err != nil {
		t.Fatal(err)
	}
	edge.Leads = []string{`AXTextField "Search name or number"`, `AXButton "New group"`}
	if err := s.LearnEdge(ctx, "WhatsApp", edge); err != nil {
		t.Fatal(err)
	}
	_ = s.LearnEdge(ctx, "Notes", MapEdge{Role: "AXButton", Title: "New Note", Leads: []string{`AXTextArea "Note"`}})
	_ = s.LearnEdge(ctx, "WhatsApp", MapEdge{Role: "AXButton", Title: "Nothing", Leads: nil})

	got, err := s.KnownEdges(ctx, "WhatsApp")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Seen != 2 || len(got[0].Leads) != 2 {
		t.Fatalf("edges = %+v, want the one control, seen twice, with what it led to most recently", got)
	}
}

func TestRunsAreRecordedNewestFirst(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "vans.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_ = s.RecordTask(ctx, Run{App: "WhatsApp", Shape: "send {s0}", Mode: "do", Steps: 3, Calls: 4})
	_ = s.RecordTask(ctx, Run{App: "WhatsApp", Shape: "send {s0}", Mode: "do", Steps: 3, Replayed: 3, Calls: 0, Finished: true, Clicked: 2, NextPressed: 1})

	runs, err := s.RecentTasks(ctx, 10)
	if err != nil || len(runs) != 2 {
		t.Fatalf("runs = %+v, err = %v", runs, err)
	}
	if runs[0].Calls != 0 || !runs[0].Finished || runs[1].Calls != 4 {
		t.Errorf("runs = %+v, want the newest first", runs)
	}
	if runs[0].Clicked != 2 || runs[0].NextPressed != 1 || runs[0].Seen != 0 {
		t.Errorf("runs[0] = %+v, want how its steps were noticed kept", runs[0])
	}
}
