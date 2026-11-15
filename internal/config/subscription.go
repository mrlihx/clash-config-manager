package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v3"
)

// SubscriptionYAML 保留完整 Clash 配置，并将支持的分享链接列表转换为可编辑配置。
func SubscriptionYAML(source string) (string, error) {
	source = strings.TrimPrefix(source, "\ufeff")
	if doc, err := Parse(source); err == nil {
		for _, key := range sections {
			if field(doc.Content[0], key) != nil {
				return source, nil
			}
		}
		return "", errors.New("订阅响应没有 Clash 配置条目")
	}
	source = strings.TrimSpace(source)
	if !strings.Contains(source, "://") {
		compact := strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, source)
		for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
			decoded, err := encoding.DecodeString(compact)
			if err == nil {
				source = string(decoded)
				break
			}
		}
	}
	if !strings.Contains(source, "://") {
		return "", errors.New("订阅响应不是 Clash YAML 或受支持的节点链接列表")
	}
	var proxies []map[string]any
	var names []string
	used := map[string]bool{}
	for i, line := range strings.Split(source, "\n") {
		link := strings.TrimSpace(line)
		if link == "" {
			continue
		}
		node, err := NodeLink(link)
		if err != nil {
			return "", fmt.Errorf("订阅第 %d 个链接无法导入。%w", i+1, err)
		}
		base := node["name"].(string)
		name := base
		for n := 2; used[name]; n++ {
			name = fmt.Sprintf("%s (%d)", base, n)
		}
		used[name] = true
		node["name"] = name
		proxies = append(proxies, node)
		names = append(names, name)
	}
	group := "🚀 手动选择"
	for used[group] {
		group += " · 订阅"
	}
	data := map[string]any{
		"mode": "rule", "proxies": proxies,
		"proxy-groups": []map[string]any{{"name": group, "type": "select", "proxies": names}},
		"rules":        []string{"MATCH," + group},
	}
	out, err := yaml.Marshal(data)
	return string(out), err
}
