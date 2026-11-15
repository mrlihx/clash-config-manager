package config

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestManualRuleTextAndAutomaticBinding(t *testing.T) {
	for _, input := range []string{"DOMAIN-SUFFIX,example.test\nIP-CIDR,203.0.113.0/24,no-resolve", "payload:\n - DOMAIN-SUFFIX,example.test\n - IP-CIDR,203.0.113.0/24,no-resolve"} {
		rules, e := RuleText(input, "classical")
		if e != nil || len(rules) != 2 {
			t.Fatal(e)
		}
	}
	if _, e := RuleText("DOMAIN-SUFFIX,example.test,DIRECT", "classical"); e == nil {
		t.Fatal("accepted per-line target")
	}
	source, e := Generate(example, "balanced", true)
	if e != nil {
		t.Fatal(e)
	}
	text := "DOMAIN-SUFFIX,example.test"
	result, e := ApplyRuleProvider(source, Edit{Section: "rule-providers", Value: map[string]any{"name": "Manual", "type": "inline", "behavior": "classical"}, PayloadText: &text})
	if e != nil {
		t.Fatal(e)
	}
	data, e := Validate(result)
	if e != nil {
		t.Fatal(e)
	}
	if !slices.Contains(data["rules"].([]any), any("RULE-SET,Manual,"+MainGroup)) {
		t.Fatal("missing default strategy binding")
	}
	provider := data["rule-providers"].(map[string]any)["Manual"].(map[string]any)
	if !reflect.DeepEqual(provider["payload"], []any{"DOMAIN-SUFFIX,example.test"}) {
		t.Fatal("manual rules were not stored")
	}
	result, e = ApplyRuleProvider(result, Edit{Section: "rule-providers", Key: "Manual", Delete: true})
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(result, "RULE-SET,Manual,") {
		t.Fatal("deleted provider left dangling reference")
	}
}

func TestRoutingInheritanceKeepsOverridesAndAddsGlobalRules(t *testing.T) {
	template, e := Generate("proxies: []", "balanced", true)
	if e != nil {
		t.Fatal(e)
	}
	baseline, e := BuildRouting(example, template, []string{"Claude"}, []string{"Ads"})
	if e != nil {
		t.Fatal(e)
	}
	current, e := ApplyEdit(baseline, Edit{Section: "proxy-groups", Key: "🧠 Claude", Value: map[string]any{"name": "🧠 Claude", "type": "fallback", "interval": 77}})
	if e != nil {
		t.Fatal(e)
	}
	current = strings.Replace(current, "port: 7890", "port: 8890", 1)
	current, e = ApplyEdit(current, Edit{Section: "rules", Value: "DOMAIN,private-override.test,DIRECT", Position: ptr(0)})
	if e != nil {
		t.Fatal(e)
	}
	template, e = ApplyEdit(template, Edit{Section: "proxy-groups", Key: AutoGroup, Value: map[string]any{"name": AutoGroup, "interval": 120}})
	if e != nil {
		t.Fatal(e)
	}
	template, e = ApplyEdit(template, Edit{Section: "proxy-groups", Key: "🧠 Claude", Value: map[string]any{"name": "🧠 Claude", "type": "url-test", "interval": 90}})
	if e != nil {
		t.Fatal(e)
	}
	text := "DOMAIN-SUFFIX,blocked.test"
	template, e = ApplyRuleProvider(template, Edit{Section: "rule-providers", Value: map[string]any{"name": "GlobalBlock", "type": "inline", "behavior": "classical"}, PayloadText: &text, Target: "REJECT"})
	if e != nil {
		t.Fatal(e)
	}
	next, e := BuildRouting(current, template, []string{"Claude"}, []string{"Ads", "GlobalBlock"})
	if e != nil {
		t.Fatal(e)
	}
	merged, e := SyncRouting(current, baseline, next)
	if e != nil {
		t.Fatal(e)
	}
	data, _ := Validate(merged)
	if data["port"] != 8890 {
		t.Fatal("custom port overwritten")
	}
	for _, raw := range data["proxy-groups"].([]any) {
		g := raw.(map[string]any)
		if g["name"] == AutoGroup && g["interval"] != 120 {
			t.Fatal("inherited group did not update")
		}
		if g["name"] == "🧠 Claude" && (g["interval"] != 77 || g["type"] != "fallback") {
			t.Fatal("subscription override lost")
		}
	}
	rules := data["rules"].([]any)
	if !slices.Contains(rules, any("RULE-SET,GlobalBlock,REJECT")) || !slices.Contains(rules, any("DOMAIN,private-override.test,DIRECT")) {
		t.Fatal("global or custom rule missing")
	}
	repeated, e := SyncRouting(merged, next, next)
	if e != nil || repeated != merged {
		t.Fatal("routing sync not idempotent", e)
	}
}
