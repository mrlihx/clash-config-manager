package server

import (
	"bytes"
	"clash-config-manager/internal/config"
	"clash-config-manager/internal/store"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestGlobalRoutingEditsUpdateLinkedSubscriptions(t *testing.T) {
	core, err := exec.LookPath("true")
	if err != nil {
		t.Skip("true command unavailable")
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "routing.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db, "test", core, t.TempDir())
	router := s.Routes()
	if err = db.SaveSession(s.sessionHash("fixture"), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	post := func(path string, data any) map[string]any {
		t.Helper()
		body, _ := json.Marshal(data)
		r := httptest.NewRequest("POST", path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "fixture"})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		var result map[string]any
		if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	src := &store.Profile{ID: "src", Kind: "source", Name: "Source", Slug: "source", Draft: "proxies: [{name: A, type: http, server: node.test, port: 8080}]"}
	if err = db.Save(src); err != nil {
		t.Fatal(err)
	}
	sets := []string{"Ads", "Reject", "Overseas", "GFW", "ForeignTLD", "TelegramCIDR"}
	sub := post("/api/subscriptions", map[string]any{"selections": []any{map[string]any{"id": "src", "nodes": []string{"A"}}}, "services": []string{"Claude"}, "ruleSets": sets})
	id := sub["id"].(string)
	originalLink := sub["subscription"]
	original, _ := db.Get(id)
	override := strings.Replace(original.Draft, "port: 7890", "port: 8890", 1)
	post("/api/profiles/"+id, map[string]any{"action": "save-subscription", "revision": original.Revision, "yaml": override})
	template, _ := s.routingTemplate()
	post("/api/profiles/"+template.ID, map[string]any{"action": "edit", "revision": template.Revision, "edit": map[string]any{"section": "rule-providers", "value": map[string]any{"name": "GlobalBlock", "type": "inline", "behavior": "classical"}, "payloadText": "DOMAIN-SUFFIX,blocked.test", "target": "REJECT"}})
	current, err := db.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	data, e := config.Validate(current.Published)
	if e != nil {
		t.Fatal(e)
	}
	if data["port"] != 8890 || !slices.Contains(data["rules"].([]any), any("RULE-SET,GlobalBlock,REJECT")) || current.UpdateError != "" {
		t.Fatal("global rule sync failed or overwrote custom settings", current.UpdateError)
	}
	state, e := routingState(current)
	if e != nil || state.RoutingBaseline == "" {
		t.Fatal("routing baseline missing", e)
	}
	post("/api/profiles/"+id, map[string]any{"action": "save-subscription", "revision": current.Revision, "yaml": current.Draft, "followRouting": false})
	template, _ = s.routingTemplate()
	post("/api/profiles/"+template.ID, map[string]any{"action": "edit", "revision": template.Revision, "edit": map[string]any{"section": "proxy-groups", "key": config.AutoGroup, "value": map[string]any{"name": config.AutoGroup, "interval": 17}}})
	independent, _ := db.Get(id)
	if independent.Published != current.Published {
		t.Fatal("independent subscription was changed")
	}
	response := httptest.NewRecorder()
	s.profileReply(response, independent)
	var reply map[string]any
	_ = json.Unmarshal(response.Body.Bytes(), &reply)
	if reply["subscription"] != originalLink {
		t.Fatal("subscription address changed")
	}
	copied := post("/api/routing-plans", map[string]any{"name": "Separate plan", "copyID": templateID})
	planID := copied["id"].(string)
	plan, _ := db.Get(planID)
	post("/api/profiles/"+planID, map[string]any{"action": "edit", "revision": plan.Revision, "edit": map[string]any{"section": "rules", "value": "DOMAIN,plan-only.test,DIRECT", "position": 0}})
	scoped := post("/api/subscriptions", map[string]any{"routingPlan": planID, "selections": []any{map[string]any{"id": "src", "nodes": []string{"A"}}}, "services": []string{"Claude"}, "ruleSets": sets})
	scopedBefore, _ := db.Get(scoped["id"].(string))
	template, _ = s.routingTemplate()
	post("/api/profiles/"+template.ID, map[string]any{"action": "edit", "revision": template.Revision, "edit": map[string]any{"section": "rules", "value": "DOMAIN,default-only.test,DIRECT", "position": 0}})
	scopedAfter, _ := db.Get(scopedBefore.ID)
	if scopedAfter.Published != scopedBefore.Published {
		t.Fatal("another plan changed the subscription")
	}
	independent, _ = db.Get(id)
	post("/api/profiles/"+id, map[string]any{"action": "save-subscription", "revision": independent.Revision, "yaml": independent.Draft, "routingPlan": planID, "followRouting": true})
	switched, _ := db.Get(id)
	switchedData, _ := config.Validate(switched.Published)
	if !slices.Contains(switchedData["rules"].([]any), any("DOMAIN,plan-only.test,DIRECT")) || switchedData["port"] != 8890 {
		t.Fatal("switching plans lost new rules or custom override")
	}
}

func TestRoutingSyncFailureKeepsLastPublished(t *testing.T) {
	core, _ := exec.LookPath("true")
	db, e := store.Open(filepath.Join(t.TempDir(), "routing.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s := New(db, "test", core, t.TempDir())
	template, e := s.routingTemplate()
	if e != nil {
		t.Fatal(e)
	}
	template.Draft, e = config.ApplyEdit(template.Draft, config.Edit{Section: "proxy-groups", Value: map[string]any{"name": "Custom", "type": "select", "proxies": []string{config.MainGroup}}})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Save(template); e != nil {
		t.Fatal(e)
	}
	baseline, e := config.BuildRouting("proxies: [{name: A, type: http, server: node.test, port: 80}]", template.Draft, []string{}, []string{})
	if e != nil {
		t.Fatal(e)
	}
	current, e := config.ApplyEdit(baseline, config.Edit{Section: "rules", Value: "DOMAIN,private.test,Custom"})
	if e != nil {
		t.Fatal(e)
	}
	state, _ := json.Marshal(subscriptionSync{RoutingPlan: template.ID, RoutingBaseline: baseline})
	sub := &store.Profile{ID: "sub", Kind: "subscription", Name: "Sub", Slug: "sub", Enabled: true, Original: baseline, Draft: current, Published: current, Sync: string(state)}
	if e = db.Save(sub); e != nil {
		t.Fatal(e)
	}
	template.Draft, e = config.ApplyEdit(template.Draft, config.Edit{Section: "proxy-groups", Key: "Custom", Delete: true})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Save(template); e != nil {
		t.Fatal(e)
	}
	if e = s.syncRoutingSubscriptions(context.Background(), template.ID, ""); e != nil {
		t.Fatal(e)
	}
	saved, _ := db.Get(sub.ID)
	if saved.Published != current || saved.UpdateError == "" {
		t.Fatal("failed sync did not preserve snapshot and report error")
	}
}
