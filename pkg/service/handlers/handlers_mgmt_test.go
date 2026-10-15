package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/spotify"
	"github.com/go-chi/chi/v5"
)

func TestHandleMgmtSpotifyInit(t *testing.T) {
	s := NewServer(nil, nil, "http://localhost", false, false, false)

	t.Run("POST - No spotify service configured", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/mgmt/spotify/init", nil)
		w := httptest.NewRecorder()
		s.HandleMgmtSpotifyInit(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("expected 503, got %d", w.Code)
		}
	})

	// With spotify service
	svc := spotify.NewSpotifyService("cid", "secret", "http://localhost/cb", t.TempDir())
	s.SetSpotifyService(svc)

	t.Run("POST - Success", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/mgmt/spotify/init", nil)
		w := httptest.NewRecorder()
		s.HandleMgmtSpotifyInit(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		var resp map[string]string
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(resp["redirectUrl"], "client_id=cid") {
			t.Errorf("expected redirectUrl to contain client_id=cid, got %s", resp["redirectUrl"])
		}
	})
}

func TestHandleMgmtSpotifyAccounts(t *testing.T) {
	s := NewServer(nil, nil, "http://localhost", false, false, false)
	svc := spotify.NewSpotifyService("cid", "secret", "http://localhost/cb", t.TempDir())
	s.SetSpotifyService(svc)

	req := httptest.NewRequest("GET", "/mgmt/spotify/accounts", nil)
	w := httptest.NewRecorder()
	s.HandleMgmtSpotifyAccounts(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string][]spotify.Account
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if len(resp["accounts"]) != 0 {
		t.Errorf("expected 0 accounts, got %d", len(resp["accounts"]))
	}
}

func TestHandleMgmtListSpeakers(t *testing.T) {
	tmpDir := t.TempDir()
	ds := datastore.NewDataStore(tmpDir)
	_, s := setupRouter("http://localhost:8000", ds)

	req := httptest.NewRequest("GET", "/mgmt/accounts/default/speakers", nil)
	w := httptest.NewRecorder()
	s.HandleMgmtListSpeakers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if _, ok := resp["speakers"]; !ok {
		t.Error("expected 'speakers' in response")
	}
}

func TestHandleMgmtSpotifyCallback(t *testing.T) {
	s := NewServer(nil, nil, "http://localhost", false, false, false)
	svc := spotify.NewSpotifyService("cid", "secret", "http://localhost/cb", t.TempDir())
	s.SetSpotifyService(svc)

	// Mock Spotify token and profile endpoints
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "at",
			"refresh_token": "rt",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	profileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           "user123",
			"display_name": "Test User",
		})
	}))
	defer profileServer.Close()

	// Use internal members to override URLs (available because we are in the same package)
	// Actually we need to reach through s.spotifyService which is private.
	// But s.spotifyService is *spotify.Service, which we have a handle to (svc).
	// We can't access private fields of spotify.Service from handlers package.
	// Wait, I can't override tokenURL from here if it's unexported in spotify package.
	// Let's check service.go again. Yes, tokenURL and apiBase are unexported.

	// Since I can't easily mock the external Spotify API here without exported fields,
	// I will test the error paths.

	t.Run("Missing code", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/mgmt/spotify/callback", nil)
		w := httptest.NewRecorder()
		s.HandleMgmtSpotifyCallback(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Missing authorization code") {
			t.Errorf("expected missing code error message, got %s", w.Body.String())
		}
	})

	t.Run("Spotify error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/mgmt/spotify/callback?error=access_denied", nil)
		w := httptest.NewRecorder()
		s.HandleMgmtSpotifyCallback(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "access_denied") {
			t.Errorf("expected access_denied error message, got %s", w.Body.String())
		}
	})
}

func TestHandleMgmtSpotifyConfirm(t *testing.T) {
	s := NewServer(nil, nil, "http://localhost", false, false, false)
	svc := spotify.NewSpotifyService("cid", "secret", "http://localhost/cb", t.TempDir())
	s.SetSpotifyService(svc)

	t.Run("Missing code", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/mgmt/spotify/confirm", nil)
		w := httptest.NewRecorder()
		s.HandleMgmtSpotifyConfirm(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})
}

func TestHandleMgmtDeviceEvents(t *testing.T) {
	tmpDir := t.TempDir()
	ds := datastore.NewDataStore(tmpDir)
	_, s := setupRouter("http://localhost:8000", ds)

	r := chi.NewRouter()
	r.Get("/mgmt/devices/{deviceId}/events", s.HandleMgmtDeviceEvents)

	req := httptest.NewRequest("GET", "/mgmt/devices/device123/events", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if _, ok := resp["events"]; !ok {
		t.Error("expected 'events' in response")
	}
}

func TestBasicAuthMgmt(t *testing.T) {
	s := NewServer(nil, nil, "http://localhost", false, false, false)
	s.SetMgmtConfig("admin", "secret123")

	handler := s.BasicAuthMgmt()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))

	t.Run("Valid credentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/mgmt/test", nil)
		req.SetBasicAuth("admin", "secret123")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		if rr.Body.String() != "OK" {
			t.Errorf("expected body 'OK', got %q", rr.Body.String())
		}
	})

	t.Run("Wrong username", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/mgmt/test", nil)
		req.SetBasicAuth("wrong", "secret123")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
		if rr.Header().Get("WWW-Authenticate") == "" {
			t.Error("expected WWW-Authenticate header to be set")
		}
	})

	t.Run("Wrong password", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/mgmt/test", nil)
		req.SetBasicAuth("admin", "wrongpass")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})

	t.Run("Missing auth header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/mgmt/test", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})

	t.Run("Empty credentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/mgmt/test", nil)
		req.SetBasicAuth("", "")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})
}

