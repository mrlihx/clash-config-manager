package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

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
