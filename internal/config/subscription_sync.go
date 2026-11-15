package config

import (
	"slices"
	"strings"
)

// 关联订阅只输出当前来源中的勾选节点及依赖，不保留历史节点快照。
func SyncSubscriptionNodes(current, next string) (string, error) {
	doc, err := Parse(current)
	if err != nil {
		return "", err
	}
	fresh, err := Parse(next)
	if err != nil {
		return "", err
	}
	snapshot, err := NodeSource(current)
	if err != nil {
		return "", err
	}
	old, err := Parse(snapshot)
	if err != nil {
		return "", err
	}
	removed := []string{}
	for _, section := range []string{"proxies", "proxy-groups"} {
		for _, node := range children(field(old.Content[0], section)) {
			name := value(field(node, "name"))
			if slicesFind(children(field(fresh.Content[0], section)), name) == nil {
				removed = append(removed, name)
			}
		}
	}
	// 已删除节点对应的专属规则停止放行，避免清理失败后继续分发旧快照。
	for _, rule := range children(field(doc.Content[0], "rules")) {
		parts := splitRule(rule.Value)
		at := targetIndex(parts)
		if at > 0 && slices.Contains(removed, parts[at]) {
			parts[at] = "REJECT"
			rule.Value = strings.Join(parts, ",")
		}
	}
	source, err := encode(doc)
	if err != nil {
		return "", err
	}
	return SyncNodes(source, snapshot, next)
}
