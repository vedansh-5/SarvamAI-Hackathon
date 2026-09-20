//go:build !darwin || !cgo

package uiact

import "errors"

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

func postKey(int) {}

func postText(string) {}

func frontmostApp() (string, bool) { return "", false }

func raiseApp(int) error { return errNoAccessibilityAPI }
