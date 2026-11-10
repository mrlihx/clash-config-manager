package config

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Edit struct {
	Target      string    `json:"target"`
	PayloadText *string   `json:"payloadText"`
	Section     string    `json:"section"`
	Key         string    `json:"key"`
	Value       any       `json:"value"`
	Delete      bool      `json:"delete"`
	Groups      *[]string `json:"groups"`
	Move        *int      `json:"move"`
	Position    *int      `json:"position"`
}

func references(root *yaml.Node, section, old, next string) {
	if old == "" || old == next {
		return
	}
	if section == "proxies" || section == "proxy-groups" {
		for _, group := range children(field(root, "proxy-groups")) {
			for _, member := range children(field(group, "proxies")) {
				if member.Value == old {
					member.Value = next
				}
			}
		}
		for _, node := range children(field(root, "proxies")) {
			if ref := field(node, "dialer-proxy"); value(ref) == old {
				ref.Value = next
			}
		}
	}
	if section == "proxies" || section == "proxy-groups" {
		dns := field(root, "dns")
		for i := 1; i < len(children(dns)); i += 2 {
			replaceResolverProxy(dns.Content[i], old, next)
		}
	}
	if section == "proxy-providers" {
		for _, group := range children(field(root, "proxy-groups")) {
			for _, member := range children(field(group, "use")) {
				if member.Value == old {
					member.Value = next
				}
			}
		}
	}
	for _, list := range append([]*yaml.Node{field(root, "rules"), field(field(root, "x-manager"), "rules")}, subRuleLists(root)...) {
		for _, rule := range children(list) {
			p := splitRule(rule.Value)
			i := targetIndex(p)
			switch {
			case (section == "proxies" || section == "proxy-groups") && i > 0 && strings.TrimSpace(p[i]) == old:
				p[i] = next
			case section == "rule-providers" && len(p) > 2 && strings.TrimSpace(p[0]) == "RULE-SET" && strings.TrimSpace(p[1]) == old:
				p[1] = next
			default:
				continue
			}
			rule.Value = strings.Join(p, ",")
		}
	}
	if section == "rule-providers" {
		dns := field(root, "dns")
		for _, name := range []string{"nameserver-policy", "proxy-server-nameserver-policy"} {
			policy := field(dns, name)
			entries := children(policy)
			for i := 0; i < len(entries); i += 2 {
				key := entries[i]
				if !strings.HasPrefix(key.Value, "rule-set:") {
					continue
				}
				names := strings.Split(strings.TrimPrefix(key.Value, "rule-set:"), ",")
				for j, n := range names {
					if strings.TrimSpace(n) == old {
						names[j] = next
					}
				}
				key.Value = "rule-set:" + strings.Join(names, ",")
			}
		}
	}
}

