package libgen

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestFindWorkingMirrorReturnsFirstAvailable(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer bad.Close()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer good.Close()

	badURL, err := url.Parse(bad.URL)
	if err != nil {
		t.Fatalf("parse bad url: %v", err)
	}
	goodURL, err := url.Parse(good.URL)
	if err != nil {
		t.Fatalf("parse good url: %v", err)
	}

	mirror, err := FindWorkingMirror([]url.URL{*badURL, *goodURL})
	if err != nil {
		t.Fatalf("FindWorkingMirror returned unexpected error: %v", err)
	}
	if mirror.String() != good.URL {
		t.Fatalf("got mirror %q, want %q", mirror.String(), good.URL)
	}
}

func TestFindWorkingMirrorReturnsErrorWhenAllUnavailable(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusServiceUnavailable)
	}))
	defer down.Close()

	downURL, err := url.Parse(down.URL)
	if err != nil {
		t.Fatalf("parse down url: %v", err)
	}

	_, err = FindWorkingMirror([]url.URL{*downURL})
	if err == nil {
		t.Fatal("expected error when all mirrors are unavailable")
	}
	if !strings.Contains(err.Error(), down.URL) {
		t.Fatalf("error %q does not include mirror url %q", err.Error(), down.URL)
	}
}
