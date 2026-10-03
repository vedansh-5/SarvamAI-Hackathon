//go:build darwin && cgo

package keychain

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

func TestASecretIsSavedReadReplacedAndRemoved(t *testing.T) {
	if os.Getenv("VANS_KEYCHAIN_TEST") == "" {
		t.Skip("set VANS_KEYCHAIN_TEST=1 to write a throwaway item to the login keychain")
	}
	service := "dev.vedansh.vans.test"
	account := fmt.Sprintf("probe-%d", os.Getpid())
	k := System{}
	t.Cleanup(func() { _ = k.Delete(service, account) })

	if _, err := k.Get(service, account); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want nothing there yet", err)
	}
	if err := k.Set(service, account, "first"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := k.Set(service, account, "second value"); err != nil {
		t.Fatalf("Set again: %v", err)
	}
	if got, err := k.Get(service, account); err != nil || got != "second value" {
		t.Fatalf("got %q, %v; want the replaced value", got, err)
	}
	if err := k.Delete(service, account); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := k.Get(service, account); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want it gone", err)
	}
	if err := k.Delete(service, account); err != nil {
		t.Errorf("removing what is not there should be fine: %v", err)
	}
}
