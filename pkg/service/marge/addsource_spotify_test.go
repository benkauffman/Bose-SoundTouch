package marge

import (
	"os"
	"strconv"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestAddSource_MultipleSpotifyAccountsCoexist is a regression test for the
// multi-account Spotify eviction bug: AddSource treated SPOTIFY as a singleton
// per device, so registering a second linked Spotify account overwrote the
// first. The bridge and the Spotify watchdog re-add every linked account in
// turn, so whichever account was registered last "won" on each device and the
// other account's presets were skipped by /full ("not in configured sources")
// — a rotating subset of speakers showing 2/6 presets.
//
// Bose's own Marge kept one <source> per Spotify user, so two distinct
// accounts must coexist, while re-adding the same account updates in place.
func TestAddSource_MultipleSpotifyAccountsCoexist(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "addsource-spotify-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	account := "1234567"
	device := "000000000001"

	if mkErr := os.MkdirAll(ds.AccountDeviceDir(account, device), 0o755); mkErr != nil {
		t.Fatalf("mkdir device dir: %v", mkErr)
	}

	sp := strconv.Itoa(constants.SpotifyProviderID)

	const (
		userA = "spotify-user-a"
		userB = "spotify-user-b"
	)

	if _, err := AddSource(ds, account, userA, sp, "bs-aaaa", constants.CredentialTypeTokenV3, "Account A"); err != nil {
		t.Fatalf("add account A: %v", err)
	}

	if _, err := AddSource(ds, account, userB, sp, "bs-bbbb", constants.CredentialTypeTokenV3, "Account B"); err != nil {
		t.Fatalf("add account B: %v", err)
	}

	// spotifyAccounts returns account -> secret for the SPOTIFY sources the
	// datastore would serve via /full + /sources.
	spotifyAccounts := func() map[string]string {
		sources, gerr := ds.GetConfiguredSources(account, device)
		if gerr != nil {
			t.Fatalf("get sources: %v", gerr)
		}

		out := map[string]string{}

		for _, s := range sources {
			if s.SourceKey.Type == constants.ProviderSpotify {
				out[s.SourceKey.Account] = s.Secret
			}
		}

		return out
	}

	got := spotifyAccounts()
	if len(got) != 2 {
		t.Fatalf("expected 2 SPOTIFY sources, got %d: %+v", len(got), got)
	}

	if _, ok := got[userA]; !ok {
		t.Errorf("first Spotify account was evicted (account %q missing)", userA)
	}

	if _, ok := got[userB]; !ok {
		t.Errorf("second Spotify account not registered (account %q missing)", userB)
	}

	// Re-adding the SAME account (e.g. the watchdog re-registering after a
	// token refresh) updates in place; it must not duplicate or drop the other.
	if _, err := AddSource(ds, account, userA, sp, "bs-aaaa-2", constants.CredentialTypeTokenV3, "Account A"); err != nil {
		t.Fatalf("re-add account A: %v", err)
	}

	got = spotifyAccounts()
	if len(got) != 2 {
		t.Fatalf("re-adding the same account should keep exactly both accounts; got %+v", got)
	}

	if got[userA] != "bs-aaaa-2" {
		t.Errorf("re-add should update account A in place; secret = %q, want %q", got[userA], "bs-aaaa-2")
	}

	if got[userB] != "bs-bbbb" {
		t.Errorf("re-adding account A must not touch account B; secret = %q, want %q", got[userB], "bs-bbbb")
	}
}
