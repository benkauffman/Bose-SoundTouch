package spotify

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestGetFreshTokenForSelection(t *testing.T) {
	svc := NewSpotifyService("cid", "csecret", "http://localhost/cb", t.TempDir())
	future := time.Now().Unix() + 3600
	svc.accounts["zzz-user"] = &Account{UserID: "zzz-user", Email: "ZZZ@example.com", AccessToken: "tok-zzz", ExpiresAt: future}
	svc.accounts["aaa-user"] = &Account{UserID: "aaa-user", Email: "aaa@example.com", AccessToken: "tok-aaa", ExpiresAt: future}

	t.Run("default is lowest user ID, not map order", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			tok, user, err := svc.GetFreshToken()
			if err != nil || user != "aaa-user" || tok != "tok-aaa" {
				t.Fatalf("iteration %d: got user=%q tok=%q err=%v", i, user, tok, err)
			}
		}
	})

	t.Run("primary by email wins", func(t *testing.T) {
		svc.SetPrimaryAccount("zzz@EXAMPLE.com")
		defer svc.SetPrimaryAccount("")

		_, user, err := svc.GetFreshToken()
		if err != nil || user != "zzz-user" {
			t.Fatalf("got user=%q err=%v", user, err)
		}
	})

	t.Run("unlinked primary falls back quietly", func(t *testing.T) {
		svc.SetPrimaryAccount("nobody@example.com")
		defer svc.SetPrimaryAccount("")

		_, user, err := svc.GetFreshToken()
		if err != nil || user != "aaa-user" {
			t.Fatalf("got user=%q err=%v", user, err)
		}
	})

	t.Run("explicit selector by ID, email, and unknown", func(t *testing.T) {
		if _, user, _ := svc.GetFreshTokenFor("zzz-user"); user != "zzz-user" {
			t.Fatalf("by id: got %q", user)
		}

		if _, user, _ := svc.GetFreshTokenFor("AAA@example.com"); user != "aaa-user" {
			t.Fatalf("by email: got %q", user)
		}

		if _, _, err := svc.GetFreshTokenFor("ghost"); err == nil {
			t.Fatal("unknown selector should error, not fall back")
		}
	})
}

func TestRefreshAllRefreshesEveryDueAccount(t *testing.T) {
	var refreshed []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		refreshed = append(refreshed, r.Form.Get("refresh_token"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-` + r.Form.Get("refresh_token") + `","expires_in":3600}`))
	}))
	defer ts.Close()

	svc := NewSpotifyService("cid", "csecret", "http://localhost/cb", t.TempDir())
	svc.SetEndpoints(ts.URL, "")
	now := time.Now().Unix()
	svc.accounts["stale"] = &Account{UserID: "stale", RefreshToken: "r-stale", AccessToken: "old", ExpiresAt: now - 10}
	svc.accounts["soon"] = &Account{UserID: "soon", RefreshToken: "r-soon", AccessToken: "old", ExpiresAt: now + 60}
	svc.accounts["fresh"] = &Account{UserID: "fresh", RefreshToken: "r-fresh", AccessToken: "keep", ExpiresAt: now + 7200}

	if err := svc.RefreshAll(15 * time.Minute); err != nil {
		t.Fatalf("RefreshAll: %v", err)
	}

	sort.Strings(refreshed)

	if len(refreshed) != 2 || refreshed[0] != "r-soon" || refreshed[1] != "r-stale" {
		t.Fatalf("refreshed %v, want [r-soon r-stale]", refreshed)
	}

	if svc.accounts["stale"].AccessToken != "new-r-stale" || svc.accounts["fresh"].AccessToken != "keep" {
		t.Fatalf("tokens after refresh: stale=%q fresh=%q", svc.accounts["stale"].AccessToken, svc.accounts["fresh"].AccessToken)
	}
}

func TestSpotifyScopesIncludeBrowseScopes(t *testing.T) {
	authURL := NewSpotifyService("cid", "csecret", "http://localhost/cb", t.TempDir()).BuildAuthorizeURL("")

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse authorize URL: %v", err)
	}

	granted := strings.Fields(parsed.Query().Get("scope"))

	for _, want := range []string{
		"streaming", "user-read-private", "user-read-email", "user-read-playback-state", "user-modify-playback-state",
		"playlist-read-private", "playlist-read-collaborative", "user-library-read", "user-read-recently-played", "user-top-read",
	} {
		if !slices.Contains(granted, want) {
			t.Errorf("authorize URL scope %v is missing %q", granted, want)
		}
	}
}

