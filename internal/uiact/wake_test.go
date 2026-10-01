package uiact

import (
	"context"
	"testing"
	"time"
)

func readable() Tree {
	t := Tree{App: "Thing"}
	for _, n := range []string{"Explorer", "Search", "Source Control", "Run", "Extensions", "Settings"} {
		t.Elements = append(t.Elements, Element{Role: "AXButton", Title: n, Enabled: true, Size: []int{40, 40}})
	}
	return t
}

func wakingApp(t *testing.T, pid int, wakes bool, deepAfter int) (shallow, deep, woke *int) {
	t.Helper()
	w, d, k, p, b, wait, poll := walker, deepWalker, waker, askTheProcessList, askTheBundles, WakeWait, WakePoll
	shallow, deep, woke = new(int), new(int), new(int)

	walker = func(int, string) (Tree, error) { *shallow++; return Tree{App: "Thing"}, nil }
	deepWalker = func(int, string) (Tree, error) {
		*deep++
		if wakes && *deep > deepAfter {
			return readable(), nil
		}
		return Tree{App: "Thing"}, nil
	}
	waker = func(int) bool { *woke++; return true }
	askTheProcessList = func(context.Context, string) (int, bool) { return pid, true }
	WakeWait, WakePoll = 60*time.Millisecond, 5*time.Millisecond

	t.Cleanup(func() {
		walker, deepWalker, waker, askTheProcessList, askTheBundles, WakeWait, WakePoll = w, d, k, p, b, wait, poll
		wokenMu.Lock()
		delete(woken, pid)
		delete(asleep, pid)
		wokenMu.Unlock()
	})
	return
}

func TestAnAppThatPublishesNothingIsAskedToAndThenRead(t *testing.T) {
	shallow, deep, woke := wakingApp(t, 91001, true, 2)

	tree, err := Capture(context.Background(), nil, "Thing")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if Assess(tree).Verdict != Ready {
		t.Fatalf("verdict %s, want the window read once it had woken", Assess(tree).Verdict)
	}
	if *woke != 1 || *deep != 3 {
		t.Errorf("woke %d time(s) and read deeply %d; want one wake and a wait for it to take", *woke, *deep)
	}

	before := *shallow
	if _, err := Capture(context.Background(), nil, "Thing"); err != nil {
		t.Fatal(err)
	}
	if *shallow != before || *woke != 1 {
		t.Errorf("read shallow %d more time(s) and woke %d; an application known to need a deep read gets one straight away", *shallow-before, *woke)
	}
}

func TestAnAppThatStaysSilentIsNotAskedAgainEveryTime(t *testing.T) {
	_, deep, woke := wakingApp(t, 91002, false, 0)
	ctx := context.Background()

	tree, _ := Capture(ctx, nil, "Thing")
	if Assess(tree).Verdict != Blind {
		t.Fatal("an application that never answered was reported as readable")
	}
	readsAfterFirst := *deep
	_, _ = Capture(ctx, nil, "Thing")
	_, _ = Capture(ctx, nil, "Thing")
	if *woke != 1 || *deep != readsAfterFirst {
		t.Errorf("woke %d time(s), %d more deep read(s); the coach reads twice a second and must not wait three seconds each time", *woke, *deep-readsAfterFirst)
	}
}
