package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Resource struct {
	Name          string
	Section       string
	Type          string              `yaml:"type"`
	URL           string              `yaml:"url"`
	Path          string              `yaml:"path"`
	Behavior      string              `yaml:"behavior"`
	Format        string              `yaml:"format"`
	Interval      int                 `yaml:"interval"`
	Header        map[string][]string `yaml:"header"`
	Filter        string              `yaml:"filter"`
	ExcludeFilter string              `yaml:"exclude-filter"`
	Override      map[string]any      `yaml:"override"`
}

func ExternalResources(source string) ([]Resource, error) {
	doc, err := Parse(source)
	if err != nil {
		return nil, err
	}
	return resourcesFromRoot(doc.Content[0])
}

func resourcesFromRoot(root *yaml.Node) ([]Resource, error) {
	resources := []Resource{}
	for _, section := range []string{"rule-providers", "proxy-providers"} {
		providers := field(root, section)
		for i := 0; i+1 < len(children(providers)); i += 2 {
			var resource Resource
			if err := providers.Content[i+1].Decode(&resource); err != nil {
				return nil, fmt.Errorf("资源 %s 配置无效", providers.Content[i].Value)
			}
			resource.Name = providers.Content[i].Value
			resource.Section = section
			if resource.Type != "http" && resource.Type != "file" {
				continue
			}
			if resource.Interval <= 0 {
				resource.Interval = 86400
			}
			if resource.Format == "" {
				resource.Format = "yaml"
			}
			resources = append(resources, resource)
		}
	}
	return resources, nil
}

// 只改输出副本，后台仍保留远程地址和更新间隔。
func InlineResources(source string, load func(Resource) ([]byte, error)) (string, error) {
	return inlineResources(source, load, true, false)
}

func InlineNodeProviders(source string, load func(Resource) ([]byte, error)) (string, error) {
	return inlineResources(source, load, false, false)
}

// InlineSubscriptionResources 将服务分流规则和节点写入输出，通用规则集保留外部引用。
func InlineSubscriptionResources(source string, load func(Resource) ([]byte, error)) (string, error) {
	return inlineResources(source, load, false, true)
}

