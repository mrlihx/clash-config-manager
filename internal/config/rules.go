package config

import (
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

func splitRule(rule string) []string {
	var parts []string
	start, depth := 0, 0
	for i, c := range rule {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, rule[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, rule[start:])
}

func targetIndex(parts []string) int {
	i := len(parts) - 1
	for i > 0 && slices.Contains([]string{"no-resolve", "src"}, strings.TrimSpace(parts[i])) {
		i--
	}
	return i
}

func subRuleLists(root *yaml.Node) []*yaml.Node {
	var lists []*yaml.Node
	for i, n := range children(field(root, "sub-rules")) {
		if i%2 == 1 {
			lists = append(lists, n)
		}
	}
	return lists
}
