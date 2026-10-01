//go:build !darwin || !cgo

package uiact

import (
	"context"
	"errors"
)

var errNoAccessibilityAPI = errors.New("uiact: this build was made without the accessibility API")

func walkApp(int, string) (Tree, error) { return Tree{}, errNoAccessibilityAPI }

func Trusted() bool { return false }

func elementAt(int, int, int) (string, string, bool, error) {
	return "", "", false, errNoAccessibilityAPI
}

func frontWindowAt(int, int) (int, string, bool) { return 0, "", false }

func windowOnScreen(int) bool { return true }

func displayBounds() (box, bool) { return box{}, false }

func postClick(int, int) {}

func postKey(int, uint64) {}

func postText(string) {}

func frontmostApp() (string, bool) { return "", false }

func appsOnScreen() []string { return nil }

func pidByWindowList(string) (int, bool) { return 0, false }

func raiseApp(int) error { return errNoAccessibilityAPI }

func pressElement(int, []int) (bool, error) { return false, errNoAccessibilityAPI }

func focusElement(int, []int) error { return errNoAccessibilityAPI }

func watchWindow(context.Context, int) (<-chan struct{}, error) { return nil, errNoAccessibilityAPI }

func scrollElement(int, []int) error { return errNoAccessibilityAPI }

func elementValue(int, []int) (string, bool) { return "", false }
