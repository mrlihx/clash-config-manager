package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clash-config-manager/internal/store"
)

func TestSourceRefreshAndDeletionKeepGeneratedSubscription(t *testing.T) {
	unavailable := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if unavailable {
			w.WriteHeader(502)
			return
		}
		_, _ = io.WriteString(w, "proxies:\n  - {name: refreshed, type: http, server: node.test, port: 8080}\nrules: ['MATCH,REJECT']\n")
	}))
	defer upstream.Close()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "test", "", t.TempDir())
	now := time.Date(2026, 11, 8, 6, 0, 0, 0, sourceTimezone)
	source := &store.Profile{ID: "source", Kind: "source", Name: "Source", Slug: "source", Revision: 1, Draft: "proxies: []\n", Original: "proxies: []\n", SourceURL: upstream.URL, UpdateMinutes: 15, NextUpdate: now.Add(-time.Minute)}
	subscription := &store.Profile{ID: "subscription", Kind: "subscription", Name: "Subscription", Slug: "subscription", Enabled: true, Revision: 1, Draft: "rules: ['MATCH,DIRECT']\n", Published: "rules: ['MATCH,DIRECT']\n"}
	if err = db.Save(source); err != nil {
		t.Fatal(err)
	}
	if err = db.Save(subscription); err != nil {
		t.Fatal(err)
	}
	s.refreshDueSources(context.Background(), now)
	updated, err := db.Get(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(updated.Draft, "refreshed") || strings.Contains(updated.Draft, "MATCH") || updated.UpdateError != "" {
		t.Fatal("source nodes did not refresh")
	}
	if !updated.NextUpdate.Equal(now.Add(15 * time.Minute)) {
		t.Fatal("next update was not scheduled")
	}
	unavailable = true
	s.refreshDueSources(context.Background(), updated.NextUpdate)
	failed, err := db.Get(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Draft != updated.Draft || failed.UpdateError == "" {
		t.Fatal("failed refresh changed source nodes or hid the error")
	}
	if err = db.DeleteSource(failed); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Get(source.ID); err == nil {
		t.Fatal("source was not removed")
	}
	published, err := db.Published(subscription.Slug)
	if err != nil || published != subscription.Published {
		t.Fatal("source changes affected generated subscription")
	}
}

func TestDailySourceUpdateUsesShanghaiTime(t *testing.T) {
	now := time.Date(2026, 11, 8, 23, 30, 0, 0, sourceTimezone)
	next, err := nextSourceUpdate(now, 0, "06:00")
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2026, 11, 9, 6, 0, 0, 0, sourceTimezone)
	if !next.Equal(expected) {
		t.Fatalf("got %v", next)
	}
	if _, err = nextSourceUpdate(now, 0, "25:00"); err == nil {
		t.Fatal("accepted invalid daily time")
	}
}

func TestSourceRefreshPreservesManualNodesAndOverrides(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "proxies: [{name: A, type: http, server: new.test, port: 8081}]")
	}))
	defer upstream.Close()
	db, err := store.Open(filepath.Join(t.TempDir(), "manual.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "test", "", t.TempDir())
	p := &store.Profile{ID: "source", Kind: "source", Name: "Source", Slug: "source", SourceURL: upstream.URL, Original: "proxies: [{name: A, type: http, server: old.test, port: 8080}]", Draft: "proxies: [{name: A, type: http, server: old.test, port: 9090}, {name: Local, type: http, server: local.test, port: 80}]"}
	if err = db.Save(p); err != nil {
		t.Fatal(err)
	}
	if err = s.refreshSource(context.Background(), p.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	updated, _ := db.Get(p.ID)
	if !strings.Contains(updated.Draft, "local.test") || !strings.Contains(updated.Draft, "9090") || !strings.Contains(updated.Draft, "new.test") || strings.Contains(updated.Original, "local.test") {
		t.Fatal("upstream refresh lost manual nodes or overrides")
	}
}
