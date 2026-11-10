package config

import (
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

var topLevelSettings = []string{"port", "socks-port", "mixed-port", "redir-port", "tproxy-port", "allow-lan", "bind-address", "mode", "log-level", "ipv6", "external-controller", "profile", "dns", "sniffer"}

// 配置导出时将网络基础设置放在节点、策略组及规则前面。
func normalizeLayout(root *yaml.Node) {
	content := make([]*yaml.Node, 0, len(root.Content))
	selected := map[string]bool{}
	for _, key := range topLevelSettings {
		if node := field(root, key); node != nil {
			content = append(content, scalar(key), node)
			selected[key] = true
		}
	}
	for i := 0; i < len(root.Content); i += 2 {
		if !selected[root.Content[i].Value] {
			content = append(content, root.Content[i], root.Content[i+1])
		}
	}
	root.Content = content
	normalizeGroupEntries(root)
	if field(root, "x-manager") != nil && slicesFind(children(field(root, "proxy-groups")), AdGroup) != nil {
		for _, list := range []*yaml.Node{field(root, "rules"), field(field(root, "x-manager"), "rules")} {
			for _, rule := range children(list) {
				parts := strings.Split(rule.Value, ",")
				if len(parts) > 2 && parts[0] == "RULE-SET" && (parts[1] == "Ads" || parts[1] == "System_Reject") {
					parts[2] = AdGroup
					rule.Value = strings.Join(parts, ",")
				}
			}
		}
	}
	for i, entry := range children(field(root, "rule-providers")) {
		if i%2 == 1 && value(field(entry, "type")) == "http" {
			set(entry, "path", nil)
		}
	}

}

func NormalizeLayout(source string) (string, error) {
	doc, err := Parse(source)
	if err != nil {
		return "", err
	}
	normalizeLayout(doc.Content[0])
	return encode(doc)
}

// 基础组和前置依赖仅使用节点，业务分流组提供统一的手动/自动入口。
func normalizeGroupEntries(root *yaml.Node) {
	if field(root, "x-manager") == nil {
		return
	}
	groups := field(root, "proxy-groups")
	if groups == nil {
		return
	}
	if slicesFind(groups.Content, MainGroup) == nil || slicesFind(groups.Content, AutoGroup) == nil {
		return
	}
	front := map[string]bool{}
	var visit func(string)
	visit = func(name string) {
		if front[name] {
			return
		}
		group := slicesFind(groups.Content, name)
		if group == nil {
			return
		}
		front[name] = true
		for _, member := range children(field(group, "proxies")) {
			visit(member.Value)
		}
	}
	for _, node := range children(field(root, "proxies")) {
		visit(value(field(node, "dialer-proxy")))
	}
	fallback := slicesFind(groups.Content, FallbackGroup)
	if fallback == nil {
		// 故障转移沿用手动组节点，按顺序选择首个可用节点。
		fallback = &yaml.Node{}
		_ = fallback.Encode(map[string]any{"name": FallbackGroup, "type": "fallback", "url": "https://www.gstatic.com/generate_204", "interval": 60, "lazy": false})
		main := slicesFind(groups.Content, MainGroup)
		for _, key := range []string{"proxies", "use"} {
			if members := field(main, key); members != nil {
				var copied yaml.Node
				var data any
				_ = members.Decode(&data)
				_ = copied.Encode(data)
				set(fallback, key, &copied)
			}
		}
	}
	ordered := []*yaml.Node{slicesFind(groups.Content, MainGroup), slicesFind(groups.Content, AutoGroup), fallback}
	var advertising *yaml.Node
	for _, group := range groups.Content {
		name := value(field(group, "name"))
		if name == MainGroup || name == AutoGroup || name == FallbackGroup {
			continue
		}
		members := field(group, "proxies")
		if !front[name] && members != nil {
			remaining := slices.DeleteFunc(members.Content, func(n *yaml.Node) bool { return n.Value == MainGroup || n.Value == AutoGroup })
			if name == AdGroup {
				members.Content = []*yaml.Node{scalar("REJECT")}
			} else {
				members.Content = append([]*yaml.Node{scalar(MainGroup), scalar(AutoGroup)}, remaining...)
			}
		}
		if name == AdGroup {
			advertising = group
		} else {
			ordered = append(ordered, group)
		}
	}
	if advertising != nil {
		ordered = append(ordered, advertising)
	}
	groups.Content = ordered
}
