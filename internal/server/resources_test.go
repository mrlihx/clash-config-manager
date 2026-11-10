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

func TestBundledSubscriptionCacheAndClientMetadata(t *testing.T) {
	calls := 0
	fail := false
	rule := "+.first.test"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if fail {
			w.WriteHeader(502)
			return
		}
		w.Write([]byte("payload:\n  - " + rule + "\n"))
	}))
	defer upstream.Close()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "data/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "pass", "", root)
	source := "proxies: []\nrule-providers:\n  Test:\n    type: http\n    behavior: domain\n    format: yaml\n    interval: 60\n    url: " + upstream.URL + "\nrules: [\"RULE-SET,Test,DIRECT\",\"MATCH,DIRECT\"]\n"
	p := &store.Profile{ID: "test", Name: "测试订阅", Slug: "test", Enabled: true, Draft: source, Published: source, ClientUpdateHours: 6}
	if err = db.Save(p); err != nil {
		t.Fatal(err)
	}
	fetch := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/s/test", nil)
		rec := httptest.NewRecorder()
		s.Routes().ServeHTTP(rec, req)
		return rec
	}
	first := fetch()
	if first.Code != 200 || !strings.Contains(first.Body.String(), "+.first.test") || strings.Contains(first.Body.String(), upstream.URL) || !strings.Contains(first.Body.String(), "type: inline") {
		t.Fatal(first.Code, first.Body.String())
	}
	if first.Header().Get("Profile-Update-Interval") != "6" || !strings.Contains(first.Header().Get("Content-Disposition"), "filename*=utf-8''") {
		t.Fatal("client metadata missing", first.Header())
	}
	if next := fetch(); next.Code != 200 || calls != 1 {
		t.Fatal("cache was not reused", calls)
	}
	resources, _ := config.ExternalResources(source)
	path := s.resourcePath(resources[0])
	old := time.Now().Add(-2 * time.Minute)
	os.Chtimes(path, old, old)
	rule = "+.updated.test"
	s.refreshResources(context.Background())
	// 非订阅旧式配置由请求刷新；自动调度覆盖模板和正式订阅。
	updated := fetch()
	if !strings.Contains(updated.Body.String(), "+.updated.test") {
		t.Fatal("expired cache did not refresh", updated.Body.String())
	}
	os.Chtimes(path, old, old)
	fail = true
	cached := fetch()
	if cached.Code != 200 || !strings.Contains(cached.Body.String(), "+.updated.test") || cached.Header().Get("X-Resource-Sync") == "" {
		t.Fatal("last valid content not retained", cached.Code, cached.Body.String())
	}
	dbProfile, _ := db.Get(p.ID)
	if !strings.Contains(dbProfile.Draft, upstream.URL) {
		t.Fatal("editable source was overwritten")
	}
}

func TestMissingResourceReturnsErrorWithoutExternalReferences(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer upstream.Close()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "pass", "", root)
	source := "rule-providers: {Missing: {type: http, behavior: domain, url: '" + upstream.URL + "'}}\nrules: ['RULE-SET,Missing,DIRECT']\n"
	if err = db.Save(&store.Profile{ID: "x", Name: "X", Slug: "x", Enabled: true, Draft: source, Published: source}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/s/x", nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != 503 {
		t.Fatal("missing cache should not silently fall back to external URL", rec.Code)
	}
	var body map[string]any
	if err = json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec.Body.String(), upstream.URL) {
		t.Fatal("source URL leaked in error")
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
