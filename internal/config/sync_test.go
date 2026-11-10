package config

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

func TestSyncNodesPreservesRulesAndGroupPolicies(t *testing.T) {
	before, err := CollectNodes([]NodeSelection{{Name: "source", Source: "proxies: [{name: A, type: http, server: old.test, port: 80}, {name: B, type: http, server: b.test, port: 80}]", Nodes: []string{"A", "B"}}})
	if err != nil {
		t.Fatal(err)
	}
	after, err := CollectNodes([]NodeSelection{{Name: "source", Source: "proxies: [{name: A, type: http, server: new.test, port: 8080}, {name: C, type: http, server: c.test, port: 80}]", Nodes: []string{"A", "C"}}})
	if err != nil {
		t.Fatal(err)
	}
	current := strings.Replace(before, "rules:\n", "rules:\n  - DOMAIN,custom.example,DIRECT\n", 1)
	current = strings.Replace(current, "interval: 60", "interval: 123", 1)
	result, err := SyncNodes(current, before, after)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "custom.example") || !strings.Contains(result, "interval: 123") || !strings.Contains(result, "new.test") || strings.Contains(result, "b.test") {
		t.Fatal("sync lost edits or retained stale nodes")
	}
	data, _ := Validate(result)
	groups := data["proxy-groups"].([]any)
	for _, g := range groups[:3] {
		members := g.(map[string]any)["proxies"].([]any)
		if len(members) != 2 || members[0] != "A" || members[1] != "C" {
			t.Fatal("base groups did not track additions and removals")
		}
	}
}

func TestClientExportVLESSRoundTrip(t *testing.T) {
	link := "vless://test-uuid@node.example:443?security=reality&type=tcp&sni=sni.example&fp=chrome&pbk=test-key&sid=1234&flow=xtls-rprx-vision#test"
	_, err := NodeLink(link)
	if err != nil {
		t.Fatal(err)
	}
	source, err := SubscriptionYAML(link)
	if err != nil {
		t.Fatal(err)
	}
	encoded, skipped, err := ExportLinks(source)
	if err != nil || skipped != 0 {
		t.Fatal("export failed", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(string(decoded))
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("pbk") != "test-key" || u.Query().Get("flow") != "xtls-rprx-vision" || u.Host != "node.example:443" {
		t.Fatal("Reality parameters lost")
	}
	if _, err = NodeLink(string(decoded)); err != nil {
		t.Fatal(err)
	}
	for agent, want := range map[string]string{"v2rayNG/1.0": "v2ray", "Clash.Meta": "clash", "Mozilla": "clash", "": "clash", "Shadowrocket/1": "v2ray"} {
		if ClientFormat(agent) != want {
			t.Fatal(agent)
		}
	}
}