// TestBasicAuthAdmin covers the #419 admin-area gate: unlike BasicAuthMgmt
// (credentials captured once at router-setup time), BasicAuthAdmin must
// read the live AdminAreaAuth mode and credentials on every request, so a
// live toggle via the Settings UI takes effect without a restart.
func TestBasicAuthAdmin(t *testing.T) {
	s := NewServer(nil, nil, "http://localhost", false, false, false)
	s.SetMgmtConfig("admin", "secret123")

	handler := s.BasicAuthAdmin()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))

	t.Run("Unset mode passes through unauthenticated (today's default)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d for unset mode, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("Disabled mode passes through unauthenticated", func(t *testing.T) {
		s.SetAdminAreaAuth("disabled")
		defer s.SetAdminAreaAuth("")

		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d for disabled mode, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("Enabled mode requires valid credentials", func(t *testing.T) {
		s.SetAdminAreaAuth("enabled")
		defer s.SetAdminAreaAuth("")

		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		req.SetBasicAuth("admin", "secret123")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d with valid credentials, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("Enabled mode rejects missing credentials", func(t *testing.T) {
		s.SetAdminAreaAuth("enabled")
		defer s.SetAdminAreaAuth("")

		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d with no credentials, got %d", http.StatusUnauthorized, rr.Code)
		}
		if rr.Header().Get("WWW-Authenticate") == "" {
			t.Error("expected WWW-Authenticate header to be set")
		}
	})

	t.Run("Enabled mode rejects wrong credentials", func(t *testing.T) {
		s.SetAdminAreaAuth("enabled")
		defer s.SetAdminAreaAuth("")

		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		req.SetBasicAuth("admin", "wrongpass")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d with wrong credentials, got %d", http.StatusUnauthorized, rr.Code)
		}
	})

	t.Run("Toggling mode live changes behavior without rebuilding the handler", func(t *testing.T) {
		// The whole point of reading s.adminAreaAuth per-request rather than
		// capturing it once: the same handler value must reflect a live change.
		s.SetAdminAreaAuth("")
		reqOpen := httptest.NewRequest(http.MethodGet, "/admin", nil)
		rrOpen := httptest.NewRecorder()
		handler.ServeHTTP(rrOpen, reqOpen)

		if rrOpen.Code != http.StatusOK {
			t.Fatalf("expected open access before toggling, got %d", rrOpen.Code)
		}

		s.SetAdminAreaAuth("enabled")
		defer s.SetAdminAreaAuth("")

		reqGated := httptest.NewRequest(http.MethodGet, "/admin", nil)
		rrGated := httptest.NewRecorder()
		handler.ServeHTTP(rrGated, reqGated)

		if rrGated.Code != http.StatusUnauthorized {
			t.Errorf("expected the SAME handler to enforce auth immediately after toggling, got %d", rrGated.Code)
		}
	})
}

func TestHandleMgmtSpotifyToken(t *testing.T) {
	s := NewServer(nil, nil, "http://localhost", false, false, false)

	t.Run("no spotify service configured", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleMgmtSpotifyToken(w, httptest.NewRequest("GET", "/mgmt/spotify/token", nil))

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("expected 503, got %d", w.Code)
		}
	})

	dir := t.TempDir()
	future := time.Now().Add(time.Hour).Unix()
	accounts := map[string]map[string]interface{}{
		"house-id": {"user_id": "house-id", "email": "home@example.com", "access_token": "tok-house", "refresh_token": "r", "expires_at": future,
			"scope": "streaming user-library-read"},
		"ben-id": {"user_id": "ben-id", "email": "ben@example.com", "access_token": "tok-ben", "refresh_token": "r", "expires_at": future},
	}

	data, err := json.Marshal(accounts)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "spotify"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "spotify", "accounts.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	svc := spotify.NewSpotifyService("cid", "secret", "http://localhost/cb", dir)
	if err := svc.Load(); err != nil {
		t.Fatal(err)
	}

	svc.SetPrimaryAccount("home@example.com")
	s.SetSpotifyService(svc)

	get := func(t *testing.T, query string) (int, map[string]string) {
		t.Helper()

		w := httptest.NewRecorder()
		s.HandleMgmtSpotifyToken(w, httptest.NewRequest("GET", "/mgmt/spotify/token"+query, nil))

		var resp map[string]string
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode %q response: %v", query, err)
		}

		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}

		return w.Code, resp
	}

	cases := []struct {
		name, query, wantUser, wantToken, wantScopes string
	}{
		{"default is the primary", "", "house-id", "tok-house", "streaming user-library-read"},
		{"by user id", "?account=ben-id", "ben-id", "tok-ben", ""},
		{"by email, case-insensitive", "?account=BEN@example.com", "ben-id", "tok-ben", ""},
		{"primary by email", "?account=home@example.com", "house-id", "tok-house", "streaming user-library-read"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, resp := get(t, tc.query)
			if code != http.StatusOK || resp["username"] != tc.wantUser || resp["access_token"] != tc.wantToken {
				t.Fatalf("got %d user=%q token=%q", code, resp["username"], resp["access_token"])
			}

			scopes, present := resp["scopes"]
			if tc.wantScopes == "" && present {
				t.Errorf("scopes should be omitted when unknown, got %q", scopes)
			}

			if tc.wantScopes != "" && scopes != tc.wantScopes {
				t.Errorf("scopes = %q, want %q", scopes, tc.wantScopes)
			}
		})
	}

	t.Run("unknown account is a 404 without any token", func(t *testing.T) {
		code, resp := get(t, "?account=ghost@example.com")
		if code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", code)
		}

		if resp["error"] == "" || resp["access_token"] != "" || resp["username"] != "" {
			t.Fatalf("unexpected 404 body: %v", resp)
		}
	})
}
