package config

import (
	"go.yaml.in/yaml/v3"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestGenerateOnlyAdoptsNodesAndDetailsServices(t *testing.T) {
	result, err := Generate(example, "balanced", true)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Validate(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result, "DOMAIN,example.test,origin") {
		t.Fatal("imported source rule")
	}
	groups := data["proxy-groups"].([]any)
	if len(groups) < 18 {
		t.Fatal("missing detailed service groups")
	}
	for _, id := range []string{"Claude", "GooglePlay", "GitHub", "YouTube", "Netflix"} {
		if !strings.Contains(result, "RULE-SET,System_"+id+",") {
			t.Fatal(id)
		}
	}
	custom, err := ApplyEdit(result, Edit{Section: "rules", Value: "DOMAIN,custom.test,DIRECT", Position: ptr(0)})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := Generate(custom, "balanced", false)
	if err != nil {
		t.Fatal(err)
	}
	_, count := ManagedInfo(generated)
	final, _ := Validate(generated)
	rules := final["rules"].([]any)
	if rules[count] != "DOMAIN,custom.test,DIRECT" {
		t.Fatal("custom rule overtook builtin rules")
	}
	repeated, err := Generate(generated, "balanced", false)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := Validate(repeated)
	if !reflect.DeepEqual(final, again) {
		t.Fatal("regeneration changed configuration")
	}
}
func ptr(i int) *int { return &i }

func TestSelectedExternalRulesBringProviderAndUseCurrentTarget(t *testing.T) {
	source := "rule-providers:\n  External:\n    type: inline\n    behavior: domain\n    payload: ['+.external.test']\nrules: ['RULE-SET,External,Airport', 'MATCH,Airport']\n"
	selection := map[string][]string{"rules": {"0"}}
	transformed, err := SelectedRules(source, selection, MainGroup)
	if err != nil {
		t.Fatal(err)
	}
	base, err := Generate("proxies: []\n", "balanced", true)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := Merge(base, transformed, selection)
	if err != nil {
		t.Fatal(err)
	}
	merged, err = Generate(merged, "balanced", false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Validate(merged)
	if err != nil {
		t.Fatal(err)
	}
	rules := data["rules"].([]any)
	_, count := ManagedInfo(merged)
	if rules[count] != "RULE-SET,External,"+MainGroup {
		t.Fatal("external rule priority or target incorrect")
	}
	if _, ok := data["rule-providers"].(map[string]any)["External"]; !ok {
		t.Fatal("missing external provider")
	}
}

func TestSubscriptionSelectionAndDomesticRules(t *testing.T) {
	selected, err := CollectNodes([]NodeSelection{{Name: "A", Source: example, Nodes: []string{"origin"}}})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := SubscriptionOptions(selected, []string{"Claude", "GooglePlay"}, []string{"Ads"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := Validate(generated)
	if err != nil {
		t.Fatal(err)
	}
	groups := data["proxy-groups"].([]any)
	if len(groups) != 6 {
		t.Fatalf("wanted manual, auto, fallback, advertising and 2 overseas groups, got %d", len(groups))
	}
	for _, raw := range groups {
		group := raw.(map[string]any)
		if group["name"] == AdGroup {
			continue
		}
		expected := []any{"origin"}
		if group["name"] != MainGroup && group["name"] != AutoGroup && group["name"] != FallbackGroup {
			expected = []any{MainGroup, AutoGroup, "origin"}
		}
		if !reflect.DeepEqual(group["proxies"], expected) {
			t.Fatal("all nodes must be default members")
		}
	}
	providers := data["rule-providers"].(map[string]any)
	if _, ok := providers["System_WeChat"]; ok {
		t.Fatal("domestic service provider should not be emitted")
	}
	if _, ok := providers["System_Reject"]; ok {
		t.Fatal("unselected rule provider was emitted")
	}
	for _, rule := range []string{"RULE-SET,System_China,DIRECT", "RULE-SET,System_ChinaCIDR,DIRECT,no-resolve"} {
		if strings.Contains(generated, rule) {
			t.Fatal("unselected domestic direct rule was emitted")
		}

	}
	if _, ok := providers["System_Gemini"]; ok {
		t.Fatal("unselected service rules were emitted")
	}
}

func TestCollectedChainRenamesDependencies(t *testing.T) {
	source := `proxies:
 - {name: Front, type: http, server: front.test, port: 8080}
 - {name: Exit, type: http, server: exit.test, port: 8080, dialer-proxy: FrontGroup}
proxy-groups:
 - {name: FrontGroup, type: select, proxies: [Front]}
`
	selected, err := CollectNodes([]NodeSelection{{Name: "A", Source: source, Nodes: []string{"Exit"}}, {Name: "B", Source: source, Nodes: []string{"Exit"}}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := Validate(selected)
	if err != nil {
		t.Fatal(err)
	}
	nodes := data["proxies"].([]any)
	if len(nodes) != 4 {
		t.Fatal("missing selected nodes or dependencies")
	}
	if nodes[3].(map[string]any)["dialer-proxy"] != "FrontGroup · B" {
		t.Fatal("renamed dialer reference was not updated")
	}
}

func TestGeneratedDefaultsAndAutomaticGroup(t *testing.T) {
	source, err := Generate(example, "balanced", true)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Validate(source)
	if err != nil {
		t.Fatal(err)
	}
	for key, expected := range map[string]any{"port": 7890, "socks-port": 7891, "redir-port": 7892, "tproxy-port": 7895, "bind-address": "*", "external-controller": "127.0.0.1:9090"} {
		if data[key] != expected {
			t.Errorf("%s got %v", key, data[key])
		}
	}
	groups := data["proxy-groups"].([]any)
	if groups[0].(map[string]any)["name"] != MainGroup || groups[1].(map[string]any)["name"] != AutoGroup {
		t.Fatal("manual and automatic groups are out of order")
	}
	if groups[1].(map[string]any)["type"] != "url-test" {
		t.Fatal("automatic group must use url-test")
	}
	dns := data["dns"].(map[string]any)
	if _, ok := dns["nameserver-policy"].(map[string]any)["rule-set:System_China,System_SteamCDN"]; !ok {
		t.Fatal("missing direct DNS policy")
	}
	if len(dns["fake-ip-filter"].([]any)) != 5 {
		t.Fatal("missing Fake-IP filters")
	}
}

func TestRoutingTemplatePreservesEditedRulesAndIntervals(t *testing.T) {
	template, err := Generate(example, "balanced", true)
	if err != nil {
		t.Fatal(err)
	}
	template, err = ApplyEdit(template, Edit{Section: "rule-providers", Key: "Ads", Value: map[string]any{"name": "Ads", "interval": 3600}})
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := Parse(template)
	var key string
	for i, rule := range field(doc.Content[0], "rules").Content {
		if strings.HasPrefix(rule.Value, "RULE-SET,Ads,") {
			key = strconv.Itoa(i)
		}
	}
	template, err = ApplyEdit(template, Edit{Section: "rules", Key: key, Value: "RULE-SET,Ads,PASS"})
	if err != nil {
		t.Fatal(err)
	}
	template, err = ApplyEdit(template, Edit{Section: "proxy-groups", Key: "🧠 Claude", Value: map[string]any{"name": "🧠 Claude", "type": "fallback", "url": "https://www.gstatic.com/generate_204", "interval": 90}})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := Generate(example, "balanced", true)
	if err != nil {
		t.Fatal(err)
	}
	generated, err = SubscriptionOptions(generated, []string{"Claude"}, []string{"Ads"})
	if err != nil {
		t.Fatal(err)
	}
	generated, err = ApplyRoutingTemplate(generated, template, []string{"Ads"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := Validate(generated)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(data["rules"].([]any), any("RULE-SET,Ads,"+AdGroup)) {
		t.Fatal("advertising rule must use the fixed blocking group")
	}
	if data["rule-providers"].(map[string]any)["Ads"].(map[string]any)["interval"] != 3600 {
		t.Fatal("provider interval did not follow template")
	}
	groups := data["proxy-groups"].([]any)
	for _, raw := range groups {
		g := raw.(map[string]any)
		if g["name"] == "🧠 Claude" && (g["type"] != "fallback" || g["interval"] != 90) {
			t.Fatal("template strategy did not follow into subscription")
		}
	}

	if !slices.ContainsFunc(groups, func(raw any) bool { return raw.(map[string]any)["name"] == "🧠 Claude" }) {
		t.Fatal("missing service emoji")
	}
}

func TestLegacyGroupUpgradeRunsOnce(t *testing.T) {
	legacy := `proxies:
 - {name: Node, type: http, server: node.test, port: 8080}
proxy-groups:
 - {name: 🚀 手动选择, type: select, proxies: [Node]}
 - {name: Claude, type: select, proxies: [Node]}
rule-providers:
 Ads: {type: inline, behavior: domain, payload: ['+.fixture.test']}
rules:
 - RULE-SET,Ads,REJECT
 - DOMAIN-SUFFIX,claude.ai,Claude
 - MATCH,🚀 手动选择
x-manager:
 preset: balanced
 ads: REJECT
`
	upgraded, err := UpgradeRoutingDefaults(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Validate(upgraded); err != nil {
		t.Fatal(err)
	}
	data, _ := Validate(upgraded)
	rules := data["rules"].([]any)
	groups := data["proxy-groups"].([]any)
	if !slices.Contains(rules, any("DOMAIN-SUFFIX,claude.ai,🧠 Claude")) || !slices.ContainsFunc(groups, func(raw any) bool { return raw.(map[string]any)["name"] == AdGroup }) {
		t.Fatal("group migration lost routing references")
	}

	manual := strings.Replace(upgraded, "RULE-SET,Ads,🛡️ 广告拦截", "RULE-SET,Ads,DIRECT", 1)
	repeated, err := UpgradeRoutingDefaults(manual)
	if err != nil || repeated != manual {
		t.Fatal("startup migration rewrote a manual rule change")
	}
}

func TestBaseSettingsFirstAndAdvertisingLast(t *testing.T) {
	source, err := Generate(example, "balanced", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(source, "port: 7890\n") {
		t.Fatal("network settings were not placed first")
	}
	if strings.Index(source, "dns:") > strings.Index(source, "proxies:") {
		t.Fatal("DNS settings appeared below nodes")
	}
	data, err := Validate(source)
	if err != nil {
		t.Fatal(err)
	}
	groups := data["proxy-groups"].([]any)
	if groups[len(groups)-1].(map[string]any)["name"] != AdGroup {
		t.Fatal("advertising group is not last")
	}
	fallback := groups[2].(map[string]any)
	if fallback["name"] != FallbackGroup || fallback["type"] != "fallback" {
		t.Fatal("fallback group must follow automatic selection")
	}
	for name, p := range data["rule-providers"].(map[string]any) {
		provider := p.(map[string]any)
		if provider["type"] == "http" && provider["path"] != nil {
			t.Fatal("rule cache path should use the core default for " + name)
		}
	}
	auto := groups[1].(map[string]any)
	if auto["interval"] != 60 || auto["tolerance"] != 0 || auto["lazy"] != false {
		t.Fatal("automatic selection defaults do not actively test nodes")
	}
}

func TestEditSubscriptionRoutingPreservesSettings(t *testing.T) {
	template, err := Generate(example, "balanced", true)
	if err != nil {
		t.Fatal(err)
	}
	current, err := SubscriptionOptions(template, []string{"Claude"}, []string{"Ads"})
	if err != nil {
		t.Fatal(err)
	}
	current = strings.Replace(current, "port: 7890", "port: 8890", 1)
	current = strings.Replace(current, "interval: 60", "interval: 123", 1)
	current = strings.Replace(current, "rules:\n", "rules:\n  - DOMAIN,custom.test,DIRECT\n", 1)
	result, err := ConfigureSubscriptionRouting(current, template, []string{"Claude", "Gemini"}, []string{"GFW"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := Validate(result)
	if data["rules"].([]any)[0] != "DOMAIN,custom.test,DIRECT" {
		t.Fatal("custom rule priority changed")
	}
	if data["port"] != 8890 || !strings.Contains(result, "interval: 123") || !strings.Contains(result, "DOMAIN,custom.test,DIRECT") {
		t.Fatal("existing settings were overwritten")
	}
	groups := data["proxy-groups"].([]any)
	if !slices.ContainsFunc(groups, func(v any) bool { return v.(map[string]any)["name"] == "✨ Gemini" }) {
		t.Fatal("selected service missing")
	}
	rules := data["rules"].([]any)
	if slices.Contains(rules, any("RULE-SET,Ads,"+AdGroup)) || !slices.Contains(rules, any("RULE-SET,System_GFW,"+MainGroup)) {
		t.Fatal("rule selection not applied")
	}
	if _, err = ConfigureSubscriptionRouting(result, template, []string{"not-a-service"}, nil); err == nil {
		t.Fatal("accepted invalid service")
	}
}

func TestAdvertisingGroupHasOnlyRejectAndCacheUsesDefaults(t *testing.T) {
	source, err := Generate(example, "balanced", true)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	group := slicesFind(children(field(doc.Content[0], "proxy-groups")), AdGroup)
	set(group, "proxies", &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{scalar("PASS"), scalar(MainGroup)}})
	providers := field(doc.Content[0], "rule-providers")
	set(field(providers, "Ads"), "path", scalar("./custom-ads.yaml"))
	source, err = encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Validate(source)
	if err != nil {
		t.Fatal(err)
	}
	if data["rule-providers"].(map[string]any)["Ads"].(map[string]any)["path"] != nil {
		t.Fatal("custom cache path retained")
	}
	groups := data["proxy-groups"].([]any)
	if !reflect.DeepEqual(groups[len(groups)-1].(map[string]any)["proxies"], []any{"REJECT"}) {
		t.Fatal("advertising group is not fixed")
	}
}

func TestDomesticDirectRulesAreOptionalAndDNSReferencesStayValid(t *testing.T) {
	template, err := Generate(example, "balanced", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		sets       []string
		domain, ip bool
	}{
		{"none", []string{}, false, false},
		{"domain", []string{"China"}, true, false},
		{"ip", []string{"ChinaCIDR"}, false, true},
		{"both", []string{"China", "ChinaCIDR"}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output, err := BuildRouting(template, template, []string{}, tc.sets)
			if err != nil {
				t.Fatal(err)
			}
			data, err := Validate(output)
			if err != nil {
				t.Fatal("DNS or routing references were broken", err)
			}
			providers := data["rule-providers"].(map[string]any)
			_, domain := providers["System_China"]
			_, ip := providers["System_ChinaCIDR"]
			if domain != tc.domain || ip != tc.ip {
				t.Fatal("provider selection ignored", domain, ip)
			}
			rules := data["rules"].([]any)
			if slices.Contains(rules, any("RULE-SET,System_China,DIRECT")) != tc.domain || slices.Contains(rules, any("RULE-SET,System_ChinaCIDR,DIRECT,no-resolve")) != tc.ip {
				t.Fatal("routing selection ignored")
			}
		})
	}
}
