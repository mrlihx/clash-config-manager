package config

import (
	"go.yaml.in/yaml/v3"
	"testing"
)

func TestShadowrocketPreservesHTTPChain(t *testing.T) {
	source := `proxies:
 - {name: US, type: anytls, server: front.test, port: 443, password: front-secret, sni: front.test}
 - {name: HTTP落地, type: http, server: exit.test, port: 8080, username: 'user@name', password: 'p:a#ss', dialer-proxy: US}
 - {name: HTTPS落地, type: http, server: exit.test, port: 8443, tls: true, sni: exit.test, dialer-proxy: US}
`
	out, err := ExportShadowrocket(source)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Validate(out)
	if err != nil {
		t.Fatal(err)
	}
	nodes := data["proxies"].([]any)
	if len(nodes) != 3 {
		t.Fatalf("lost nodes: %d", len(nodes))
	}
	exit := nodes[1].(map[string]any)
	if exit["dialer-proxy"] != "US" || exit["username"] != "user@name" || exit["password"] != "p:a#ss" {
		t.Fatal("lost HTTP credentials or chain")
	}
	if nodes[2].(map[string]any)["tls"] != true {
		t.Fatal("lost HTTPS TLS")
	}
	if ClientFormat("Shadowrocket/2.2.66") != "shadowrocket" {
		t.Fatal("wrong client format")
	}
	links, skipped, err := ExportLinks(source)
	if err != nil || links == "" || skipped != 2 {
		t.Fatal("other client export changed")
	}
}

func TestShadowrocketExpandsFrontGroup(t *testing.T) {
	source := `proxies:
 - {name: A, type: anytls, server: a.test, port: 443, password: secret}
 - {name: B, type: vless, server: b.test, port: 443, uuid: example-id, tls: true}
 - {name: Exit, type: http, server: exit.test, port: 80, dialer-proxy: Front}
proxy-groups:
 - {name: Front, type: select, proxies: [A, B]}
rules: ["MATCH,DIRECT"]
`
	// 这里只校验节点订阅，不包含 Clash 策略组或路由规则。
	out, err := ExportShadowrocket(source)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Validate(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 1 {
		t.Fatal("export included routing settings")
	}
	nodes := data["proxies"].([]any)
	if len(nodes) != 4 {
		t.Fatalf("wanted 2 fronts and 2 exits, got %d", len(nodes))
	}
	fronts := map[string]bool{}
	for _, raw := range nodes {
		node := raw.(map[string]any)
		if node["type"] == "http" {
			fronts[node["dialer-proxy"].(string)] = true
		}
	}
	if !fronts["A"] || !fronts["B"] {
		t.Fatal("front group members not retained")
	}
	var parsed map[string]any
	if err = yaml.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
}

func TestShadowrocketNestedChainHasNoDanglingReferences(t *testing.T) {
	source := `proxies:
 - {name: A, type: http, server: a.test, port: 80}
 - {name: B, type: http, server: b.test, port: 80}
 - {name: Mid, type: http, server: mid.test, port: 80, dialer-proxy: Front}
 - {name: End, type: http, server: end.test, port: 80, dialer-proxy: Mid}
proxy-groups:
 - {name: Front, type: select, proxies: [A,B]}
`
	out, err := ExportShadowrocket(source)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Validate(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(data["proxies"].([]any)) != 6 {
		t.Fatal("multi-hop chain not expanded")
	}
}

func TestShadowrocketKeepsOriginalNodeOrder(t *testing.T) {
	source := `proxies:
 - {name: Exit, type: http, server: exit.test, port: 80, dialer-proxy: Front}
 - {name: Last, type: http, server: last.test, port: 80}
 - {name: Front, type: http, server: front.test, port: 80}
`
	out, err := ExportShadowrocket(source)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Validate(out)
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"Exit", "Last", "Front"} {
		if data["proxies"].([]any)[i].(map[string]any)["name"] != name {
			t.Fatal("chain resolution changed node order")
		}
	}
}
