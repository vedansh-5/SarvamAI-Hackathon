//go:build !darwin || !cgo

package uiact

import "errors"

var errNoAccessibilityAPI = errors.New("uiact: this build was made without the accessibility API")

func walkApp(int, string) (Tree, error) { return Tree{}, errNoAccessibilityAPI }
