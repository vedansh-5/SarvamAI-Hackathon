//go:build integration && darwin && cgo

package uiact

import (
	"bufio"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func openTextPad(t *testing.T, mode, word string) int {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "VANS basics")
	if out, err := exec.Command("swiftc", "-O", "-o", bin, "testdata/textpad.swift").CombinedOutput(); err != nil {
		t.Skipf("swiftc could not build the test window: %v\n%s", err, out)
	}
	pad := exec.Command(bin, mode, word)
	out, err := pad.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := pad.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pad.Process.Kill(); _ = pad.Wait() })

	ready := make(chan bool, 1)
	go func() { ready <- bufio.NewScanner(out).Scan() }()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("the test window closed before it was ready")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the test window never said it was ready")
	}
	time.Sleep(300 * time.Millisecond)
	return pad.Process.Pid
}

func clipboard(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("pbpaste").Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestLiveTheSelectionIsReadInEveryKindOfTextBox(t *testing.T) {
	if !trusted() {
		t.Skip("this process may not use accessibility; allow the terminal in Privacy & Security, Accessibility")
	}
	for _, c := range []struct{ mode, word, how string }{
		{"plain", "bedazzled", "a text box that says what is selected"},
		{"quiet", "flabbergasted", "a text box that only gives it through Copy, as WhatsApp does"},
	} {
		t.Run(c.mode, func(t *testing.T) {
			pid := openTextPad(t, c.mode, c.word)
			before := clipboard(t)
			started := time.Now()
			got, ok := readSelection(pid)
			took := time.Since(started)
			if !ok || got != c.word {
				t.Fatalf("%s: read %q (%v), want %q", c.how, got, ok, c.word)
			}
			if after := clipboard(t); after != before {
				t.Errorf("the clipboard was not put back: %q, was %q", after, before)
			}
			if took > time.Second {
				t.Errorf("reading the selection took %s, want under a second", took)
			}
			t.Logf("%s: %q in %s", c.how, got, took.Round(time.Millisecond))
		})
	}
}
