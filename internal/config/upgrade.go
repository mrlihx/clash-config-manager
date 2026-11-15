package config

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

func UpgradeRoutingDefaults(source string) (string, error) {
	doc, err := Parse(source)
	if err != nil {
		return "", err
	}
	root := doc.Content[0]
	meta := field(root, "x-manager")
	if meta == nil || value(field(meta, "routing-version")) == "3" {
		return source, nil
	}
	groups := field(root, "proxy-groups")
	if groups == nil {
		return source, nil
	}
	var version yaml.Node
	_ = version.Encode(3)
	set(meta, "routing-version", &version)
	for _, category := range Categories {
		emoji := serviceEmoji[category.ID]
		if emoji == "" {
			continue
		}
		old := strings.TrimPrefix(category.Name, emoji+" ")
		group := slicesFind(groups.Content, old)
		if group == nil || slicesFind(groups.Content, category.Name) != nil {
			continue
		}
		set(group, "name", scalar(category.Name))
		references(root, "proxy-groups", old, category.Name)
	}
	nodes := []string{}
	for _, node := range children(field(root, "proxies")) {
		nodes = append(nodes, value(field(node, "name")))
	}
	if len(nodes) == 0 {
		nodes = append(nodes, "DIRECT")
	}
	for _, entry := range []struct {
		name    string
		members []string
	}{{MainGroup, nodes}, {AutoGroup, nodes}, {AdGroup, []string{"REJECT", "PASS"}}} {
		if slicesFind(groups.Content, entry.name) != nil {
			continue
		}
		var group yaml.Node
		fields := map[string]any{"name": entry.name, "type": "select", "proxies": entry.members}
		if entry.name == AutoGroup {
			fields["type"] = "url-test"
			fields["url"] = "https://www.gstatic.com/generate_204"
			fields["interval"] = 300
			fields["tolerance"] = 50
		}
		_ = group.Encode(fields)
		groups.Content = append(groups.Content, &group)
	}
	auto := slicesFind(groups.Content, AutoGroup)
	if auto != nil {
		if value(field(auto, "interval")) == "300" && value(field(auto, "tolerance")) == "50" {
			var interval, tolerance yaml.Node
			_ = interval.Encode(60)
			_ = tolerance.Encode(0)
			set(auto, "interval", &interval)
			set(auto, "tolerance", &tolerance)
		}
		if field(auto, "lazy") == nil {
			var lazy yaml.Node
			_ = lazy.Encode(false)
			set(auto, "lazy", &lazy)
		}
	}
	ordered := []*yaml.Node{slicesFind(groups.Content, MainGroup), slicesFind(groups.Content, AutoGroup)}
	for _, group := range groups.Content {
		if group != ordered[0] && group != ordered[1] {
			ordered = append(ordered, group)
		}
	}
	if !slices.Equal(groups.Content, ordered) {
		groups.Content = ordered
	}
	for _, list := range []*yaml.Node{field(root, "rules"), field(field(root, "x-manager"), "rules")} {
		for _, rule := range children(list) {
			for _, provider := range []string{"Ads", "System_Reject"} {
				if rule.Value == "RULE-SET,"+provider+",REJECT" {
					rule.Value = "RULE-SET," + provider + "," + AdGroup
				}
			}
		}
	}
	if ads := field(field(root, "x-manager"), "ads"); value(ads) == "REJECT" {
		ads.Value = AdGroup
	}
	providers := field(root, "rule-providers")
	sources := BuiltinProviderSources()
	for i, n := range children(providers) {
		if i%2 != 0 {
			continue
		}
		name := n.Value
		provider := field(providers, name)
		if sources[name] == "" || value(field(provider, "type")) != "inline" {
			continue
		}
		id := strings.TrimPrefix(name, "System_")
		var payload []string
		if field(provider, "payload") == nil || field(provider, "payload").Decode(&payload) != nil {
			continue
		}
		data, _ := json.Marshal(payload)
		if fmt.Sprintf("%x", sha256.Sum256(data)) != legacyProviderHashes[id] {
			continue
		}
		set(provider, "type", scalar("http"))
		set(provider, "url", scalar(sources[name]))
		set(provider, "format", scalar("yaml"))
		set(provider, "path", nil)
		set(provider, "payload", nil)
		if field(provider, "interval") == nil {
			var interval yaml.Node
			_ = interval.Encode(86400)
			set(provider, "interval", &interval)
		}
	}
	return encode(doc)
}