func ApplyEdit(source string, e Edit) (string, error) {
	if !slices.Contains(sections, e.Section) {
		return "", errors.New("配置区段无效")
	}
	doc, err := Parse(source)
	if err != nil {
		return "", err
	}
	root := doc.Content[0]
	list := field(root, e.Section)
	keyed := strings.HasSuffix(e.Section, "providers")
	if list == nil {
		list = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		if keyed {
			list.Kind = yaml.MappingNode
			list.Tag = "!!map"
		}
		set(root, e.Section, list)
	}
	index := -1
	if e.Key != "" {
		for i, n := range list.Content {
			match := false
			switch {
			case keyed:
				match = i%2 == 0 && n.Value == e.Key
			case e.Section == "rules":
				match = strconv.Itoa(i) == e.Key
			default:
				match = value(field(n, "name")) == e.Key
			}
			if match {
				index = i
				break
			}
		}
		if index < 0 {
			return "", errors.New("该条目不存在，请刷新后重试")
		}
	}
	if e.Delete {
		if index < 0 {
			return "", errors.New("请选择要删除的条目")
		}
		if e.Section == "proxies" && field(root, "x-manager") != nil {
			for _, group := range children(field(root, "proxy-groups")) {
				members := field(group, "proxies")
				if members == nil {
					continue
				}
				members.Content = slices.DeleteFunc(members.Content, func(n *yaml.Node) bool { return n.Value == e.Key })
				if len(members.Content) == 0 && len(children(field(group, "use"))) == 0 {
					members.Content = append(members.Content, scalar("DIRECT"))
				}
			}
		}
		width := 1
		if keyed {
			width = 2
		}
		list.Content = slices.Delete(list.Content, index, index+width)
		output, err := encode(doc)
		if err != nil {
			return "", err
		}
		if _, err = Validate(output); err != nil {
			return "", fmt.Errorf("删除前请先处理引用。%w", err)
		}
		return output, nil
	}
	if e.Move != nil {
		if e.Section != "rules" || index < 0 || *e.Move < 0 || *e.Move >= len(list.Content) {
			return "", errors.New("规则位置无效")
		}
		n := list.Content[index]
		list.Content = slices.Insert(slices.Delete(list.Content, index, index+1), *e.Move, n)
		return encode(doc)
	}
	if e.Section == "rules" {
		rule, ok := e.Value.(string)
		if !ok || strings.TrimSpace(rule) == "" {
			return "", errors.New("请输入规则")
		}
		if index < 0 {
			index = len(list.Content)
			for i, n := range list.Content {
				if strings.TrimSpace(splitRule(n.Value)[0]) == "MATCH" {
					index = i
					break
				}
			}
			if e.Position != nil {
				if *e.Position < 0 || *e.Position > len(list.Content) {
					return "", errors.New("插入位置无效")
				}
				index = *e.Position
			}
			list.Content = slices.Insert(list.Content, index, scalar(rule))
		} else {
			list.Content[index].Value = rule
			if e.Position != nil {
				if *e.Position < 0 || *e.Position >= len(list.Content) {
					return "", errors.New("规则位置无效")
				}
				node := list.Content[index]
				list.Content = slices.Insert(slices.Delete(list.Content, index, index+1), *e.Position, node)
			}
		}
		return encode(doc)
	}
	fields, ok := e.Value.(map[string]any)
	if !ok {
		return "", errors.New("条目内容必须是对象")
	}
	name, _ := fields["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("名称不能为空")
	}
	for i, n := range list.Content {
		if i == index {
			continue
		}
		if (keyed && i%2 == 0 && n.Value == name) || (!keyed && value(field(n, "name")) == name) {
			return "", errors.New("名称已存在")
		}
	}
	var n *yaml.Node
	if index < 0 {
		n = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		if keyed {
			list.Content = append(list.Content, scalar(name))
		}
		list.Content = append(list.Content, n)
	} else if keyed {
		list.Content[index].Value = name
		n = list.Content[index+1]
	} else {
		n = list.Content[index]
	}
	if n.Kind != yaml.MappingNode {
		return "", errors.New("请在 YAML 编辑器中修改该条目")
	}
	if !keyed {
		set(n, "name", scalar(name))
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		v := fields[k]
		if k == "name" {
			continue
		}
		if v == nil {
			set(n, k, nil)
			continue
		}
		var node yaml.Node
		if err := node.Encode(v); err != nil {
			return "", err
		}
		var previous any
		if old := field(n, k); old != nil && old.Decode(&previous) == nil && equalValue(previous, v) {
			continue
		}
		set(n, k, &node)
	}
	references(root, e.Section, e.Key, name)
	if e.Section == "proxies" && e.Groups != nil {
		remaining := slices.Clone(*e.Groups)
		for _, g := range children(field(root, "proxy-groups")) {
			groupName := value(field(g, "name"))
			selected := slices.Contains(*e.Groups, groupName)
			remaining = slices.DeleteFunc(remaining, func(v string) bool { return v == groupName })
			members := field(g, "proxies")
			if members == nil && selected {
				members = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
				set(g, "proxies", members)
			}
			if members == nil {
				continue
			}
			contains := slices.ContainsFunc(members.Content, func(n *yaml.Node) bool { return n.Value == name })
			if selected && !contains {
				members.Content = append(members.Content, scalar(name))
			}
			if !selected && contains {
				members.Content = slices.DeleteFunc(members.Content, func(n *yaml.Node) bool { return n.Value == name })
			}
		}
		if len(remaining) > 0 {
			return "", errors.New("部分目标策略组已不存在")
		}
	}
	return encode(doc)
}

func equalValue(a, b any) bool {
	if reflect.DeepEqual(a, b) {
		return true
	}
	if number, ok := b.(float64); ok {
		if i, ok := a.(int); ok {
			return float64(i) == number
		}
	}
	return false
}

// DNS 服务器的 #代理组 后缀也会引用策略名称。
func replaceResolverProxy(n *yaml.Node, old, next string) {
	if n.Kind == yaml.ScalarNode {
		host, fragment, ok := strings.Cut(n.Value, "#")
		if !ok {
			return
		}
		group, suffix, hasSuffix := strings.Cut(fragment, "&")
		if group == old {
			n.Value = host + "#" + next
			if hasSuffix {
				n.Value += "&" + suffix
			}
		}
		return
	}
	if n.Kind == yaml.MappingNode {
		for i := 1; i < len(n.Content); i += 2 {
			replaceResolverProxy(n.Content[i], old, next)
		}
	} else if n.Kind == yaml.SequenceNode {
		for _, child := range n.Content {
			replaceResolverProxy(child, old, next)
		}
	}
}
