package keychain

import "errors"

var (
	ErrNotFound    = errors.New("keychain: nothing is stored under that name")
	ErrUnavailable = errors.New("keychain: this build cannot reach the keychain")
)

type Store interface {
	Get(service, account string) (string, error)
	Set(service, account, secret string) error
	Delete(service, account string) error
}

type System struct{}

func (System) Get(service, account string) (string, error) { return get(service, account) }

func (System) Set(service, account, secret string) error { return set(service, account, secret) }

func (System) Delete(service, account string) error { return remove(service, account) }
