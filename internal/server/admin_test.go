package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clash-config-manager/internal/store"
	"go.yaml.in/yaml/v3"
)

func TestAdminPasswordAndOnlineBackup(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "custom.yaml")
	if err := os.WriteFile(configPath, []byte("listen: 127.0.0.1:8787\ndatabase: data/clash.db\nmihomo: /usr/bin/true\nadmin-password: admin123\n"), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(root, "data/clash.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Save(&store.Profile{ID: "fixture", Name: "Fixture", Slug: "fixture", Draft: "fixture-data"}); err != nil {
		t.Fatal(err)
	}
	s := New(db, "admin123", "", root)
	s.ConfigPath = configPath
	handler := s.Routes()
	request := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	if r := request("GET", "/api/backup", "", nil); r.Code != 401 {
		t.Fatal("unauthenticated backup accepted")
	}
	login := request("POST", "/api/login", `{"password":"admin123"}`, nil)
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	backup := request("GET", "/api/backup", "", cookie)
	if backup.Code != 200 || !strings.HasPrefix(backup.Body.String(), "SQLite format 3") || !strings.Contains(backup.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("invalid database download")
	}
	backupPath := filepath.Join(t.TempDir(), "restored.db")
	if err = os.WriteFile(backupPath, backup.Body.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Open(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if profile, err := restored.Get("fixture"); err != nil || profile.Draft != "fixture-data" {
		t.Fatal("backup lost stored data", err)
	}
	for _, body := range []string{`{"currentPassword":"wrong","newPassword":"new-password"}`, `{"currentPassword":"admin123","newPassword":"1234"}`} {
		if r := request("POST", "/api/password", body, cookie); r.Code != 400 {
			t.Fatal("invalid password change accepted")
		}
	}
	if r := request("POST", "/api/password", `{"currentPassword":"admin123","newPassword":"new: password"}`, cookie); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if r := request("GET", "/api/backup", "", cookie); r.Code != 401 {
		t.Fatal("old login remains active")
	}
	if r := request("POST", "/api/login", `{"password":"admin123"}`, nil); r.Code != 401 {
		t.Fatal("old password accepted")
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]string
	if err = yaml.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["admin-password"] != "new: password" || settings["database"] != "data/clash.db" {
		t.Fatal("config not persisted correctly")
	}
	// 模拟重启，使用写入配置的密码构造新服务。
	handler = New(db, settings["admin-password"], "", root).Routes()
	if r := request("POST", "/api/login", `{"password":"new: password"}`, nil); r.Code != 200 {
		t.Fatal("password lost after restart")
	}
}
