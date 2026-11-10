package config

import (
	"embed"
	"errors"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// 内嵌初始化配置；规则内容由远程 provider 更新。
//
//go:embed builtin/*.yaml
var builtinFiles embed.FS

const MainGroup = "🚀 手动选择"
const FallbackGroup = "🔁 故障转移"
const AutoGroup = "♻️ 自动选择"
const DirectGroup = "🎯 全球直连"
const AdGroup = "🛡️ 广告拦截"

type RoutingCategory struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Direct bool   `json:"direct"`
}

var Categories = []RoutingCategory{
	{"AppleStore", "Apple 商店", true}, {"ApplePush", "Apple 推送", true}, {"iCloud", "iCloud", true}, {"AppleChina", "Apple 国内服务", true}, {"Bambu", "拓竹", true},
	{"Claude", "Claude", false}, {"Gemini", "Gemini", false}, {"OpenAI", "OpenAI", false},
	{"Grok", "Grok", false}, {"Perplexity", "Perplexity", false},
	{"GooglePlay", "Google Play", false}, {"YouTube", "YouTube", false}, {"Google", "Google 服务", false},
	{"Telegram", "Telegram", false}, {"Discord", "Discord", false}, {"Twitter", "X / Twitter", false}, {"Facebook", "Facebook", false},
	{"Netflix", "Netflix", false}, {"Disney", "Disney+", false}, {"Spotify", "Spotify", false}, {"TikTok", "TikTok", false},
	{"Bilibili", "哔哩哔哩", true}, {"Douyin", "抖音", true}, {"WeChat", "微信", true}, {"Alipay", "支付宝", true}, {"Taobao", "淘宝", true}, {"JD", "京东", true}, {"Baidu", "百度", true}, {"Tencent", "腾讯服务", true},
	{"GitHub", "GitHub", false}, {"Microsoft", "Microsoft", true}, {"Apple", "Apple 服务", true}, {"Steam", "Steam", false},
}

type managedSettings struct {
	Version    int       `yaml:"routing-version" json:"routingVersion"`
	Services   *[]string `yaml:"services,omitempty" json:"services,omitempty"`
	RuleSets   *[]string `yaml:"rule-sets,omitempty" json:"ruleSets,omitempty"`
	Preset     string    `yaml:"preset" json:"preset"`
	Rules      []string  `yaml:"rules" json:"rules"`
	Ads        string    `yaml:"ads" json:"ads"`
	Exceptions []string  `yaml:"ad-exceptions,omitempty" json:"adExceptions,omitempty"`
}

