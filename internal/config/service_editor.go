package config

import (
	"reflect"
)

// 编辑器中的未修改内联规则还原为后台来源，继续参与自动更新；手改内容保留为覆盖。
func RestoreServiceSources(original, edited string, load func(Resource) ([]byte, error)) (string, error) {
	rendered, err := InlineSubscriptionResources(original, load)
	if err != nil {
		return "", err
	}
	before, err := Parse(rendered)
	if err != nil {
		return "", err
	}
	after, err := Parse(edited)
	if err != nil {
		return "", err
	}
	source, err := Parse(original)
	if err != nil {
		return "", err
	}
	providers := field(after.Content[0], "rule-providers")
	for _, c := range Categories {
		if c.Direct {
			continue
		}
		name := "System_" + c.ID
		shown := field(field(before.Content[0], "rule-providers"), name)
		current := field(providers, name)
		origin := field(field(source.Content[0], "rule-providers"), name)
		if shown == nil || current == nil || origin == nil {
			continue
		}
		var a, b any
		if err := shown.Decode(&a); err != nil {
			return "", err
		}
		if err := current.Decode(&b); err != nil {
			return "", err
		}
		if reflect.DeepEqual(a, b) {
			set(providers, name, origin)
		}
	}
	return encode(after)
}
