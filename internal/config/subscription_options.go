package config

import (
	"errors"
	"slices"

	"go.yaml.in/yaml/v3"
)

type RuleOption struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Required bool   `json:"required"`
}

var RuleOptions = []RuleOption{
	{"China", "国内域名直连", false}, {"ChinaCIDR", "国内 IP 直连", false},
	{"Ads", "广告拦截", false}, {"Reject", "广告与跟踪拦截", false},
	{"Overseas", "海外域名", false}, {"GFW", "GFW 域名", false}, {"ForeignTLD", "海外顶级域名", false},
	{"TelegramCIDR", "Telegram IP", false},
}

var DefaultRuleSets = []string{}
var DefaultServices = []string{"Claude", "Gemini", "OpenAI", "GooglePlay", "YouTube", "Google", "Twitter", "Netflix", "TikTok"}

func ruleEnabled(settings managedSettings, id string) bool {
	return settings.RuleSets == nil || slices.Contains(*settings.RuleSets, id)
}

func SubscriptionOptions(source string, services, rules []string) (string, error) {
	if services == nil {
		services = []string{}
	}
	if rules == nil {
		rules = []string{}
	}
	for _, id := range services {
		if !slices.ContainsFunc(Categories, func(c RoutingCategory) bool { return c.ID == id && !c.Direct }) {
			return "", errors.New("分流组无效")
		}
	}

	doc, err := Parse(source)
	if err != nil {
		return "", err
	}
	var meta yaml.Node
	settings := managedSettings{Preset: "balanced", Ads: AdGroup}
	if old := field(doc.Content[0], "x-manager"); old != nil {
		if err := old.Decode(&settings); err != nil {
			return "", err
		}
	}
	settings.Services = &services
	settings.RuleSets = &rules
	if groups := field(doc.Content[0], "proxy-groups"); groups != nil {
		groups.Content = slices.DeleteFunc(groups.Content, func(g *yaml.Node) bool {
			name := value(field(g, "name"))
			return slices.ContainsFunc(Categories, func(c RoutingCategory) bool { return c.Name == name && (c.Direct || !slices.Contains(services, c.ID)) })
		})
	}
	_ = meta.Encode(settings)
	set(doc.Content[0], "x-manager", &meta)
	source, err = encode(doc)
	if err != nil {
		return "", err
	}
	return Generate(source, "balanced", false)
}

// ConfigureSubscriptionRouting 更新已发布订阅的系统分流，保留网络设置和已有组的策略参数。
func ConfigureSubscriptionRouting(source, template string, services, rules []string) (string, error) {
	result, err := SubscriptionOptions(source, services, rules)
	if err != nil {
		return "", err
	}
	doc, err := Parse(result)
	if err != nil {
		return "", err
	}
	original, err := Parse(source)
	if err != nil {
		return "", err
	}
	defaults, err := Parse(template)
	if err != nil {
		return "", err
	}
	root, old, base := doc.Content[0], original.Content[0], defaults.Content[0]
	providers := field(root, "rule-providers")
	for i, key := range children(field(base, "rule-providers")) {
		if i%2 != 0 {
			continue
		}
		existing := field(field(old, "rule-providers"), key.Value)
		if field(providers, key.Value) != nil {
			if existing != nil {
				set(providers, key.Value, existing)
			} else {
				set(providers, key.Value, field(field(base, "rule-providers"), key.Value))
			}
		} else if slices.Contains(rules, key.Value) {
			set(providers, key.Value, field(field(base, "rule-providers"), key.Value))
		}
	}
	for _, g := range children(field(root, "proxy-groups")) {
		name := value(field(g, "name"))
		if slicesFind(children(field(old, "proxy-groups")), name) != nil {
			continue
		}
		policy := slicesFind(children(field(base, "proxy-groups")), name)
		if policy == nil {
			continue
		}
		for i, key := range children(policy) {
			if i%2 == 0 && !slices.Contains([]string{"name", "proxies", "use"}, key.Value) {
				set(g, key.Value, policy.Content[i+1])
			}
		}
	}
	// 模板自定义规则集仅在选中时加入；服务规则由系统生成。
	entries := field(root, "rules")
	for _, rule := range children(field(base, "rules")) {
		parts := splitRule(rule.Value)
		if len(parts) < 3 || parts[0] != "RULE-SET" || !slices.Contains(rules, parts[1]) {
			continue
		}
		if slices.ContainsFunc(entries.Content, func(n *yaml.Node) bool { return n.Value == rule.Value }) {
			continue
		}
		at := len(entries.Content) - 1
		entries.Content = append(entries.Content[:at], append([]*yaml.Node{rule}, entries.Content[at:]...)...)
		if meta := field(root, "x-manager"); meta != nil {
			if managed := field(meta, "rules"); managed != nil {
				managed.Content = append(managed.Content, scalar(rule.Value))
			}
		}
	}
	// 手工规则维持在原有相邻系统规则之前，避免切换编辑方式改变优先级。
	oldRules := []string{}
	if meta := field(old, "x-manager"); meta != nil {
		for _, r := range children(field(meta, "rules")) {
			oldRules = append(oldRules, r.Value)
		}
	}
	originalRules := children(field(old, "rules"))
	manual := []string{}
	for _, r := range originalRules {
		if !slices.Contains(oldRules, r.Value) && splitRule(r.Value)[0] != "MATCH" {
			manual = append(manual, r.Value)
		}
	}
	entries.Content = slices.DeleteFunc(entries.Content, func(r *yaml.Node) bool { return slices.Contains(manual, r.Value) })
	for i, r := range originalRules {
		if !slices.Contains(manual, r.Value) {
			continue
		}
		at := len(entries.Content) - 1
		for _, following := range originalRules[i+1:] {
			if !slices.Contains(oldRules, following.Value) {
				continue
			}
			found := slices.IndexFunc(entries.Content, func(n *yaml.Node) bool { return n.Value == following.Value })
			if found >= 0 {
				at = found
				break
			}
		}
		entries.Content = append(entries.Content[:at], append([]*yaml.Node{r}, entries.Content[at:]...)...)
	}
	// 自定义兜底目标也保留；若引用已移除的组，交给保存校验报错。
	for _, r := range originalRules {
		if splitRule(r.Value)[0] == "MATCH" {
			entries.Content[len(entries.Content)-1] = r
			break
		}
	}
	result, err = encode(doc)
	if err != nil {
		return "", err
	}
	_, err = Validate(result)
	return result, err
}
