package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

var legacySnapshotSuffix = regexp.MustCompile(`(?: · 订阅自有节点(?: \(\d+\))?)+$`)

var sections = []string{"proxies", "proxy-groups", "rules", "rule-providers", "proxy-providers"}

func Parse(source string) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(strings.NewReader(source))
	var doc, extra yaml.Node
	if err := decoder.Decode(&doc); err != nil || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("请输入有效的 YAML 配置对象")
	}
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("配置必须是单个 YAML 文档")
	}
	var values map[string]any
	if err := doc.Decode(&values); err != nil {
		return nil, errors.New("YAML 包含重复键或无法解析的字段")
	}
	for _, key := range sections {
		n := field(doc.Content[0], key)
		if n == nil {
			continue
		}
		kind := yaml.SequenceNode
		if strings.HasSuffix(key, "providers") {
			kind = yaml.MappingNode
		}
		if n.Kind != kind {
			return nil, fmt.Errorf("%s 的 YAML 结构不正确", key)
		}
	}
	// 旧版地区后缀统一为通用前置组名称，并同步节点和规则引用。
	root := doc.Content[0]
	for _, group := range children(field(root, "proxy-groups")) {
		name := value(field(group, "name"))
		if strings.Contains(name, "链式前置") && strings.HasSuffix(name, "-新加坡") {
			next := strings.TrimSuffix(name, "-新加坡")
			if slicesFind(children(field(root, "proxy-groups")), next) == nil {
				set(group, "name", scalar(next))
				references(root, "proxy-groups", name, next)
			}
		}
	}
	// 清理旧版本合并快照时生成的来源后缀，同时修正前置和策略组引用。
	for _, section := range []string{"proxies", "proxy-groups"} {
		list := field(root, section)
		for i := 0; i < len(children(list)); i++ {
			node := list.Content[i]
			old := value(field(node, "name"))
			base := legacySnapshotSuffix.ReplaceAllString(old, "")
			if base == old || base == "" {
				continue
			}
			next := base
			existing := slicesFind(list.Content, next)
			if existing != nil {
				var a, b map[string]any
				_ = node.Decode(&a)
				_ = existing.Decode(&b)
				delete(a, "name")
				delete(b, "name")
				if reflect.DeepEqual(a, b) {
					references(root, section, old, next)
					list.Content = append(list.Content[:i], list.Content[i+1:]...)
					i--
					continue
				}
				for n := 2; slicesFind(list.Content, next) != nil; n++ {
					next = fmt.Sprintf("%s (%d)", base, n)
				}
			}
			set(node, "name", scalar(next))
			references(root, section, old, next)
		}
	}
	return &doc, nil
}

func field(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func value(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	return n.Value
}

func set(n *yaml.Node, key string, v *yaml.Node) {
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			if v == nil {
				n.Content = slices.Delete(n.Content, i, i+2)
			} else {
				n.Content[i+1] = v
			}
			return
		}
	}
	if v != nil {
		n.Content = append(n.Content, scalar(key), v)
	}
}

func scalar(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }
func children(n *yaml.Node) []*yaml.Node {
	if n == nil {
		return nil
	}
	return n.Content
}

func encode(doc *yaml.Node) (string, error) {
	normalizeLayout(doc.Content[0])
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	return out.String(), enc.Close()
}
