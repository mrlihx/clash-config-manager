package config

import (
	"fmt"
	"maps"
	"slices"

	"go.yaml.in/yaml/v3"
)

// 小火箭原生节点订阅保留 dialer-proxy；策略组展开为具体前置组合。
func ExportShadowrocket(source string) (string, error) {
	data, err := Validate(source)
	if err != nil {
		return "", err
	}
	nodes, _ := data["proxies"].([]any)
	groups, _ := data["proxy-groups"].([]any)
	byName := map[string]map[string]any{}
	reserved := map[string]bool{}
	groupNames := map[string]bool{}
	for _, raw := range groups {
		groupNames[raw.(map[string]any)["name"].(string)] = true
	}
	for _, raw := range append(slices.Clone(nodes), groups...) {
		item := raw.(map[string]any)
		name := item["name"].(string)
		byName[name] = item
		reserved[name] = true
	}
	output := []map[string]any{}
	resolved := map[string][]string{}
	var resolve func(string) ([]string, error)
	resolve = func(name string) ([]string, error) {
		if name == "DIRECT" {
			return []string{"DIRECT"}, nil
		}
		if names, ok := resolved[name]; ok {
			return names, nil
		}
		item := byName[name]
		if item == nil {
			return nil, fmt.Errorf("小火箭前置代理 %s 不是可导出的节点", name)
		}
		if groupNames[name] {
			if use, _ := item["use"].([]any); len(use) > 0 {
				return nil, fmt.Errorf("前置组 %s 的动态节点集合尚未展开", name)
			}
			names := []string{}
			membersList, _ := item["proxies"].([]any)
			for _, raw := range membersList {
				members, e := resolve(raw.(string))
				if e != nil {
					return nil, e
				}
				for _, member := range members {
					if !slices.Contains(names, member) {
						names = append(names, member)
					}
				}
			}
			if len(names) == 0 {
				return nil, fmt.Errorf("前置组 %s 没有可用成员", name)
			}
			resolved[name] = names
			return names, nil
		}
		fronts := []string{""}
		if ref, _ := item["dialer-proxy"].(string); ref != "" {
			fronts, err = resolve(ref)
			if err != nil {
				return nil, err
			}
		}
		names := []string{}
		for _, front := range fronts {
			node := maps.Clone(item)
			next := name
			if len(fronts) > 1 {
				base := name + " · 前置 " + front
				next = base
				for n := 2; reserved[next]; n++ {
					next = fmt.Sprintf("%s (%d)", base, n)
				}
				reserved[next] = true
			}
			node["name"] = next
			if front == "DIRECT" {
				delete(node, "dialer-proxy")
			} else if front != "" {
				node["dialer-proxy"] = front
			}
			// 这些协议在小火箭中由协议类型决定 TLS，避免携带 Clash 专用开关。
			if slices.Contains([]string{"trojan", "anytls", "hysteria2"}, fmt.Sprint(node["type"])) {
				delete(node, "tls")
			}
			if slices.Contains([]string{"vless", "vmess"}, fmt.Sprint(node["type"])) && node["sni"] != nil {
				node["servername"] = node["sni"]
				delete(node, "sni")
			}
			output = append(output, node)
			names = append(names, next)
		}
		resolved[name] = names
		return names, nil
	}
	for _, raw := range nodes {
		if _, err = resolve(raw.(map[string]any)["name"].(string)); err != nil {
			return "", err
		}
	}
	if len(output) == 0 {
		return "", fmt.Errorf("没有可导出的小火箭节点")
	}
	// 前置依赖解析不改变订阅节点的原始排列。
	exported := map[string]map[string]any{}
	for _, node := range output {
		exported[node["name"].(string)] = node
	}
	ordered := make([]map[string]any, 0, len(output))
	for _, raw := range nodes {
		for _, name := range resolved[raw.(map[string]any)["name"].(string)] {
			ordered = append(ordered, exported[name])
		}
	}
	body, err := yaml.Marshal(map[string]any{"proxies": ordered})
	return string(body), err
}
