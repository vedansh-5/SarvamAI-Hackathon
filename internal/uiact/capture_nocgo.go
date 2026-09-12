//go:build !darwin || !cgo

package uiact

import "errors"

var errNoAccessibilityAPI = errors.New("uiact: this build was made without the accessibility API")

func walkApp(int, string) (Tree, error) { return Tree{}, errNoAccessibilityAPI }

func Trusted() bool { return false }

func elementAt(int, int, int) (string, string, bool, error) {
	return "", "", false, errNoAccessibilityAPI
}
