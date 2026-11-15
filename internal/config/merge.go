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

func Merge(base, source string, selection map[string][]string) (string, error) {
	doc, err := Parse(base)
	if err != nil {
		return "", err
	}
	incoming, err := Parse(source)
	if err != nil {
		return "", err
	}
	root := doc.Content[0]
	for _, key := range sections {
		picked := selection[key]
		if len(picked) == 0 {
			continue
		}
		src := field(incoming.Content[0], key)
		if src == nil {
			return "", fmt.Errorf("来源配置缺少 %s", key)
		}
		dst := field(root, key)
		if dst == nil {
			dst = &yaml.Node{Kind: src.Kind, Tag: src.Tag}
			set(root, key, dst)
		}
		for _, name := range picked {
			if key == "rules" {
				i, err := strconv.Atoi(name)
				if err != nil || i < 0 || i >= len(src.Content) {
					return "", errors.New("合并规则索引无效")
				}
				rule := src.Content[i]
				if strings.TrimSpace(splitRule(rule.Value)[0]) == "MATCH" {
					return "", errors.New("合并以基准配置的 MATCH 为准")
				}
				if slices.ContainsFunc(dst.Content, func(n *yaml.Node) bool { return n.Value == rule.Value }) {
					continue
				}
				at := len(dst.Content)
				for j, n := range dst.Content {
					if strings.TrimSpace(splitRule(n.Value)[0]) == "MATCH" {
						at = j
						break
					}
				}
				dst.Content = slices.Insert(dst.Content, at, rule)
				continue
			}
			a, b := field(src, name), field(dst, name)
			if src.Kind == yaml.SequenceNode {
				a = slicesFind(src.Content, name)
				b = slicesFind(dst.Content, name)
			}
			if a == nil {
				return "", fmt.Errorf("来源配置缺少 %s", name)
			}
			if b != nil {
				var av, bv any
				if a.Decode(&av) != nil || b.Decode(&bv) != nil || !reflect.DeepEqual(av, bv) {
					return "", fmt.Errorf("%s 中 %s 同名但内容不同，请先调整名称", key, name)
				}
				continue
			}
			if dst.Kind == yaml.MappingNode {
				set(dst, name, a)
			} else {
				dst.Content = append(dst.Content, a)
			}
		}
	}
	return encode(doc)
}

func slicesFind(list []*yaml.Node, name string) *yaml.Node {
	for _, n := range list {
		if value(field(n, "name")) == name {
			return n
		}
	}
	return nil
}

// SelectedRules 使用当前配置的出口，自动带入选中规则所引用的规则集。
func SelectedRules(source string, selection map[string][]string, target string) (string, error) {
	doc, err := Parse(source)
	if err != nil {
		return "", err
	}
	rules := field(doc.Content[0], "rules")
	for _, key := range selection["rules"] {
		index, err := strconv.Atoi(key)
		if err != nil || rules == nil || index < 0 || index >= len(rules.Content) {
			return "", errors.New("外部规则索引无效")
		}
		rule := rules.Content[index]
		parts := splitRule(rule.Value)
		if len(parts) < 3 || parts[0] == "SUB-RULE" || parts[0] == "MATCH" {
			return "", errors.New("请选择普通匹配规则，子规则和 MATCH 不参与导入")
		}
		if parts[0] == "RULE-SET" && !slices.Contains(selection["rule-providers"], parts[1]) {
			selection["rule-providers"] = append(selection["rule-providers"], parts[1])
		}
		parts[targetIndex(parts)] = target
		rule.Value = strings.Join(parts, ",")
	}
	return encode(doc)
}
