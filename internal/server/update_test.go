package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"clash-config-manager/internal/store"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestExecutableUpdateChecksHashAndRequestsRestart(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux executable update is tested in Linux CI")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(binary)
	filename := "clash-config-manager_linux_" + runtime.GOARCH
	checksum := "wrong"
	var origin string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/mrlihx/clash-config-manager/releases/tags/build-new":
			json.NewEncoder(w).Encode(map[string]any{"tag_name": "build-new", "target_commitish": "new-commit", "assets": []map[string]string{{"name": filename, "browser_download_url": origin + "/binary"}, {"name": "SHA256SUMS", "browser_download_url": origin + "/checksums"}}})
		case "/checksums":
			w.Write([]byte(checksum + "  " + filename + "\n"))
		case "/binary":
			w.Write(binary)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	origin = upstream.URL
	target, _ := url.Parse(origin)
	oldRelease, oldDownload := releaseClient, downloadClient
	defer func() { releaseClient, downloadClient = oldRelease, oldDownload }()
	releaseClient = &http.Client{Transport: updateTransport(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		clone.URL.Scheme = target.Scheme
		clone.URL.Host = target.Host
		return http.DefaultTransport.RoundTrip(clone)
	})}
	downloadClient = upstream.Client()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "data/live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "admin123", "", root)
	s.Version = "old-commit"
	restarted := false
	s.Restart = func() { restarted = true }
	handler := s.Routes()
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"password":"admin123"}`))
	req.Header.Set("Content-Type", "application/json")
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, req)
	cookie := login.Result().Cookies()[0]
	apply := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/update", strings.NewReader(`{"tag":"build-new"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	checksum = strings.Repeat("0", 64)
	if rec := apply(); rec.Code != 422 || restarted {
		t.Fatal("incorrect checksum accepted", rec.Code)
	}
	installed := filepath.Join(root, "data/bin/clash-config-manager")
	if _, err = os.Stat(installed); !os.IsNotExist(err) {
		t.Fatal("failed update replaced program")
	}
	checksum = hex.EncodeToString(sum[:])
	if rec := apply(); rec.Code != 200 || !restarted {
		t.Fatal("update failed", rec.Body.String())
	}
	contents, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	got := sha256.Sum256(contents)
	if got != sum {
		t.Fatal("installed executable differs")
	}
	backups, err := filepath.Glob(filepath.Join(root, "data/backups/before-update-*.db"))
	if err != nil || len(backups) != 1 {
		t.Fatal("missing update backup")
	}
}
