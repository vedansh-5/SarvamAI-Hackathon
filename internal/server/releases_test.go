package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type shelf struct {
	latest Release
	linked []string
}

func (s *shelf) Latest(context.Context) (Release, error) { return s.latest, nil }

func (s *shelf) Link(_ context.Context, key string) (string, error) {
	s.linked = append(s.linked, key)
	return "https://downloads.example/" + key + "?signed", nil
}

func updateServer(t *testing.T, latest Release) (*httptest.Server, *shelf) {
	t.Helper()
	releases := &shelf{latest: latest}
	srv := httptest.NewServer(&Server{Ledger: NewMemory([]Tester{{Name: "Riya", Code: riya, Daily: 10}}), Releases: releases})
	t.Cleanup(srv.Close)
	return srv, releases
}

func checkForUpdate(t *testing.T, srv *httptest.Server, code, path string) (*http.Response, offered) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+UpdatePath+path, nil)
	if code != "" {
		req.Header.Set("Authorization", "Bearer "+code)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got offered
	if resp.StatusCode == http.StatusOK {
		_ = json.NewDecoder(resp.Body).Decode(&got)
	}
	return resp, got
}

var released = Release{
	Version: "0.2.0", Notes: "Habits", PubDate: "2026-10-15T10:00:00Z",
	Platforms: map[string]Build{"darwin-aarch64": {Key: "0.2.0/VANS_aarch64.app.tar.gz", Signature: "sig"}},
}

func TestANewerReleaseIsOfferedOnlyToATester(t *testing.T) {
	srv, releases := updateServer(t, released)

	if resp, _ := checkForUpdate(t, srv, "", "darwin/aarch64/0.1.0"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("without a code: %d, want 401", resp.StatusCode)
	}
	if resp, _ := checkForUpdate(t, srv, "not-a-code-at-all", "darwin/aarch64/0.1.0"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("with a wrong code: %d, want 401", resp.StatusCode)
	}
	if len(releases.linked) != 0 {
		t.Fatalf("a download link was made for someone without a code: %v", releases.linked)
	}

	resp, got := checkForUpdate(t, srv, riya, "darwin/aarch64/0.1.0")
	if resp.StatusCode != http.StatusOK || got.Version != "0.2.0" || got.Signature != "sig" || got.URL != "https://downloads.example/0.2.0/VANS_aarch64.app.tar.gz?signed" {
		t.Fatalf("offered %d %+v", resp.StatusCode, got)
	}
}

func TestNothingIsOfferedWhenUpToDateOrForAnotherMac(t *testing.T) {
	srv, _ := updateServer(t, released)
	for _, path := range []string{"darwin/aarch64/0.2.0", "darwin/aarch64/0.10.0", "darwin/x86_64/0.1.0"} {
		if resp, got := checkForUpdate(t, srv, riya, path); resp.StatusCode != http.StatusNoContent {
			t.Errorf("%s: %d %+v, want nothing offered", path, resp.StatusCode, got)
		}
	}
}

func TestVersionsCompareByNumberNotByText(t *testing.T) {
	for _, c := range []struct {
		latest, current string
		want            bool
	}{
		{"0.10.0", "0.9.9", true},
		{"0.2.0", "0.2.0", false},
		{"1.0.0", "0.99.0", true},
		{"0.2.1", "v0.2.0", true},
		{"0.2.0-beta", "0.2.0", false},
	} {
		if got := newer(c.latest, c.current); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}
