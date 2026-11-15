package config

import (
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

func Validate(source string) (map[string]any, error) {
	doc, err := Parse(source)
	if err != nil {
		return nil, err
	}
	root := doc.Content[0]
	var data map[string]any
	if err := root.Decode(&data); err != nil {
		return nil, err
	}
	known := map[string]bool{"DIRECT": true, "REJECT": true, "REJECT-DROP": true, "PASS": true, "PASS-RULE": true, "COMPATIBLE": true, "GLOBAL": true}
	graph := map[string][]string{}
	var issues []string
	for _, section := range []string{"proxies", "proxy-groups"} {
		for _, n := range children(field(root, section)) {
			name := value(field(n, "name"))
			_, defined := graph[name]
			if name == "" || (known[name] && name != "GLOBAL") || defined {
				issues = append(issues, fmt.Sprintf("%s 包含空名称或重复名称 %q", section, name))
				continue
			}
			known[name] = true
			graph[name] = nil
			if ref := value(field(n, "dialer-proxy")); ref != "" {
				graph[name] = append(graph[name], ref)
			}
			for _, m := range children(field(n, "proxies")) {
				graph[name] = append(graph[name], m.Value)
			}
			for _, m := range children(field(n, "use")) {
				if field(field(root, "proxy-providers"), m.Value) == nil {
					issues = append(issues, fmt.Sprintf("%s 引用了不存在的代理集合 %s", name, m.Value))
				}
			}
		}
	}
	visited := map[string]int{}
	var visit func(string)
	visit = func(name string) {
		if visited[name] == 1 {
			issues = append(issues, "策略组或链式代理存在循环："+name)
			return
		}
		if visited[name] == 2 {
			return
		}
		visited[name] = 1
		for _, ref := range graph[name] {
			if !known[ref] {
				issues = append(issues, fmt.Sprintf("%s 引用了不存在的节点或策略组 %s", name, ref))
				continue
			}
			if _, ok := graph[ref]; ok {
				visit(ref)
			}
		}
		visited[name] = 2
	}
	for name := range graph {
		visit(name)
	}
	providers := field(root, "rule-providers")
	dns := field(root, "dns")
	for _, key := range []string{"nameserver-policy", "proxy-server-nameserver-policy"} {
		for i, n := range children(field(dns, key)) {
			if i%2 != 0 {
				continue
			}
			names, ok := strings.CutPrefix(n.Value, "rule-set:")
			if !ok {
				continue
			}
			for _, name := range strings.Split(names, ",") {
				if field(providers, strings.TrimSpace(name)) == nil {
					issues = append(issues, fmt.Sprintf("DNS 策略 %s 引用了不存在的规则集 %s", key, name))
				}
			}
		}
	}

	subRules := field(root, "sub-rules")
	for _, list := range append([]*yaml.Node{field(root, "rules")}, subRuleLists(root)...) {
		for i, n := range children(list) {
			p := splitRule(n.Value)
			kind := strings.TrimSpace(p[0])
			if n.Kind != yaml.ScalarNode || len(p) < 2 || kind == "" {
				issues = append(issues, fmt.Sprintf("第 %d 条规则格式不正确", i+1))
				continue
			}
			target := strings.TrimSpace(p[targetIndex(p)])
			if kind == "SUB-RULE" {
				if field(subRules, target) == nil {
					issues = append(issues, "不存在的子规则 "+target)
				}
			} else if !known[target] {
				issues = append(issues, fmt.Sprintf("第 %d 条规则引用了不存在的策略 %s", i+1, target))
			}
			if kind == "MATCH" && i != len(list.Content)-1 {
				issues = append(issues, fmt.Sprintf("第 %d 条 MATCH 后还有规则", i+1))
			}
			if kind == "RULE-SET" && (len(p) < 3 || field(providers, strings.TrimSpace(p[1])) == nil) {
				issues = append(issues, fmt.Sprintf("第 %d 条规则引用了不存在的规则集", i+1))
			}
		}
	}
	if len(issues) > 0 {
		return data, errors.New(strings.Join(issues, "\n"))
	}
	return data, nil
}
