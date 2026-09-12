package engine

import "testing"

// The panel stays first, so a hurried Return never lands on something that
// changes the world.
func TestDefaultSinksOfferTheApplicationInFront(t *testing.T) {
	e := &Engine{Sinks: DefaultSinks()}
	got := e.Destinations()

	if got[0].Name != PanelDestination {
		t.Fatalf("first destination is %q, want the panel", got[0].Name)
	}

	var app *Destination
	for i := range got {
		if got[i].Name == "app" {
			app = &got[i]
		}
	}
	if app == nil {
		t.Fatal("the application destination is not offered")
	}
	if !app.TouchesWorld {
		t.Error("typing into another application is not marked as touching the world")
	}
}