func Generate(source, preset string, nodesOnly bool) (string, error) {
	if preset == "" {
		preset = "balanced"
	}
	if !slices.Contains([]string{"balanced", "global", "direct"}, preset) {
		return "", errors.New("分流预设无效")
	}
	doc, err := Parse(source)
	if err != nil {
		return "", err
	}
	root := doc.Content[0]
	settings := managedSettings{Version: 3, Preset: preset, Ads: AdGroup}
	var old managedSettings
	if meta := field(root, "x-manager"); meta != nil && !nodesOnly {
		if err := meta.Decode(&old); err != nil {
			return "", errors.New("系统规则元数据无效")
		}
		settings.Ads = AdGroup
		settings.Exceptions = old.Exceptions
		settings.Services = old.Services
		settings.RuleSets = old.RuleSets
	}
	if settings.Ads == "" {
		settings.Ads = AdGroup
	}
	var custom []*yaml.Node
	if !nodesOnly {
		for _, rule := range children(field(root, "rules")) {
			parts := splitRule(rule.Value)
			if strings.TrimSpace(parts[0]) == "MATCH" {
				continue
			}
			if slices.Contains(old.Rules, rule.Value) {
				continue
			}
			// 广告策略由广告页管理，规则始终保留在内置区域。
			if len(parts) > 2 && parts[0] == "RULE-SET" && parts[1] == "Ads" && len(old.Rules) > 0 {
				settings.Ads = AdGroup
				continue
			}
			custom = append(custom, rule)
		}
	}
	if nodesOnly {
		original := root
		root = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		doc.Content[0] = root
		set(root, "proxies", field(original, "proxies"))
		set(root, "proxy-providers", field(original, "proxy-providers"))
		// 只保留节点实际依赖的前置组，避免链式代理因剥离规则丢失引用。
		needed := map[string]bool{}
		var keep func(string)
		groups := field(original, "proxy-groups")
		keep = func(name string) {
			if needed[name] {
				return
			}
			needed[name] = true
			if g := slicesFind(children(groups), name); g != nil {
				for _, m := range children(field(g, "proxies")) {
					keep(m.Value)
				}
			}
		}
		for _, p := range children(field(root, "proxies")) {
			if ref := value(field(p, "dialer-proxy")); ref != "" {
				keep(ref)
			}
		}
		kept := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, g := range children(groups) {
			if needed[value(field(g, "name"))] {
				kept.Content = append(kept.Content, g)
			}
		}
		set(root, "proxy-groups", kept)
		defaults, err := builtinFiles.ReadFile("builtin/defaults.yaml")
		if err != nil {
			return "", err
		}
		base, err := Parse(string(defaults))
		if err != nil {
			return "", err
		}
		for i := 0; i < len(base.Content[0].Content); i += 2 {
			set(root, base.Content[0].Content[i].Value, base.Content[0].Content[i+1])
		}
	}
	proxies := field(root, "proxies")
	if proxies == nil {
		proxies = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		set(root, "proxies", proxies)
	}
	groups := field(root, "proxy-groups")
	if groups == nil {
		groups = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		set(root, "proxy-groups", groups)
	}
	names := []string{}
	for _, p := range children(proxies) {
		names = append(names, value(field(p, "name")))
	}
	uses := []string{}
	for i, n := range children(field(root, "proxy-providers")) {
		if i%2 == 0 {
			uses = append(uses, n.Value)
		}
	}
	ensure := func(name string, members []string) error {
		if slices.Contains(names, name) {
			return fmt.Errorf("节点名称 %s 与系统策略组重名，请先改名", name)
		}
		if slicesFind(groups.Content, name) != nil {
			return nil
		}
		var n yaml.Node
		_ = n.Encode(map[string]any{"name": name, "type": "select", "proxies": members})
		groups.Content = append(groups.Content, &n)
		return nil
	}
	if len(names) == 0 && len(uses) == 0 {
		names = append(names, "DIRECT")
	}
	if err := ensure(MainGroup, names); err != nil {
		return "", err
	}
	if err := ensure(AutoGroup, names); err != nil {
		return "", err
	}
	auto := slicesFind(groups.Content, AutoGroup)
	set(auto, "type", scalar("url-test"))
	if field(auto, "lazy") == nil {
		var n yaml.Node
		_ = n.Encode(false)
		set(auto, "lazy", &n)
	}
	if field(auto, "url") == nil {
		set(auto, "url", scalar("https://www.gstatic.com/generate_204"))
	}
	for key, number := range map[string]int{"interval": 60, "tolerance": 0} {
		if field(auto, key) == nil {
			var n yaml.Node
			_ = n.Encode(number)
			set(auto, key, &n)
		}
	}
	main := slicesFind(groups.Content, MainGroup)
	ordered := []*yaml.Node{main, auto}
	for _, group := range groups.Content {
		if group != main && group != auto {
			ordered = append(ordered, group)
		}
	}
	groups.Content = ordered
	// 新建时默认全选，后续保留用户编辑过的成员。
	if field(main, "proxies") == nil {
		var memberNode yaml.Node
		_ = memberNode.Encode(names)
		set(main, "proxies", &memberNode)
	}
	if len(uses) > 0 {
		var n yaml.Node
		_ = n.Encode(uses)
		set(main, "use", &n)
	}
	if err := ensure(AdGroup, []string{"REJECT"}); err != nil {
		return "", err
	}
	// 国内服务始终直连；分流组只为选中的海外服务生成，默认包含全部节点。
	for _, category := range Categories {
		if category.Direct || (settings.Services != nil && !slices.Contains(*settings.Services, category.ID)) {
			continue
		}
		if err := ensure(category.Name, names); err != nil {
			return "", err
		}
	}

	providers := field(root, "rule-providers")
	if providers == nil {
		providers = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		set(root, "rule-providers", providers)
	}
	for _, id := range append([]string{"Ads", "SteamCDN"}, categoryIDs()...) {
		var provider yaml.Node
		_ = provider.Encode(map[string]any{
			"type": "http", "behavior": "domain", "format": "yaml",
			"url":      "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geosite/" + serviceSlugs[id] + ".yaml",
			"interval": 86400,
		})
		set(providers, "System_"+id, &provider)
	}

	generalData, err := builtinFiles.ReadFile("builtin/providers.yaml")
	if err != nil {
		return "", err
	}
	var general yaml.Node
	if err = yaml.Unmarshal(generalData, &general); err != nil {
		return "", err
	}
	for i := 0; i < len(general.Content[0].Content); i += 2 {
		set(providers, general.Content[0].Content[i].Value, general.Content[0].Content[i+1])
	}

	// Ads 名称沿用现有广告管理入口。
	set(providers, "Ads", field(providers, "System_Ads"))
	set(providers, "System_Ads", nil)
	rules := []string{"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve", "IP-CIDR,10.0.0.0/8,DIRECT,no-resolve", "IP-CIDR,172.16.0.0/12,DIRECT,no-resolve", "IP-CIDR,192.168.0.0/16,DIRECT,no-resolve", "IP-CIDR6,::1/128,DIRECT,no-resolve", "IP-CIDR6,fc00::/7,DIRECT,no-resolve", "IP-CIDR6,fe80::/10,DIRECT,no-resolve"}
	rules = append(rules, "RULE-SET,System_Private,DIRECT", "RULE-SET,System_LanCIDR,DIRECT,no-resolve", "RULE-SET,System_SteamCDN,DIRECT")
	if preset == "balanced" {

		for _, category := range Categories {
			if category.Direct {
				continue
			}
			if settings.Services == nil || slices.Contains(*settings.Services, category.ID) {
				rules = append(rules, "RULE-SET,System_"+category.ID+","+category.Name)
			}
		}
	}
	// App 服务细分在广告集前，避免已知认证、商店和设备接口被通用广告规则拦截。
	for _, domain := range settings.Exceptions {
		rules = append(rules, "DOMAIN-SUFFIX,"+domain+",DIRECT")
	}
	if ruleEnabled(settings, "Ads") {
		rules = append(rules, "RULE-SET,Ads,"+settings.Ads)
	}
	if ruleEnabled(settings, "Reject") {
		rules = append(rules, "RULE-SET,System_Reject,"+settings.Ads)
	}
	if preset == "balanced" {
		telegram := MainGroup
		if settings.Services == nil || slices.Contains(*settings.Services, "Telegram") {
			for _, category := range Categories {
				if category.ID == "Telegram" {
					telegram = category.Name
				}
			}
		}
		if ruleEnabled(settings, "TelegramCIDR") {
			rules = append(rules, "RULE-SET,System_TelegramCIDR,"+telegram+",no-resolve")
		}
		rules = append(rules, "RULE-SET,System_China,DIRECT", "RULE-SET,System_ChinaCIDR,DIRECT,no-resolve")
		for _, id := range []string{"Overseas", "GFW", "ForeignTLD"} {
			if ruleEnabled(settings, id) {
				rules = append(rules, "RULE-SET,System_"+id+","+MainGroup)
			}
		}
	}
	if field(providers, "Custom_Direct_Domain") != nil {
		policy := field(field(root, "dns"), "nameserver-policy")
		for i, key := range children(policy) {
			if i%2 != 0 {
				continue
			}
			names, ok := strings.CutPrefix(key.Value, "rule-set:")
			if !ok {
				continue
			}
			ids := strings.Split(names, ",")
			if slices.Contains(ids, "System_China") && !slices.Contains(ids, "Custom_Direct_Domain") {
				key.Value += ",Custom_Direct_Domain"
			}
		}
	}

	settings.Rules = rules
	var ruleNode yaml.Node
	_ = ruleNode.Encode(rules)
	ruleNode.Content = append(ruleNode.Content, custom...)
	target := MainGroup
	if preset == "direct" {
		target = "DIRECT"
	}
	ruleNode.Content = append(ruleNode.Content, scalar("MATCH,"+target))
	set(root, "mode", scalar("rule"))
	set(root, "rules", &ruleNode)
	used := map[string]bool{}
	for _, rule := range ruleNode.Content {
		parts := splitRule(rule.Value)
		if len(parts) > 2 && parts[0] == "RULE-SET" {
			used[parts[1]] = true
		}
	}
	for _, policyName := range []string{"nameserver-policy", "proxy-server-nameserver-policy"} {
		for i, key := range children(field(field(root, "dns"), policyName)) {
			if i%2 != 0 {
				continue
			}
			if names, ok := strings.CutPrefix(key.Value, "rule-set:"); ok {
				for _, name := range strings.Split(names, ",") {
					used[strings.TrimSpace(name)] = true
				}
			}
		}
	}
	for i := len(providers.Content) - 2; i >= 0; i -= 2 {
		key := providers.Content[i].Value
		if (strings.HasPrefix(key, "System_") || key == "Ads") && !used[key] {
			set(providers, key, nil)
		}
	}
	var meta yaml.Node
	_ = meta.Encode(settings)
	set(root, "x-manager", &meta)
	return encode(doc)
}

func categoryIDs() []string {
	result := make([]string, 0, len(Categories))
	for _, category := range Categories {
		if !category.Direct {
			result = append(result, category.ID)
		}
	}
	return result
}

func ManagedInfo(source string) (string, int) {
	doc, err := Parse(source)
	if err != nil {
		return "", 0
	}
	var settings managedSettings
	if meta := field(doc.Content[0], "x-manager"); meta != nil {
		_ = meta.Decode(&settings)
	}
	return settings.Preset, len(settings.Rules)
}

func SystemGroups() []string {
	names := []string{MainGroup, AutoGroup, FallbackGroup, DirectGroup, AdGroup, "国内网站", "国内 IP", "其他海外", "下载工具"}
	for _, category := range Categories {
		names = append(names, category.Name)
	}
	return names
}
