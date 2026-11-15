package server

import (
	"clash-config-manager/internal/config"
	"clash-config-manager/internal/store"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestShadowrocketSubscriptionKeepsChainWithoutRuleCache(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := `proxies:
 - {name: US, type: anytls, server: front.test, port: 443, password: example}
 - {name: Exit, type: http, server: exit.test, port: 80, dialer-proxy: US}
rule-providers:
 System_Claude: {type: http, behavior: domain, url: 'http://127.0.0.1:1/unavailable'}
rules: ['RULE-SET,System_Claude,DIRECT','MATCH,DIRECT']
`
	if err = db.Save(&store.Profile{ID: "x", Slug: "stable", Enabled: true, Draft: source, Published: source}); err != nil {
		t.Fatal(err)
	}
	s := New(db, "password", "", root)
	for _, agent := range []string{"Shadowrocket/2.2.66", "v2rayNG/1"} {
		request := httptest.NewRequest("GET", "/s/stable", nil)
		request.Header.Set("User-Agent", agent)
		response := httptest.NewRecorder()
		s.Routes().ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatal(agent, response.Code, response.Body.String())
		}
		if response.Header().Get("Vary") != "User-Agent" {
			t.Fatal("missing client cache separation")
		}
		if strings.HasPrefix(agent, "Shadowrocket") {
			data, err := config.Validate(response.Body.String())
			if err != nil {
				t.Fatal(err)
			}
			nodes := data["proxies"].([]any)
			if len(nodes) != 2 || nodes[1].(map[string]any)["dialer-proxy"] != "US" {
				t.Fatal("HTTP chain missing")
			}
			if data["rule-providers"] != nil {
				t.Fatal("node subscription includes rules")
			}
		} else if response.Header().Get("X-Subscription-Skipped") != "1" {
			t.Fatal("v2ray client export unexpectedly changed")
		}
	}
	p, _ := db.Get("x")
	if p.Draft != source {
		t.Fatal("export modified stored subscription")
	}
}

func TestClientSubscriptionCleansLegacySnapshotNames(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := `proxies:
 - {name: Front, type: http, server: front.test, port: 80}
 - {name: Front · 订阅自有节点, type: http, server: front.test, port: 80}
 - {name: Exit, type: http, server: exit.test, port: 80, dialer-proxy: Front · 订阅自有节点}
`
	if err = db.Save(&store.Profile{ID: "x", Slug: "stable", Enabled: true, Draft: source, Published: source}); err != nil {
		t.Fatal(err)
	}
	s := New(db, "password", "", root)
	for _, agent := range []string{"Clash.Meta", "Shadowrocket/2.2.66"} {
		req := httptest.NewRequest("GET", "/s/stable", nil)
		req.Header.Set("User-Agent", agent)
		response := httptest.NewRecorder()
		s.Routes().ServeHTTP(response, req)
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "订阅自有节点") {
			t.Fatal("legacy name exposed to client")
		}
		data, err := config.Validate(response.Body.String())
		if err != nil {
			t.Fatal(err)
		}
		if len(data["proxies"].([]any)) != 2 {
			t.Fatal("client lost node or retained duplicate")
		}
	}
}
