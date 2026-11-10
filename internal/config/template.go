package config

import (
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ApplyRoutingTemplate 将模板中的规则顺序、人工修改和更新周期应用到新订阅。
func ApplyRoutingTemplate(source, template string, selected []string) (string, error) {
	doc, err := Parse(source)
	if err != nil {
		return "", err
	}
	defaults, err := Parse(template)
	if err != nil {
		return "", err
	}
	root := doc.Content[0]
	base := defaults.Content[0]
	copyMembers := func() *yaml.Node {
		var members yaml.Node
		var names []string
		_ = field(slicesFind(children(field(root, "proxy-groups")), MainGroup), "proxies").Decode(&names)
		_ = members.Encode(names)
		return &members
	}

	for i, key := range base.Content {
		if i%2 == 0 && !slices.Contains([]string{"proxies", "proxy-groups", "rule-providers", "proxy-providers", "rules", "x-manager"}, key.Value) {
			set(root, key.Value, base.Content[i+1])
		}
	}
	// 自定义组作为方案的一部分，默认使用订阅节点；服务组仍由订阅选项控制。
	for _, group := range children(field(base, "proxy-groups")) {
		name := value(field(group, "name"))
		if slices.Contains(SystemGroups(), name) || slicesFind(children(field(root, "proxy-groups")), name) != nil {
			continue
		}
		var copied yaml.Node
		var data map[string]any
		_ = group.Decode(&data)
		_ = copied.Encode(data)
		set(&copied, "proxies", copyMembers())
		field(root, "proxy-groups").Content = append(field(root, "proxy-groups").Content, &copied)
	}

	providers := field(root, "rule-providers")
	baseProviders := field(base, "rule-providers")
	for i, key := range children(baseProviders) {
		if i%2 != 0 {
			continue
		}
		if field(providers, key.Value) != nil || slices.Contains(selected, key.Value) {
			set(providers, key.Value, field(baseProviders, key.Value))
		}
	}
	for _, group := range children(field(root, "proxy-groups")) {
		policy := slicesFind(children(field(base, "proxy-groups")), value(field(group, "name")))
		if policy == nil {
			continue
		}
		for i, key := range children(policy) {
			if i%2 != 0 || slices.Contains([]string{"name", "proxies", "use"}, key.Value) {
				continue
			}
			set(group, key.Value, policy.Content[i+1])
		}
		if value(field(group, "name")) == AdGroup {
			set(group, "proxies", field(policy, "proxies"))
		}
	}
	rules := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, rule := range children(field(base, "rules")) {
		parts := splitRule(rule.Value)
		if len(parts) > 2 && parts[0] == "RULE-SET" && field(providers, parts[1]) == nil {
			continue
		}
		if target := parts[targetIndex(parts)]; target != "DIRECT" && target != "REJECT" && target != "PASS" && target != "REJECT-DROP" && slicesFind(children(field(root, "proxy-groups")), target) == nil {
			policy := slicesFind(children(field(base, "proxy-groups")), target)
			if policy == nil {
				continue
			}
			var copied yaml.Node
			var data map[string]any
			_ = policy.Decode(&data)
			_ = copied.Encode(data)
			set(&copied, "proxies", copyMembers())
			field(root, "proxy-groups").Content = append(field(root, "proxy-groups").Content, &copied)
		}
		rules.Content = append(rules.Content, rule)
	}
	set(root, "rules", rules)
	if field(providers, "Custom_Direct_Domain") != nil {
		policy := field(field(root, "dns"), "nameserver-policy")
		for i, key := range children(policy) {
			if i%2 != 0 {
				continue
			}
			names, ok := strings.CutPrefix(key.Value, "rule-set:")
			if ok && strings.Contains(names, "System_China") && !strings.Contains(names, "Custom_Direct_Domain") {
				key.Value += ",Custom_Direct_Domain"
			}
		}
	}
	if meta := field(root, "x-manager"); meta != nil {
		set(meta, "rules", rules)
	}
	pruneChinaDNS(root)
	return encode(doc)
}
