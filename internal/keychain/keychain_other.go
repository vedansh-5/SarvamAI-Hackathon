//go:build !darwin || !cgo

package keychain

func get(string, string) (string, error) { return "", ErrUnavailable }

func set(string, string, string) error { return ErrUnavailable }

func remove(string, string) error { return ErrUnavailable }
