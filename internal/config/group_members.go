package config

import (
	"go.yaml.in/yaml/v3"
	"slices"
	"strings"
)

// 模板中保存节点名称筛选，生成订阅时再匹配实际节点，避免模板携带节点凭据。
func applyTemplateGroupMembers(root, template *yaml.Node) {
	for _, group := range children(field(root, "proxy-groups")) {
		policy := slicesFind(children(field(template, "proxy-groups")), value(field(group, "name")))
		selected := field(policy, "x-node-members")
		if selected != nil {
			var names []string
			_ = selected.Decode(&names)
			members := []string{}
			for _, node := range children(field(root, "proxies")) {
				name := value(field(node, "name"))
				if slices.ContainsFunc(names, func(want string) bool { return name == want || strings.HasPrefix(name, want+" · ") }) {
					members = append(members, name)
				}
			}
			if len(members) == 0 {
				members = []string{"REJECT"}
			}
			var list yaml.Node
			_ = list.Encode(members)
			set(group, "proxies", &list)
			set(group, "x-members-fixed", scalar("true"))
			set(group, "use", nil)
			set(group, "include-all", nil)
			set(group, "include-all-proxies", nil)
			set(group, "include-all-providers", nil)
		}
		if selected == nil {
			set(group, "x-members-fixed", nil)
		}
		set(group, "x-node-members", nil)
	}
}
