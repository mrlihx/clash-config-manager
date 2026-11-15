package config

import (
	"strings"
	"testing"
)

func TestInlineResourcesPreservesRoutingAndExpandsNodes(t *testing.T) {
	source := `proxies: [{name: Local, type: http, server: local.test, port: 8080}]
proxy-providers:
 Remote: {type: http, url: 'https://nodes.example.test', filter: 'Remote'}
rule-providers:
 Domains: {type: http, behavior: domain, url: 'https://rules.example.test', interval: 60}
 Networks: {type: file, behavior: ipcidr, path: './networks.yaml'}
proxy-groups:
 - {name: Choice, type: select, proxies: [Local], use: [Remote]}
rules: ['RULE-SET,Domains,Choice', 'RULE-SET,Networks,DIRECT,no-resolve', 'MATCH,DIRECT']
dns:
 nameserver-policy:
  rule-set:Domains: [114.114.114.114]
`
	out, err := InlineResources(source, func(resource Resource) ([]byte, error) {
		switch resource.Name {
		case "Remote":
			return []byte("proxies: [{name: Remote, type: http, server: remote.test, port: 8080}]"), nil
		case "Domains":
			return []byte("payload: ['+.example.test']"), nil
		default:
			return []byte("payload: ['203.0.113.0/24', '2001:db8::/32']"), nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := Validate(out)
	if err != nil {
		t.Fatal(err)
	}
	if data["proxy-providers"] != nil || strings.Contains(out, "https://rules.example.test") || strings.Contains(out, "https://nodes.example.test") {
		t.Fatal("external resource remained", out)
	}
	providers := data["rule-providers"].(map[string]any)
	if providers["Domains"].(map[string]any)["type"] != "inline" {
		t.Fatal("rules were not inlined")
	}
	group := data["proxy-groups"].([]any)[0].(map[string]any)
	if group["use"] != nil || len(group["proxies"].([]any)) != 2 {
		t.Fatal("group use reference was lost", out)
	}
	if len(data["rules"].([]any)) != 3 || !strings.Contains(out, "rule-set:Domains") {
		t.Fatal("routing order or DNS policy changed")
	}
	if !strings.Contains(source, "use: [Remote]") {
		t.Fatal("source was mutated")
	}
}

func TestNodeOnlyExportDoesNotFetchRules(t *testing.T) {
	source := `proxy-providers: {Remote: {type: http, url: 'https://nodes.example.test'}}
rule-providers: {Rules: {type: http, behavior: domain, url: 'https://rules.example.test'}}
proxy-groups: [{name: Choice, type: select, use: [Remote]}]
rules: ['RULE-SET,Rules,Choice', 'MATCH,DIRECT']
`
	out, err := InlineNodeProviders(source, func(resource Resource) ([]byte, error) {
		if resource.Section != "proxy-providers" {
			t.Fatal("node client should not download rule resources")
		}
		return []byte("proxies: [{name: Remote, type: vless, server: node.test, port: 443, uuid: fixture}]"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, skipped, err := ExportLinks(out); err != nil || skipped != 0 {
		t.Fatal("expanded nodes cannot be exported", err)
	}
}
