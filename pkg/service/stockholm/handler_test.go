package stockholm

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func newMountedHandler(t *testing.T, basePath string) http.Handler {
	t.Helper()

	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html><head></head><body>stockholm</body></html>"), 0644)

	state := NewNativeState(t.TempDir())
	cfg := &Config{BasePath: basePath}
	h := &Handler{
		cfg:          cfg,
		backendCfg:   &BackendConfig{},
		state:        state,
		bridge:       newBridge(cfg, state),
		stockholmDir: dir,
	}

	// Mirror the service router: CleanPath is what turns "/stockholm/" into
	// "/stockholm" for routing purposes.
	r := chi.NewRouter()
	r.Use(middleware.CleanPath)
	h.Mount(r)

	return r
}

// TestMount_BasePathTrailingSlash_ServesIndex is a regression test for the
// /stockholm/ redirect loop: with CleanPath in the middleware chain, a request
// for "/stockholm/" was routed to the bare-path handler, which redirected to
// "/stockholm/" again, forever. It must serve index.html instead.
func TestMount_BasePathTrailingSlash_ServesIndex(t *testing.T) {
	r := newMountedHandler(t, "/stockholm")

	req := httptest.NewRequest(http.MethodGet, "/stockholm/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /stockholm/: expected 200, got %d (Location=%q)", rec.Code, rec.Header().Get("Location"))
	}
}

// TestMount_BasePathBare_RedirectsToSlash keeps the intended behaviour: the
// bare base path redirects once so relative asset URLs resolve under it.
func TestMount_BasePathBare_RedirectsToSlash(t *testing.T) {
	r := newMountedHandler(t, "/stockholm")

	req := httptest.NewRequest(http.MethodGet, "/stockholm", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /stockholm: expected 301, got %d", rec.Code)
	}

	if got := rec.Header().Get("Location"); got != "/stockholm/" {
		t.Fatalf("GET /stockholm: expected Location /stockholm/, got %q", got)
	}
}
