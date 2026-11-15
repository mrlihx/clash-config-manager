package server

import (
	"clash-config-manager/internal/store"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoginSurvivesRestartAndLogoutRevokes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, "admin-password", "", t.TempDir())
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"password":"admin-password"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("login: %d", rec.Code)
	}
	cookie := rec.Result().Cookies()[0]
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s = New(db, "admin-password", "", t.TempDir())
	check := httptest.NewRequest("GET", "/api/session", nil)
	check.AddCookie(cookie)
	if !s.signedIn(check) {
		t.Fatal("session lost after database reopen")
	}
	if New(db, "changed-password", "", t.TempDir()).signedIn(check) {
		t.Fatal("old session accepted after password change")
	}
	logout := httptest.NewRequest("POST", "/api/logout", strings.NewReader(`{}`))
	logout.Header.Set("Content-Type", "application/json")
	logout.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, logout)
	if rec.Code != http.StatusOK || s.signedIn(check) {
		t.Fatal("logout failed to revoke session")
	}
}
