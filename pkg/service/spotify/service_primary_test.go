package spotify

import (
	"net/http"
	"net/http/httptest"
	"sort"
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