func inlineResources(source string, load func(Resource) ([]byte, error), includeRules, serviceRules bool) (string, error) {
	doc, err := Parse(source)
	if err != nil {
		return "", err
	}
	resources, err := resourcesFromRoot(doc.Content[0])
	if err != nil {
		return "", err
	}
	root := doc.Content[0]
	if !includeRules && !serviceRules && field(root, "proxy-providers") == nil {
		return source, nil
	}
	for _, rule := range children(field(root, "rules")) {
		if includeRules && (strings.Contains(rule.Value, "GEOIP,") || strings.Contains(rule.Value, "GEOSITE,")) {
			return "", fmt.Errorf("请先将 GEOIP/GEOSITE 转为域名或 IP 规则集，才能输出无外部规则依赖的订阅")
		}
	}
	if len(resources) == 0 && field(root, "proxy-providers") == nil {
		return source, nil
	}
	providerNodes := map[string][]string{}
	for _, resource := range resources {
		if !includeRules && resource.Section == "rule-providers" {
			service := serviceRules && slices.ContainsFunc(Categories, func(c RoutingCategory) bool {
				return !c.Direct && resource.Name == "System_"+c.ID
			})
			if !service {
				continue
			}
		}
		data, err := load(resource)
		if err != nil {
			return "", fmt.Errorf("资源 %s：%w", resource.Name, err)
		}
		if resource.Section == "rule-providers" {
			var payload struct {
				Payload []string `yaml:"payload"`
			}
			if err := yaml.Unmarshal(data, &payload); err != nil {
				return "", fmt.Errorf("规则集 %s 缓存无法解析", resource.Name)
			}
			lines := payload.Payload
			var node yaml.Node
			if err = node.Encode(map[string]any{"type": "inline", "behavior": resource.Behavior, "payload": lines}); err != nil {
				return "", err
			}
			set(field(root, "rule-providers"), resource.Name, &node)
		} else {
			converted, err := SubscriptionYAML(string(data))
			if err != nil {
				return "", fmt.Errorf("节点集合 %s 无法解析", resource.Name)
			}
			nodes, err := Parse(converted)
			if err != nil {
				return "", err
			}
			list := field(nodes.Content[0], "proxies")
			providerNodes[resource.Name], err = appendProviderNodes(root, list, resource)
			if err != nil {
				return "", err
			}
		}
	}
	// inline 节点集合也展开，输出无需客户端再加载任何节点集合。
	providers := field(root, "proxy-providers")
	for i := 0; i+1 < len(children(providers)); i += 2 {
		name, node := providers.Content[i].Value, providers.Content[i+1]
		if value(field(node, "type")) != "inline" {
			continue
		}
		var resource Resource
		if err = node.Decode(&resource); err != nil {
			return "", err
		}
		resource.Name = name
		providerNodes[name], err = appendProviderNodes(root, field(node, "payload"), resource)
		if err != nil {
			return "", err
		}
	}
	for _, group := range children(field(root, "proxy-groups")) {
		var use []string
		if n := field(group, "use"); n != nil {
			if err = n.Decode(&use); err != nil {
				return "", err
			}
		}
		if value(field(group, "include-all-providers")) == "true" || value(field(group, "include-all")) == "true" {
			for i := 0; i+1 < len(children(providers)); i += 2 {
				if !slices.Contains(use, providers.Content[i].Value) {
					use = append(use, providers.Content[i].Value)
				}
			}
		}
		if len(use) == 0 {
			continue
		}
		members := field(group, "proxies")
		if members == nil {
			members = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			set(group, "proxies", members)
		}
		for _, name := range use {
			names, ok := providerNodes[name]
			if !ok {
				return "", fmt.Errorf("节点集合 %s 未能完整展开", name)
			}
			for _, node := range names {
				matched, err := matchesNode(node, value(field(group, "filter")), value(field(group, "exclude-filter")))
				if err != nil {
					return "", err
				}
				if matched && !slices.ContainsFunc(members.Content, func(n *yaml.Node) bool { return n.Value == node }) {
					members.Content = append(members.Content, scalar(node))
				}
			}
		}
		set(group, "use", nil)
		set(group, "include-all-providers", nil)
	}
	set(root, "proxy-providers", nil)
	return encode(doc)
}

func matchesNode(name, include, exclude string) (bool, error) {
	for _, filter := range []struct {
		pattern string
		exclude bool
	}{{include, false}, {exclude, true}} {
		if filter.pattern == "" {
			continue
		}
		matched, err := regexp.MatchString(filter.pattern, name)
		if err != nil {
			return false, fmt.Errorf("节点筛选表达式无效")
		}
		if (!filter.exclude && !matched) || (filter.exclude && matched) {
			return false, nil
		}
	}
	return true, nil
}

func appendProviderNodes(root, list *yaml.Node, resource Resource) ([]string, error) {
	proxies := field(root, "proxies")
	if proxies == nil {
		proxies = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		set(root, "proxies", proxies)
	}
	names := []string{}
	for _, node := range children(list) {
		name := value(field(node, "name"))
		matched, err := matchesNode(name, resource.Filter, resource.ExcludeFilter)
		if err != nil {
			return nil, err
		}
		if !matched {
			continue
		}
		if name == "" {
			return nil, fmt.Errorf("节点集合 %s 包含空节点名称", resource.Name)
		}
		for _, existing := range proxies.Content {
			if value(field(existing, "name")) == name {
				return nil, fmt.Errorf("节点集合 %s 存在重复节点名称 %s，请先重命名", resource.Name, name)
			}
		}
		for key, val := range resource.Override {
			if strings.HasPrefix(key, "additional-") {
				return nil, fmt.Errorf("节点集合 %s 的名称覆盖需先转换为普通节点", resource.Name)
			}
			var override yaml.Node
			if err = override.Encode(val); err != nil {
				return nil, err
			}
			set(node, key, &override)
		}
		proxies.Content = append(proxies.Content, node)
		names = append(names, name)
	}
	return names, nil
}
