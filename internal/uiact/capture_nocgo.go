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

func postMove(int, int) {}

func postDoubleClick(int, int) {}

func postRightClick(int, int) {}

func postScroll(int, int, int, int) {}

func postDrag(int, int, int, int) {}

func elementNumber(int, []int) (float64, bool) { return 0, false }

func setElementNumber(int, []int, float64) error { return errNoAccessibilityAPI }

func performOn(int, []int, string) (bool, error) { return false, errNoAccessibilityAPI }

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

func setElementValue(int, []int, string) error { return errNoAccessibilityAPI }

func windowOf(int) (int, int, int, int, int, bool) { return 0, 0, 0, 0, 0, false }

func selectedText(int) (string, bool) { return "", false }

func wakeApp(int) bool { return false }

func walkAppDeep(int, string) (Tree, error) { return Tree{}, errNoAccessibilityAPI }

func walkAppText(int, string) (Tree, error) { return Tree{}, errNoAccessibilityAPI }

func NoticeAt(int, int) (Noticed, bool) { return Noticed{}, false }

func ReadWords([]byte) ([]Word, error) { return nil, ErrCannotReadWords }
