package engine

import (
	"context"
	"testing"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/screen"
)

func pageAction() route.Route {
	return route.Route{
		Kind:  route.KindPageAction,
		Slots: map[string]string{"action": "type hello"},
	}
}

// Seen live: TextEdit in front, "type hello", and VANS asked a model how to type
// into a LinkedIn profile open in a browser nobody was looking at.
func TestAPageActionGoesToTheApplicationInFront(t *testing.T) {
	e := &Engine{UIScreen: &fakeScreen{front: "TextEdit"}}

	got := e.aimedAtWhatIsInFront(context.Background(), pageAction(),
		&screen.Context{AppName: "TextEdit"})

	if got.Kind != route.KindAppAction {
		t.Fatalf("kind = %s, want it redirected to an application action", got.Kind)
	}
	if got.Slot("app") != "TextEdit" {
		t.Errorf("app = %q, want TextEdit", got.Slot("app"))
	}
	if got.Slot("action") != "type hello" {
		t.Errorf("action = %q, want it carried over", got.Slot("action"))
	}
}

// With a browser in front, the page really is the thing being looked at.
func TestAPageActionStaysOnThePageWhenABrowserIsInFront(t *testing.T) {
	e := &Engine{UIScreen: &fakeScreen{front: "Google Chrome"}}

	got := e.aimedAtWhatIsInFront(context.Background(), pageAction(),
		&screen.Context{AppName: "Google Chrome"})

	if got.Kind != route.KindPageAction {
		t.Errorf("kind = %s, want it left as a page action", got.Kind)
	}
}

// Only page actions are redirected. Anything else was already aimed on purpose.
func TestOtherKindsAreLeftAlone(t *testing.T) {
	e := &Engine{UIScreen: &fakeScreen{front: "TextEdit"}}
	r := route.Route{Kind: route.KindPageStyle, Slots: map[string]string{"intent": "x"}}

	if got := e.aimedAtWhatIsInFront(context.Background(), r, nil); got.Kind != route.KindPageStyle {
		t.Errorf("kind = %s, want it untouched", got.Kind)
	}
}

// The stored decision is shared by every request of this shape, so redirecting
// this one must not rewrite the one that was passed in.
func TestRedirectingDoesNotChangeTheRouteItWasGiven(t *testing.T) {
	e := &Engine{UIScreen: &fakeScreen{front: "TextEdit"}}
	original := pageAction()

	e.aimedAtWhatIsInFront(context.Background(), original, &screen.Context{AppName: "TextEdit"})

	if _, set := original.Slots["app"]; set {
		t.Error("the caller's route gained an app slot; the redirect leaked into shared state")
	}
}
