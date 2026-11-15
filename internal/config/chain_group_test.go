package config

import "testing"

func TestChainGroupLegacyNameAndReferences(t *testing.T) {
	source := `proxies:
 - {name: Front, type: http, server: front.test, port: 80}
 - {name: Exit, type: http, server: exit.test, port: 80, dialer-proxy: 🔀 链式前置-新加坡}
proxy-groups:
 - {name: 🔀 链式前置-新加坡, type: select, proxies: [Front]}
rules: ["MATCH,🔀 链式前置-新加坡"]
`
	data, err := Validate(source)
	if err != nil {
		t.Fatal(err)
	}
	if data["proxy-groups"].([]any)[0].(map[string]any)["name"] != "🔀 链式前置" {
		t.Fatal("group not renamed")
	}
	if data["proxies"].([]any)[1].(map[string]any)["dialer-proxy"] != "🔀 链式前置" {
		t.Fatal("dialer reference not renamed")
	}
	if data["rules"].([]any)[0] != "MATCH,🔀 链式前置" {
		t.Fatal("rule target not renamed")
	}
}
