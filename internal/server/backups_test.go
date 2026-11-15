package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clash-config-manager/internal/store"
)

func TestBackupManagementRestoreAndValidation(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "data/clash.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	initial := &store.Profile{ID: "kept", Name: "Original", Slug: "original", Draft: "proxies: []\n", Revision: 1}
	if err = db.Save(initial); err != nil {
		t.Fatal(err)
	}
	s := New(db, "admin123", "", root)
	handler := s.Routes()
	login := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"password":"admin123"}`))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(login, req)
	cookie := login.Result().Cookies()[0]
	request := func(method, path, body, contentType string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(cookie)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	create := request("POST", "/api/backups", "{}", "application/json")
	if create.Code != 200 {
		t.Fatal(create.Body.String())
	}
	var created map[string]string
	json.Unmarshal(create.Body.Bytes(), &created)
	download := request("GET", "/api/backups/"+created["id"]+"/download", "", "")
	if download.Code != 200 {
		t.Fatal(download.Body.String())
	}
	bad := request("POST", "/api/backups/upload", "not a database", "application/octet-stream")
	if bad.Code != 422 {
		t.Fatal("accepted invalid upload", bad.Code)
	}
	upload := request("POST", "/api/backups/upload", download.Body.String(), "application/octet-stream")
	if upload.Code != 200 {
		t.Fatal(upload.Body.String())
	}
	var uploaded map[string]string
	json.Unmarshal(upload.Body.Bytes(), &uploaded)
	list := request("GET", "/api/backups", "", "")
	var items []struct{ ID, Source string }
	if err = json.Unmarshal(list.Body.Bytes(), &items); err != nil || len(items) != 2 {
		t.Fatal("wrong backup list", list.Body.String())
	}
	initial.Name = "Changed"
	if err = db.Save(initial); err != nil {
		t.Fatal(err)
	}
	if err = db.Save(&store.Profile{ID: "later", Name: "Later", Slug: "later", Draft: "proxies: []\n"}); err != nil {
		t.Fatal(err)
	}
	restore := request("POST", "/api/backups/"+uploaded["id"]+"/restore", "{}", "application/json")
	if restore.Code != 200 {
		t.Fatal(restore.Body.String())
	}
	var safety map[string]string
	json.Unmarshal(restore.Body.Bytes(), &safety)
	if _, err = os.Stat(filepath.Join(root, "data/backups", safety["backup"])); err != nil {
		t.Fatal("missing safety backup", err)
	}
	restored, err := db.Get("kept")
	if err != nil || restored.Name != "Original" {
		t.Fatal("restore did not replace data", err)
	}
	if _, err = db.Get("later"); err == nil {
		t.Fatal("later record survived full restore")
	}
	if r := request("GET", "/api/backups", "", ""); r.Code != 401 {
		t.Fatal("old session survived restore")
	}
	req = httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"password":"admin123"}`))
	req.Header.Set("Content-Type", "application/json")
	login = httptest.NewRecorder()
	handler.ServeHTTP(login, req)
	cookie = login.Result().Cookies()[0]
	if r := request("DELETE", "/api/backups/"+uploaded["id"], "{}", "application/json"); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if _, err = s.backupPath("../clash.db"); err == nil {
		t.Fatal("path traversal accepted")
	}
}
