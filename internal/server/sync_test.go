package server

import (
	"clash-config-manager/internal/config"
	"clash-config-manager/internal/store"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSourceUpdatesPublishedNodesAndClientFormats(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("proxies: [{name: A, type: vless, server: new.test, port: 443, uuid: test-id, tls: true}, {name: B, type: vless, server: b.test, port: 443, uuid: b-id, tls: true}]"))
	}))
	defer upstream.Close()
	db, err := store.Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "password", "", t.TempDir())
	src := &store.Profile{ID: "source", Kind: "source", Name: "Source", Slug: "source", SourceURL: upstream.URL, Draft: "proxies: [{name: A, type: vless, server: old.test, port: 443, uuid: test-id, tls: true}]"}
	baseline, err := config.CollectNodes([]config.NodeSelection{{Name: src.Name, Source: src.Draft, Nodes: []string{"A"}}})
	if err != nil {
		t.Fatal(err)
	}
	state, _ := json.Marshal(subscriptionSync{Sources: []sourceSelection{{ID: src.ID, Name: src.Name, Nodes: []string{"A"}, All: true}}, Baseline: baseline})
	sub := &store.Profile{ID: "sub", Kind: "subscription", Name: "Sub", Slug: "stable", Enabled: true, Original: baseline, Draft: baseline, Published: baseline, Sync: string(state)}
	if err = db.Save(src); err != nil {
		t.Fatal(err)
	}
	if err = db.Save(sub); err != nil {
		t.Fatal(err)
	}
	if err = s.refreshSource(context.Background(), src.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	updated, err := db.Get(sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(updated.Published, "new.test") || !strings.Contains(updated.Published, "b.test") || updated.Slug != "stable" {
		t.Fatal("published subscription did not sync")
	}
	seedResources(t, s, updated.Published)
	for _, agent := range []string{"v2rayNG/1", "Clash.Meta", "unknown"} {
		r := httptest.NewRequest("GET", "/s/stable", nil)
		r.Header.Set("User-Agent", agent)
		w := httptest.NewRecorder()
		s.Routes().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if w.Header().Get("Vary") != "User-Agent" {
			t.Fatal("missing vary")
		}
		if strings.HasPrefix(agent, "v2rayNG") {
			b, e := base64.StdEncoding.DecodeString(w.Body.String())
			if e != nil || !strings.Contains(string(b), "vless://") {
				t.Fatal("not link subscription")
			}
		} else if !strings.Contains(w.Body.String(), "proxies:") {
			t.Fatal("not clash YAML")
		}
	}
	if err = db.DeleteSource(src); err != nil {
		t.Fatal(err)
	}
	s.Store.Lock()
	err = s.syncSubscriptions("")
	s.Store.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	preserved, _ := db.Get(sub.ID)
	if preserved.Published != updated.Published {
		t.Fatal("source deletion changed subscription")
	}
}

func TestLegacySubscriptionBindsOnlyUniqueSource(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "test", "", t.TempDir())
	src := &store.Profile{ID: "source", Kind: "source", Name: "Source", Slug: "source", Draft: "proxies: [{name: A, type: http, server: a.test, port: 80}]"}
	baseline, err := config.CollectNodes([]config.NodeSelection{{Name: src.Name, Source: src.Draft, Nodes: []string{"A"}}})
	if err != nil {
		t.Fatal(err)
	}
	sub := &store.Profile{ID: "sub", Kind: "subscription", Name: "Sub", Slug: "sub", Original: baseline, Draft: baseline, Published: baseline}
	if err = db.Save(src); err != nil {
		t.Fatal(err)
	}
	if err = db.Save(sub); err != nil {
		t.Fatal(err)
	}
	if err = s.bindLegacySubscription(sub, []*store.Profile{src, src}); err != nil {
		t.Fatal(err)
	}
	if sub.Sync != "" {
		t.Fatal("ambiguous source was bound")
	}
	if err = s.bindLegacySubscription(sub, []*store.Profile{src}); err != nil {
		t.Fatal(err)
	}
	if sub.Sync == "" {
		t.Fatal("unique source was not bound")
	}
}
