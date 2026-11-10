package config

import (
	"go.yaml.in/yaml/v3"
	"slices"
)

// SyncNodes 只替换上次同步的节点及前置依赖，保留手工维护的规则和策略参数。
func SyncNodes(current, previous, next string) (string, error) {
	doc, err := Parse(current)
	if err != nil {
		return "", err
	}
	old, err := Parse(previous)
	if err != nil {
		return "", err
	}
	fresh, err := Parse(next)
	if err != nil {
		return "", err
	}
	root := doc.Content[0]
	oldRoot, newRoot := old.Content[0], fresh.Content[0]
	oldNames, newNames := []string{}, []string{}
	for _, n := range children(field(oldRoot, "proxies")) {
		oldNames = append(oldNames, value(field(n, "name")))
	}
	for _, n := range children(field(newRoot, "proxies")) {
		newNames = append(newNames, value(field(n, "name")))
	}
	for _, section := range []string{"proxies", "proxy-groups"} {
		target := field(root, section)
		if target == nil {
			if field(newRoot, section) == nil {
				continue
			}
			target = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			set(root, section, target)
		}
		before, after := field(oldRoot, section), field(newRoot, section)
		for i := 0; i < len(target.Content); i++ {
			n := target.Content[i]
			name := value(field(n, "name"))
			owned := slicesFind(children(before), name)
			if owned == nil || (section == "proxy-groups" && slices.Contains(SystemGroups(), name)) {
				continue
			}
			replacement := slicesFind(children(after), name)
			if replacement != nil {
				var c, o, n any
				_ = target.Content[i].Decode(&c)
				_ = owned.Decode(&o)
				_ = replacement.Decode(&n)
				var merged yaml.Node
				_ = merged.Encode(mergeRoutingValue(c, o, n))
				target.Content[i] = &merged
			} else {
				target.Content = append(target.Content[:i], target.Content[i+1:]...)
				i--
			}
		}
		for _, n := range children(after) {
			name := value(field(n, "name"))
			if section == "proxy-groups" && slices.Contains(SystemGroups(), name) {
				continue
			}
			if slicesFind(target.Content, name) == nil {
				target.Content = append(target.Content, n)
			}
		}
	}
	for _, g := range children(field(root, "proxy-groups")) {
		members := field(g, "proxies")
		if members == nil {
			continue
		}
		all := len(oldNames) > 0
		for _, name := range oldNames {
			if !slices.ContainsFunc(members.Content, func(n *yaml.Node) bool { return n.Value == name }) {
				all = false
			}
		}
		members.Content = slices.DeleteFunc(members.Content, func(n *yaml.Node) bool {
			return slices.Contains(oldNames, n.Value) && !slices.Contains(newNames, n.Value)
		})
		if all {
			for _, name := range newNames {
				if !slices.ContainsFunc(members.Content, func(n *yaml.Node) bool { return n.Value == name }) {
					members.Content = append(members.Content, scalar(name))
				}
			}
		}
		if len(members.Content) == 0 && field(g, "use") == nil {
			members.Content = append(members.Content, scalar("DIRECT"))
		}
	}
	result, err := encode(doc)
	if err != nil {
		return "", err
	}
	_, err = Validate(result)
	return result, err
}
