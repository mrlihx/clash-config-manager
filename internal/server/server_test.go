package server

import (
	"clash-config-manager/internal/store"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubscriptionOnlyServesPublishedSnapshot(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "test", "", t.TempDir())
	p := &store.Profile{ID: "id", Name: "Home", Slug: "home", Enabled: true, Draft: "draft", Published: "marker: published\n"}
	if err := s.Store.Save(p); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		url    string
		status int
		body   string
	}{
		{"/s/home", 200, "published"},
		{"/sub/home", 200, "published"},
		{"/sub/home.yaml", 200, "published"},
		{"/sub/home.yaml?token=legacy", 200, "published"},
		{"/sub/missing.yaml", 404, ""},
	} {
		w := httptest.NewRecorder()
		s.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.url, nil))
		if tc.status == 200 && (!strings.HasPrefix(w.Header().Get("Content-Disposition"), "inline;") || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain")) {
			t.Fatal("subscription must render as plain text")
		}
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.body) {
			t.Fatalf("%s got %d %q", tc.url, w.Code, w.Body)
		}
	}
	p.Enabled = false
	if err := s.Store.Save(p); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sub/home.yaml", nil))
	if w.Code != 404 {
		t.Fatalf("disabled subscription returned %d", w.Code)
	}
}

func TestInvalidPublishKeepsPreviousVersion(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "test", "", t.TempDir())
	p := &store.Profile{ID: "profile", Name: "Home", Slug: "home", Enabled: true, Revision: 1,
		Draft: "rules: [\"MATCH,Missing\"]\n", Published: "rules: [\"MATCH,DIRECT\"]\n"}
	if err = s.Store.Save(p); err != nil {
		t.Fatal(err)
	}
	router := s.Routes()
	w := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/api/profiles/profile", strings.NewReader(`{"action":"publish","revision":1}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session"})
	s.Store.SaveSession(s.sessionHash("session"), time.Now().Add(time.Hour))
	router.ServeHTTP(w, request)
	if w.Code != 422 {
		t.Fatalf("invalid publish returned %d", w.Code)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/sub/home.yaml", nil))
	if w.Code != 200 || w.Body.String() != p.Published {
		t.Fatal("published snapshot changed after failed validation")
	}
}

func TestInvalidSubscriptionEditPreservesPublishedContent(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "test", "", t.TempDir())
	original := "rules: [\"MATCH,DIRECT\"]\n"
	p := &store.Profile{Kind: "subscription", ID: "subscription", Name: "Home", Slug: "home", Enabled: true, Revision: 1, Draft: original, Published: original}
	if err = s.Store.Save(p); err != nil {
		t.Fatal(err)
	}
	s.Store.SaveSession(s.sessionHash("session"), time.Now().Add(time.Hour))
	request := httptest.NewRequest("POST", "/api/profiles/subscription", strings.NewReader(`{"action":"save-subscription","revision":1,"yaml":"rules: [\"MATCH,Missing\"]\n"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session"})
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, request)
	if w.Code != 422 {
		t.Fatalf("invalid edit returned %d", w.Code)
	}
	current, err := s.Store.Get(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Published != original || current.Draft != original || current.Revision != 1 {
		t.Fatal("failed edit changed the stored subscription")
	}
}

func TestDeleteSubscriptionInvalidatesLinkAndKeepsSource(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "data", "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "test", "", t.TempDir())
	for _, p := range []*store.Profile{{ID: "source", Kind: "source", Name: "Source", Slug: "source", Revision: 1, Draft: "proxies: []\n"}, {ID: "subscription", Kind: "subscription", Name: "Subscription", Slug: "subscription", Revision: 1, Enabled: true, Published: "rules: [MATCH,DIRECT]"}} {
		if err = db.Save(p); err != nil {
			t.Fatal(err)
		}
	}
	s.Store.SaveSession(s.sessionHash("session"), time.Now().Add(time.Hour))
	request := httptest.NewRequest("DELETE", "/api/subscriptions/subscription", strings.NewReader(`{"revision":1}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session"})
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, request)
	if w.Code != 200 {
		t.Fatalf("delete returned %d", w.Code)
	}
	w = httptest.NewRecorder()
	s.Routes().ServeHTTP(w, httptest.NewRequest("GET", "/sub/subscription.yaml", nil))
	if w.Code != 404 {
		t.Fatal("deleted subscription is still accessible")
	}
	if _, err = db.Get("source"); err != nil {
		t.Fatal("source was deleted with subscription")
	}
}
