package server

import (
	"clash-config-manager/internal/config"
	"clash-config-manager/internal/store"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubscriptionNodeSelectionSurvivesSourceSync(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "password", "", t.TempDir())
	src := &store.Profile{ID: "src", Kind: "source", Name: "Source", Slug: "source", Draft: "proxies: [{name: A, type: http, server: a.test, port: 80}, {name: B, type: http, server: b.test, port: 80}]"}
	if err = db.Save(src); err != nil {
		t.Fatal(err)
	}
	before, err := config.CollectNodes([]config.NodeSelection{{Name: "Source", Source: src.Draft, Nodes: []string{"A", "B"}}})
	if err != nil {
		t.Fatal(err)
	}
	state, _ := json.Marshal(subscriptionSync{Sources: []sourceSelection{{ID: "src", Name: "Source", Nodes: []string{"A", "B"}, All: true}}, Baseline: before})
	p := &store.Profile{ID: "sub", Kind: "subscription", Slug: "sub", Draft: before, Original: before, Published: before, Sync: string(state)}
	next, selection, err := s.selectSubscriptionNodes(p, before, []sourceSelection{{ID: "src", Nodes: []string{"B"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := config.Validate(next)
	if err != nil {
		t.Fatal(err)
	}
	if nodes := data["proxies"].([]any); len(nodes) != 1 || nodes[0].(map[string]any)["name"] != "B" {
		t.Fatal("incorrect node selection")
	}
	p.Draft = next
	p.Published = next
	state, _ = json.Marshal(selection)
	p.Sync = string(state)
	if err = db.Save(p); err != nil {
		t.Fatal(err)
	}
	src.Draft = strings.Replace(src.Draft, "b.test", "updated.test", 1)
	if err = db.Save(src); err != nil {
		t.Fatal(err)
	}
	db.Lock()
	err = s.syncSubscriptions("src")
	db.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	p, err = db.Get("sub")
	if err != nil {
		t.Fatal(err)
	}
	data, err = config.Validate(p.Draft)
	if err != nil {
		t.Fatal(err)
	}
	nodes := data["proxies"].([]any)
	if len(nodes) != 1 || nodes[0].(map[string]any)["server"] != "updated.test" {
		t.Fatal("source sync restored deselected nodes or lost update")
	}
}

func TestSubscriptionKeepsLocalNodesWithoutSource(t *testing.T) {
	s := New(nil, "password", "", t.TempDir())
	source := "proxies: [{name: Local, type: http, server: local.test, port: 80}]"
	p := &store.Profile{Kind: "subscription", Draft: source}
	next, state, err := s.selectSubscriptionNodes(p, source, nil, []string{"Local"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(next, "local.test") || len(state.Sources) != 0 {
		t.Fatal("lost local node")
	}
}

func TestEditingSubscriptionKeepsSourceAndNodeOrder(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "order.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "password", "", t.TempDir())
	first := &store.Profile{ID: "first", Kind: "source", Name: "First", Slug: "first", Draft: "proxies: [{name: A, type: http, server: a.test, port: 80}, {name: B, type: http, server: b.test, port: 80}]"}
	second := &store.Profile{ID: "second", Kind: "source", Name: "Second", Slug: "second", Draft: "proxies: [{name: C, type: http, server: c.test, port: 80}]"}
	for _, src := range []*store.Profile{first, second} {
		if err = db.Save(src); err != nil {
			t.Fatal(err)
		}
	}
	original, err := config.CollectNodes([]config.NodeSelection{{Name: "Second", Source: second.Draft, Nodes: []string{"C"}}, {Name: "First", Source: first.Draft, Nodes: []string{"A", "B"}}})
	if err != nil {
		t.Fatal(err)
	}
	state, _ := json.Marshal(subscriptionSync{Sources: []sourceSelection{{ID: "second"}, {ID: "first"}}, Baseline: original})
	p := &store.Profile{Kind: "subscription", Draft: original, Sync: string(state)}
	next, _, err := s.selectSubscriptionNodes(p, original, []sourceSelection{{ID: "first", Nodes: []string{"B", "A"}}, {ID: "second", Nodes: []string{"C"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := config.Validate(next)
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"C", "A", "B"} {
		if data["proxies"].([]any)[i].(map[string]any)["name"] != name {
			t.Fatal("edit reordered sources or nodes")
		}
	}
}

func TestClientRefreshPrunesUntrackedHistoricalNodes(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "password", "", root)
	source := "proxies: [{name: Active, type: http, server: fresh.test, port: 80}]"
	if err = db.Save(&store.Profile{ID: "src", Kind: "source", Name: "Source", Slug: "src", Draft: source}); err != nil {
		t.Fatal(err)
	}
	current := `proxies:
 - {name: Active, type: http, server: obsolete.test, port: 80}
 - {name: Old, type: http, server: old.test, port: 80}
proxy-groups:
 - {name: Choice, type: select, proxies: [Old,Active]}
rules: ['MATCH,Choice']
`
	// 旧版已有路由信息，但没有保存来源关系，仍应自动恢复有效关联。
	legacy, _ := json.Marshal(subscriptionSync{RoutingPlan: "system-routing-template"})
	p := &store.Profile{ID: "sub", Kind: "subscription", Name: "Sub", Slug: "stable", Enabled: true, Original: current, Draft: current, Published: current, Sync: string(legacy)}
	if err = db.Save(p); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		db.Lock()
		err = s.syncSubscriptions("sub:sub")
		db.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		saved, err := db.Get("sub")
		if err != nil {
			t.Fatal(err)
		}
		data, err := config.Validate(saved.Published)
		if err != nil {
			t.Fatal(err)
		}
		nodes := data["proxies"].([]any)
		if len(nodes) != 1 || nodes[0].(map[string]any)["server"] != "fresh.test" {
			t.Fatal("old node or old server returned")
		}
		if strings.Contains(saved.Published, "old.test") {
			t.Fatal("stale information resurrected")
		}
	}
	files, err := filepath.Glob(filepath.Join(root, "data", "backups", "before-node-cleanup*"))
	if err != nil || len(files) != 1 {
		t.Fatal("expected one cleanup backup", err, len(files))
	}
}
