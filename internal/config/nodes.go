package config

import (
	"errors"
	"fmt"
	"slices"

	"go.yaml.in/yaml/v3"
)

// NodeSource 只保留节点，以及链式代理实际引用的组。
func NodeSource(source string) (string, error) {
	doc, err := Parse(source)
	if err != nil {
		return "", err
	}
	original := doc.Content[0]
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	proxies := field(original, "proxies")
	if proxies == nil {
		proxies = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	}
	set(root, "proxies", proxies)
	set(root, "proxy-providers", field(original, "proxy-providers"))
	required := map[string]bool{}
	groups := field(original, "proxy-groups")
	var visit func(string)
	visit = func(name string) {
		if required[name] {
			return
		}
		required[name] = true
		if g := slicesFind(children(groups), name); g != nil {
			for _, member := range children(field(g, "proxies")) {
				visit(member.Value)
			}
		}
	}
	for _, node := range children(proxies) {
		if ref := value(field(node, "dialer-proxy")); ref != "" {
			visit(ref)
		}
	}
	kept := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, group := range children(groups) {
		if required[value(field(group, "name"))] {
			kept.Content = append(kept.Content, group)
		}
	}
	if len(kept.Content) > 0 {
		set(root, "proxy-groups", kept)
	}
	doc.Content[0] = root
	return encode(doc)
}

type NodeSelection struct {
	Name   string
	Source string
	Nodes  []string
}

// CollectNodes 汇集勾选节点及其前置依赖。同名节点按来源重命名并同步引用。
func CollectNodes(selections []NodeSelection) (string, error) {
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	proxies := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	groups := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	set(root, "proxies", proxies)
	set(root, "proxy-groups", groups)
	used := map[string]bool{}
	for _, name := range SystemGroups() {
		used[name] = true
	}
	for _, selection := range selections {
		doc, err := Parse(selection.Source)
		if err != nil {
			return "", err
		}
		src := doc.Content[0]
		nodes := field(src, "proxies")
		srcGroups := field(src, "proxy-groups")
		included := map[string]bool{}
		var visit func(string) error
		visit = func(name string) error {
			if slices.Contains([]string{"DIRECT", "REJECT", "PASS", "REJECT-DROP"}, name) {
				return nil
			}
			if included[name] {
				return nil
			}
			included[name] = true
			if node := slicesFind(children(nodes), name); node != nil {
				if dependency := value(field(node, "dialer-proxy")); dependency != "" {
					return visit(dependency)
				}
				return nil
			}
			if group := slicesFind(children(srcGroups), name); group != nil {
				if len(children(field(group, "use"))) > 0 {
					return errors.New("前置组引用了动态代理集合，请先导入为具体节点")
				}
				for _, member := range children(field(group, "proxies")) {
					if err := visit(member.Value); err != nil {
						return err
					}
				}
				return nil
			}
			return fmt.Errorf("节点或前置依赖 %s 不存在", name)
		}
		for _, name := range selection.Nodes {
			if slicesFind(children(nodes), name) == nil {
				return "", fmt.Errorf("%s 中的节点 %s 已不存在", selection.Name, name)
			}
			if err := visit(name); err != nil {
				return "", err
			}
		}
		type entry struct {
			node    *yaml.Node
			section string
			old     string
		}
		entries := []entry{}
		for _, section := range []string{"proxies", "proxy-groups"} {
			for _, node := range children(field(src, section)) {
				name := value(field(node, "name"))
				if included[name] {
					entries = append(entries, entry{node, section, name})
				}
			}
		}
		for _, entry := range entries {
			name := entry.old
			if used[name] {
				name = entry.old + " · " + selection.Name
			}
			for n := 2; used[name]; n++ {
				name = fmt.Sprintf("%s · %s (%d)", entry.old, selection.Name, n)
			}
			used[name] = true
			if name != entry.old {
				references(src, entry.section, entry.old, name)
				set(entry.node, "name", scalar(name))
			}
		}
		for _, entry := range entries {
			// Decode 后重新编码，避免引用来源文件里未带入的 YAML 锚点。
			var data map[string]any
			if err := entry.node.Decode(&data); err != nil {
				return "", err
			}
			var node yaml.Node
			_ = node.Encode(data)
			if entry.section == "proxies" {
				proxies.Content = append(proxies.Content, &node)
			} else {
				groups.Content = append(groups.Content, &node)
			}
		}
	}
	if len(proxies.Content) == 0 {
		return "", errors.New("请至少勾选一个节点")
	}
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	source, err := encode(doc)
	if err != nil {
		return "", err
	}
	// 基础网络配置与规则均由系统模板创建。
	return Generate(source, "balanced", true)
}
