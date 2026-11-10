package config

import (
	"go.yaml.in/yaml/v3"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// SyncRouting 三方合并：未修改的字段跟随方案，订阅自己的修改继续保留。
func SyncRouting(current, previous, next string) (string, error) {
	decode := func(source string) (map[string]any, error) {
		doc, e := Parse(source)
		if e != nil {
			return nil, e
		}
		var value map[string]any
		e = doc.Decode(&value)
		return value, e
	}
	now, err := decode(current)
	if err != nil {
		return "", err
	}
	old, err := decode(previous)
	if err != nil {
		return "", err
	}
	fresh, err := decode(next)
	if err != nil {
		return "", err
	}
	for key, value := range fresh {
		if slices.Contains([]string{"proxies", "proxy-providers", "rules", "proxy-groups"}, key) {
			continue
		}
		now[key] = mergeRoutingValue(now[key], old[key], value)
	}
	for key := range old {
		if slices.Contains([]string{"proxies", "proxy-providers", "rules", "proxy-groups"}, key) {
			continue
		}
		if _, ok := fresh[key]; !ok && reflect.DeepEqual(now[key], old[key]) {
			delete(now, key)
		}
	}
	for _, key := range []string{"rules", "proxy-groups"} {
		now[key] = mergeRoutingList(now[key], old[key], fresh[key], key == "rules")
	}
	bytes, err := yaml.Marshal(now)
	if err != nil {
		return "", err
	}
	result, err := NormalizeLayout(string(bytes))
	if err != nil {
		return "", err
	}
	_, err = Validate(result)
	return result, err
}

func mergeRoutingValue(current, old, next any) any {
	if reflect.DeepEqual(current, old) {
		return next
	}
	if reflect.DeepEqual(old, next) {
		return current
	}
	c, cok := current.(map[string]any)
	o, _ := old.(map[string]any)
	n, nok := next.(map[string]any)
	if cok && nok {
		result := map[string]any{}
		for key, value := range c {
			result[key] = value
		}
		for key, value := range n {
			merged := mergeRoutingValue(c[key], o[key], value)
			if merged != nil {
				result[key] = merged
			} else {
				delete(result, key)
			}
		}
		for key := range o {
			if _, exists := n[key]; !exists && reflect.DeepEqual(c[key], o[key]) {
				delete(result, key)
			}
		}
		return result
	}
	return current
}

func mergeRoutingList(current, old, next any, rules bool) []any {
	index := func(value any) ([]string, map[string]any) {
		items, _ := value.([]any)
		order := []string{}
		values := map[string]any{}
		counts := map[string]int{}
		for _, item := range items {
			key := ""
			if rules {
				parts := splitRule(item.(string))
				at := targetIndex(parts)
				if at >= 0 && at < len(parts) {
					parts = append(parts[:at], parts[at+1:]...)
				}
				key = strings.Join(parts, ",")
			} else {
				key = item.(map[string]any)["name"].(string)
			}
			counts[key]++
			key += "#" + strconv.Itoa(counts[key])
			order = append(order, key)
			values[key] = item
		}
		return order, values
	}
	co, c := index(current)
	oo, o := index(old)
	no, n := index(next)
	values := map[string]any{}
	keys := slices.Concat(oo, no, co)
	for _, key := range keys {
		merged := mergeRoutingValue(c[key], o[key], n[key])
		if merged != nil {
			values[key] = merged
		}
	}
	common := func(order []string, other map[string]any) []string {
		result := []string{}
		for _, key := range order {
			if other[key] != nil {
				result = append(result, key)
			}
		}
		return result
	}
	order := slices.Clone(no)
	if !slices.Equal(common(co, o), common(oo, c)) {
		order = slices.Clone(co)
	}
	order = slices.DeleteFunc(order, func(key string) bool { return values[key] == nil })
	for _, reference := range [][]string{co, no} {
		for i, key := range reference {
			if values[key] == nil || slices.Contains(order, key) {
				continue
			}
			at := len(order)
			for _, following := range reference[i+1:] {
				if found := slices.Index(order, following); found >= 0 {
					at = found
					break
				}
			}
			order = slices.Insert(order, at, key)
		}
	}
	result := []any{}
	for _, key := range order {
		result = append(result, values[key])
	}
	return result
}

// BuildRouting 从方案生成继承基线，节点来自订阅自身而非模板中的占位项。
func BuildRouting(source, template string, services, sets []string) (string, error) {
	generated, err := Generate(source, "balanced", true)
	if err != nil {
		return "", err
	}
	generated, err = SubscriptionOptions(generated, services, sets)
	if err != nil {
		return "", err
	}
	return ApplyRoutingTemplate(generated, template, sets)
}

func RoutingSelections(source string) ([]string, []string, error) {
	doc, err := Parse(source)
	if err != nil {
		return nil, nil, err
	}
	var settings managedSettings
	if meta := field(doc.Content[0], "x-manager"); meta != nil {
		if err = meta.Decode(&settings); err != nil {
			return nil, nil, err
		}
	}
	services, sets := []string{}, []string{}
	if settings.Services != nil {
		services = *settings.Services
	} else {
		for _, category := range Categories {
			if !category.Direct && slicesFind(children(field(doc.Content[0], "proxy-groups")), category.Name) != nil {
				services = append(services, category.ID)
			}
		}
	}
	if settings.RuleSets != nil {
		sets = *settings.RuleSets
	} else {
		for _, option := range RuleOptions {
			key := "System_" + option.ID
			if option.ID == "Ads" {
				key = "Ads"
			}
			if field(field(doc.Content[0], "rule-providers"), key) != nil {
				sets = append(sets, option.ID)
			}
		}
		for i, key := range children(field(doc.Content[0], "rule-providers")) {
			if i%2 == 0 && !strings.HasPrefix(key.Value, "System_") && key.Value != "Ads" {
				sets = append(sets, key.Value)
			}
		}
	}
	return services, sets, nil
}

func RoutingOverrideSections(current, baseline string) []string {
	var now, old map[string]any
	if yaml.Unmarshal([]byte(current), &now) != nil || yaml.Unmarshal([]byte(baseline), &old) != nil {
		return nil
	}
	result := []string{}
	for key, value := range now {
		if slices.Contains([]string{"proxies", "proxy-providers", "x-manager"}, key) {
			continue
		}
		before := old[key]
		if key == "proxy-groups" {
			stripMembers := func(input any) any {
				groups, _ := input.([]any)
				out := []any{}
				for _, raw := range groups {
					group := map[string]any{}
					for name, v := range raw.(map[string]any) {
						if name != "proxies" && name != "use" {
							group[name] = v
						}
					}
					out = append(out, group)
				}
				return out
			}
			value, before = stripMembers(value), stripMembers(before)
		}
		if !reflect.DeepEqual(value, before) {
			result = append(result, key)
		}
	}
	slices.Sort(result)
	return result
}