func TestGetFreshTokenWithScopeFor(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-` + r.Form.Get("refresh_token") + `","expires_in":3600,"scope":"streaming user-library-read"}`))
	}))
	defer ts.Close()

	svc := NewSpotifyService("cid", "csecret", "http://localhost/cb", t.TempDir())
	svc.SetEndpoints(ts.URL, "")
	now := time.Now().Unix()
	svc.accounts["house"] = &Account{UserID: "house", Email: "home@example.com", AccessToken: "tok-house", ExpiresAt: now + 3600, Scope: "streaming user-read-email"}
	svc.accounts["legacy"] = &Account{UserID: "legacy", Email: "legacy@example.com", AccessToken: "tok-legacy", ExpiresAt: now + 3600}
	svc.accounts["stale"] = &Account{UserID: "stale", Email: "stale@example.com", RefreshToken: "r-stale", AccessToken: "old", ExpiresAt: now - 10}

	t.Run("stored scope is returned", func(t *testing.T) {
		tok, user, scope, err := svc.GetFreshTokenWithScopeFor("HOME@example.com")
		if err != nil || user != "house" || tok != "tok-house" || scope != "streaming user-read-email" {
			t.Fatalf("got user=%q tok=%q scope=%q err=%v", user, tok, scope, err)
		}
	})

	t.Run("account without a recorded scope reports unknown", func(t *testing.T) {
		_, user, scope, err := svc.GetFreshTokenWithScopeFor("legacy")
		if err != nil || user != "legacy" || scope != "" {
			t.Fatalf("got user=%q scope=%q err=%v", user, scope, err)
		}
	})

	t.Run("refresh records the granted scope", func(t *testing.T) {
		tok, _, scope, err := svc.GetFreshTokenWithScopeFor("stale")
		if err != nil || tok != "new-r-stale" || scope != "streaming user-library-read" {
			t.Fatalf("got tok=%q scope=%q err=%v", tok, scope, err)
		}
	})

	t.Run("unknown selector is ErrAccountNotLinked", func(t *testing.T) {
		if _, _, _, err := svc.GetFreshTokenWithScopeFor("ghost@example.com"); !errors.Is(err, ErrAccountNotLinked) {
			t.Fatalf("err = %v, want ErrAccountNotLinked", err)
		}
	})

	t.Run("unknown selector with no accounts is ErrAccountNotLinked", func(t *testing.T) {
		empty := NewSpotifyService("cid", "csecret", "http://localhost/cb", t.TempDir())
		if _, _, _, err := empty.GetFreshTokenWithScopeFor("ghost"); !errors.Is(err, ErrAccountNotLinked) {
			t.Fatalf("err = %v, want ErrAccountNotLinked", err)
		}

		if _, _, _, err := empty.GetFreshTokenWithScopeFor(""); err == nil || errors.Is(err, ErrAccountNotLinked) {
			t.Fatalf("empty selector with no accounts: err = %v, want a non-ErrAccountNotLinked error", err)
		}
	})
}

func TestExchangeCodeAndStoreRecordsScope(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/me" {
			_, _ = w.Write([]byte(`{"id":"u1","display_name":"U","email":"u1@example.com"}`))
			return
		}

		_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","expires_in":3600,"scope":"streaming user-top-read"}`))
	}))
	defer ts.Close()

	dir := t.TempDir()
	svc := NewSpotifyService("cid", "csecret", "http://localhost/cb", dir)
	svc.SetEndpoints(ts.URL+"/token", ts.URL)

	if err := svc.ExchangeCodeAndStore("code"); err != nil {
		t.Fatalf("ExchangeCodeAndStore: %v", err)
	}

	reloaded := NewSpotifyService("cid", "csecret", "http://localhost/cb", dir)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := reloaded.accounts["u1"].Scope; got != "streaming user-top-read" {
		t.Fatalf("persisted scope = %q", got)
	}
}
