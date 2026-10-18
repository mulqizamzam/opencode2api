package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A browser renders the console only when the document is served as HTML.
// mime.TypeByExtension(".html") is registry-driven on Windows and this machine
// maps it to "video/html", so http.FileServer sent a type no browser renders:
// opening the WebUI address downloaded the page instead of showing it.
func TestStaticHandlerServesConsoleAsHTML(t *testing.T) {
	server, _ := newTestAdmin(t, testRuntimeConfig())

	// The address an operator opens. It must arrive as a document, not a file.
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("GET / Content-Type = %q, want text/html", got)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "<!doctype html>") {
		t.Fatalf("GET / did not return the console document")
	}

	// /index.html is redirected to ./ by http.FileServer; a client follows it
	// to the document asserted above.
	redirect := httptest.NewRecorder()
	server.Handler().ServeHTTP(redirect, httptest.NewRequest(http.MethodGet, "/index.html", nil))
	if redirect.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /index.html = %d, want 301", redirect.Code)
	}

	// The document fix must not disturb an asset that already has a real type.
	asset := httptest.NewRecorder()
	server.Handler().ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/css/tokens.css", nil))
	if got := asset.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
		t.Fatalf("GET /css/tokens.css Content-Type = %q, want text/css", got)
	}
}
