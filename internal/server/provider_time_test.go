package server

import (
	"clash-config-manager/internal/store"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuleProviderTimeUsesActualCache(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "data", "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "password", "", root)
	if err = os.MkdirAll(filepath.Join(root, "data", "rules"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "data", "rules", fmt.Sprintf("%x", md5.Sum([]byte("https://example.test/rules"))))
	if err = os.WriteFile(path, []byte("payload: []"), 0600); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1700000000, 0)
	if err = os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	p := &store.Profile{Draft: "rule-providers:\n  Cached: {type: http, behavior: domain, url: 'https://example.test/rules', path: ./data/rules/Cached.yaml}\n  Missing: {type: http, behavior: domain, url: 'https://example.test/missing', path: ./data/rules/Missing.yaml}\n"}
	w := httptest.NewRecorder()
	s.profileReply(w, p)
	var result struct {
		RuleProviderUpdated map[string]int64 `json:"ruleProviderUpdated"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.RuleProviderUpdated["Cached"] != stamp.Unix() || result.RuleProviderUpdated["Missing"] != 0 {
		t.Fatal("cache time was fabricated or missing")
	}
}
