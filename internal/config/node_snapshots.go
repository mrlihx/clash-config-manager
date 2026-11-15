package config

// 保留订阅快照中独有的节点和依赖；同名依赖沿用选中的来源，不再次改名。
func AppendNodeSnapshots(primary, snapshot string, names []string) (string, error) {
	local, err := CollectNodes([]NodeSelection{{Source: snapshot, Nodes: names}})
	if err != nil {
		return "", err
	}
	local, err = NodeSource(local)
	if err != nil {
		return "", err
	}
	doc, err := Parse(primary)
	if err != nil {
		return "", err
	}
	extra, err := Parse(local)
	if err != nil {
		return "", err
	}
	for _, section := range []string{"proxies", "proxy-groups"} {
		incoming := field(extra.Content[0], section)
		target := field(doc.Content[0], section)
		if target == nil {
			set(doc.Content[0], section, incoming)
			continue
		}
		for _, node := range children(incoming) {
			if slicesFind(target.Content, value(field(node, "name"))) == nil {
				target.Content = append(target.Content, node)
			}
		}
	}
	return encode(doc)
}
