package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clash-config-manager/internal/config"
	"clash-config-manager/internal/store"
	"go.yaml.in/yaml/v3"
)

// 旧同步测试只验证节点关系，预置规则缓存以避免依赖外部网络。
func seedResources(t *testing.T, s *Server, source string) {
	t.Helper()
	resources, err := config.ExternalResources(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range resources {
		rule := "+.example.test"
		if resource.Behavior == "ipcidr" {
			rule = "203.0.113.0/24"
		}
		if resource.Behavior == "classical" {
			rule = "DOMAIN-SUFFIX,example.test"
		}
		data, _ := yaml.Marshal(map[string]any{"payload": []string{rule}})
		path := s.resourcePath(resource)
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRuleCacheAndExternalSubscriptionMetadata(t *testing.T) {
	calls := 0
	failed := false
	rule := "+.first.test"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if failed {
			w.WriteHeader(502)
			return
		}
		w.Write([]byte("payload: ['" + rule + "']"))
	}))
	defer upstream.Close()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "data/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "pass", "", root)
	source := "proxies: []\nrule-providers: {Test: {type: http, behavior: domain, interval: 60, url: '" + upstream.URL + "'}}\nrules: ['RULE-SET,Test,DIRECT','MATCH,DIRECT']\n"
	p := &store.Profile{ID: "test", Name: "测试订阅", Slug: "test", Enabled: true, Draft: source, Published: source, ClientUpdateHours: 6}
	if err = db.Save(p); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	s.Routes().ServeHTTP(response, httptest.NewRequest("GET", "/s/test", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), upstream.URL) || calls != 0 {
		t.Fatal("subscription should retain external references without downloading", response.Code, calls)
	}
	if response.Header().Get("Profile-Update-Interval") != "6" || !strings.Contains(response.Header().Get("Content-Disposition"), "filename*=utf-8''") {
		t.Fatal("client metadata missing")
	}
	resources, _ := config.ExternalResources(source)
	resource := resources[0]
	first, err := s.loadResource(context.Background(), resource)
	if err != nil || !strings.Contains(string(first), "+.first.test") {
		t.Fatal("cache download failed", err)
	}
	if _, err = s.loadResource(context.Background(), resource); err != nil || calls != 1 {
		t.Fatal("cache was not reused", calls)
	}
	path := s.resourcePath(resource)
	old := time.Now().Add(-2 * time.Minute)
	os.Chtimes(path, old, old)
	rule = "+.updated.test"
	updated, err := s.loadResource(context.Background(), resource)
	if err != nil || !strings.Contains(string(updated), "+.updated.test") {
		t.Fatal("expired cache not refreshed", err)
	}
	os.Chtimes(path, old, old)
	failed = true
	cached, err := s.loadResource(context.Background(), resource)
	if err != nil || !strings.Contains(string(cached), "+.updated.test") {
		t.Fatal("last valid cache lost", err)
	}
	dbProfile, _ := db.Get(p.ID)
	if dbProfile.Draft != source {
		t.Fatal("editable source was modified")
	}
}

func TestExternalSubscriptionDoesNotRequirePlatformCache(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "pass", "", root)
	source := "rule-providers: {Missing: {type: http, behavior: domain, url: 'https://raw.githubusercontent.com/example/rules/main/rules.yaml'}}\nrules: ['RULE-SET,Missing,DIRECT']\n"
	if err = db.Save(&store.Profile{ID: "x", Name: "X", Slug: "x", Enabled: true, Draft: source, Published: source, ResourceProxyPrefix: "https://accelerator.example/"}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	s.Routes().ServeHTTP(response, httptest.NewRequest("GET", "/s/x", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "https://accelerator.example/https://raw.githubusercontent.com/") || strings.Contains(response.Body.String(), "type: inline") {
		t.Fatal("external acceleration not applied", response.Code, response.Body.String())
	}
	p, _ := db.Get("x")
	if p.Draft != source {
		t.Fatal("original URLs changed in saved config")
	}
}

func TestPlatformRefreshUsesShortestResourceInterval(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte("payload: ['+.scheduled.test']"))
	}))
	defer upstream.Close()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "pass", "", root)
	for i, interval := range []int{3600, 60} {
		source := fmt.Sprintf("rule-providers: {Shared: {type: http, behavior: domain, interval: %d, url: '%s'}}\nrules: ['RULE-SET,Shared,DIRECT']\n", interval, upstream.URL)
		if err = db.Save(&store.Profile{ID: fmt.Sprint(i), Name: fmt.Sprint(i), Slug: fmt.Sprint(i), Kind: "template", Draft: source}); err != nil {
			t.Fatal(err)
		}
	}
	s.refreshResources(context.Background())
	if calls != 1 {
		t.Fatal("shared resource downloaded more than once", calls)
	}
	p, _ := db.Get("0")
	resources, _ := config.ExternalResources(p.Draft)
	path := s.resourcePath(resources[0])
	old := time.Now().Add(-2 * time.Minute)
	os.Chtimes(path, old, old)
	s.refreshResources(context.Background())
	if calls != 2 {
		t.Fatal("shorter refresh interval was ignored", calls)
	}
}

func TestSubscriptionNameAndUpdateIntervalCanBeEdited(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "pass", "/usr/bin/true", root)
	source := "proxies: []\nrules: ['MATCH,DIRECT']\n"
	p := &store.Profile{ID: "sub", Kind: "subscription", Name: "Original", Slug: "fixed", Enabled: true, Revision: 1, Draft: source, Original: source, Published: source, Sync: `{"FollowRouting":false}`}
	if err = db.Save(p); err != nil {
		t.Fatal(err)
	}
	s.Store.SaveSession(s.sessionHash("test"), time.Now().Add(time.Hour))
	body, _ := json.Marshal(map[string]any{"action": "save-subscription", "revision": 1, "name": "新的订阅名称", "clientUpdateHours": 6, "yaml": source})
	req := httptest.NewRequest("POST", "/api/profiles/sub", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "test"})
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	current, err := db.Get("sub")
	if err != nil || current.Name != "新的订阅名称" || current.ClientUpdateHours != 6 || current.Slug != "fixed" {
		t.Fatal("edited subscription metadata not saved", err)
	}
	response := httptest.NewRecorder()
	s.Routes().ServeHTTP(response, httptest.NewRequest("GET", "/s/fixed", nil))
	_, params, err := mime.ParseMediaType(response.Header().Get("Content-Disposition"))
	if err != nil || params["filename"] != current.Name {
		t.Fatal("subscription name header did not change", response.Header())
	}
	if response.Header().Get("Profile-Update-Interval") != "6" {
		t.Fatal("update header did not change")
	}
}
