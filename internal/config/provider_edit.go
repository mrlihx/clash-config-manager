package config

import (
	"errors"
	"fmt"
	"go.yaml.in/yaml/v3"
	"net/netip"
	"slices"
	"strings"
)

// RuleText 接受逐行文本、YAML 列表或 payload 对象；策略由规则集统一指定。
func RuleText(text, behavior string) ([]string, error) {
	var input any
	var lines []string
	if err := yaml.Unmarshal([]byte(text), &input); err == nil {
		if object, ok := input.(map[string]any); ok {
			input = object["payload"]
		}
		if items, ok := input.([]any); ok {
			for i, item := range items {
				value, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("第 %d 条规则必须是文本", i+1)
				}
				lines = append(lines, value)
			}
		}
	}
	if lines == nil {
		lines = strings.Split(text, "\n")
	}
	result := []string{}
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch behavior {
		case "domain":
			if strings.ContainsAny(line, ", \t") {
				return nil, fmt.Errorf("第 %d 行请填写域名；DOMAIN 等完整匹配条件请选择综合规则", i+1)
			}
		case "ipcidr":
			if _, err := netip.ParsePrefix(line); err != nil {
				return nil, fmt.Errorf("第 %d 行不是有效的 IP 网段", i+1)
			}
		case "classical":
			parts := splitRule(line)
			if len(parts) < 2 {
				return nil, fmt.Errorf("第 %d 行缺少匹配条件", i+1)
			}
			if slices.Contains([]string{"DIRECT", "REJECT", "PASS", "REJECT-DROP"}, parts[len(parts)-1]) {
				return nil, fmt.Errorf("第 %d 行不需要填写目标策略，请在下方统一选择", i+1)
			}
			if slices.Contains([]string{"DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "DST-PORT", "SRC-PORT", "NETWORK"}, parts[0]) && len(parts) != 2 {
				return nil, fmt.Errorf("第 %d 行只能包含匹配类型和条件", i+1)
			}
			if parts[0] == "IP-CIDR" || parts[0] == "IP-CIDR6" {
				if _, err := netip.ParsePrefix(parts[1]); err != nil {
					return nil, fmt.Errorf("第 %d 行 IP 网段无效", i+1)
				}
			}
		default:
			return nil, errors.New("请选择有效的规则匹配类型")
		}
		result = append(result, line)
	}
	if len(result) == 0 {
		return nil, errors.New("请至少填写一条规则")
	}
	return result, nil
}

// ApplyRuleProvider 同时保存规则集和引用，避免定义后没有规则实际使用。
func ApplyRuleProvider(source string, e Edit) (string, error) {
	original, err := Parse(source)
	if err != nil {
		return "", err
	}
	name := e.Key
	if value, ok := e.Value.(map[string]any); ok {
		if entered, ok := value["name"].(string); ok {
			name = entered
		}
		if value["type"] == "inline" {
			if e.PayloadText != nil {
				payload, err := RuleText(*e.PayloadText, fmt.Sprint(value["behavior"]))
				if err != nil {
					return "", err
				}
				value["payload"] = payload
			}
			value["format"] = "yaml"
			value["url"] = nil
			value["interval"] = nil
			value["path"] = nil
		}
	}
	if e.Key == "" && strings.HasPrefix(name, "System_") {
		return "", errors.New("System_ 前缀由系统保留，请使用其他名称")
	}
	root := original.Content[0]
	if e.Delete {
		// 一并移除系统生成的引用；其他需要此规则集的手工配置仍由校验提示。
		for _, list := range []*yaml.Node{field(root, "rules"), field(field(root, "x-manager"), "rules")} {
			if list != nil {
				list.Content = slices.DeleteFunc(list.Content, func(n *yaml.Node) bool {
					p := splitRule(n.Value)
					return len(p) > 2 && p[0] == "RULE-SET" && p[1] == name
				})
			}
		}
		source, err = encode(original)
		if err != nil {
			return "", err
		}
	}
	updated, err := ApplyEdit(source, e)
	if err != nil {
		return "", err
	}
	if e.Delete {
		return updated, nil
	}
	doc, err := Parse(updated)
	if err != nil {
		return "", err
	}
	root = doc.Content[0]
	target := e.Target
	if target == "" {
		for _, rule := range children(field(root, "rules")) {
			p := splitRule(rule.Value)
			if len(p) > 2 && p[0] == "RULE-SET" && p[1] == name {
				target = p[targetIndex(p)]
				break
			}
		}
	}
	if target == "" {
		target = MainGroup
	}
	if name == "Ads" || name == "System_Reject" {
		target = AdGroup
	}
	provider := field(field(root, "rule-providers"), name)
	rule := "RULE-SET," + name + "," + target
	if value(field(provider, "behavior")) == "ipcidr" {
		rule += ",no-resolve"
	}
	list := field(root, "rules")
	if list == nil {
		list = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		set(root, "rules", list)
	}
	at := -1
	for i, n := range list.Content {
		p := splitRule(n.Value)
		if len(p) > 2 && p[0] == "RULE-SET" && p[1] == name {
			at = i
			break
		}
	}
	list.Content = slices.DeleteFunc(list.Content, func(n *yaml.Node) bool {
		p := splitRule(n.Value)
		return len(p) > 2 && p[0] == "RULE-SET" && p[1] == name
	})
	if e.Position != nil {
		at = *e.Position
	}
	if at < 0 {
		at = len(list.Content)
		for i, n := range list.Content {
			p := splitRule(n.Value)
			if p[0] == "MATCH" || (len(p) > 2 && p[0] == "RULE-SET" && strings.HasPrefix(p[1], "System_") && !slices.Contains([]string{"System_Private", "System_LanCIDR", "System_SteamCDN"}, p[1])) {
				at = i
				break
			}
		}
	}
	at = min(max(at, 0), len(list.Content))
	list.Content = slices.Insert(list.Content, at, scalar(rule))
	if meta := field(root, "x-manager"); meta != nil {
		tracked := field(meta, "rules")
		if tracked != nil {
			tracked.Content = slices.DeleteFunc(tracked.Content, func(n *yaml.Node) bool {
				p := splitRule(n.Value)
				return len(p) > 2 && p[0] == "RULE-SET" && p[1] == name
			})
			tracked.Content = append(tracked.Content, scalar(rule))
		}
	}
	updated, err = encode(doc)
	if err != nil {
		return "", err
	}
	_, err = Validate(updated)
	return updated, err
}

func BindUnreferencedRuleProviders(source string) (string, error) {
	doc, err := Parse(source)
	if err != nil {
		return "", err
	}
	root := doc.Content[0]
	for i, key := range children(field(root, "rule-providers")) {
		if i%2 != 0 || strings.HasPrefix(key.Value, "System_") || key.Value == "Ads" {
			continue
		}
		referenced := slices.ContainsFunc(children(field(root, "rules")), func(rule *yaml.Node) bool {
			p := splitRule(rule.Value)
			return len(p) > 2 && p[0] == "RULE-SET" && p[1] == key.Value
		})
		if referenced {
			continue
		}
		source, err = ApplyRuleProvider(source, Edit{Section: "rule-providers", Key: key.Value, Value: map[string]any{"name": key.Value}, Target: MainGroup})
		if err != nil {
			return "", err
		}
	}
	return source, nil
}
