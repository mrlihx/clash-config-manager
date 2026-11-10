package config

import (
	"errors"
	"net/url"
	"strings"
)

func NormalizeResourceProxy(prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", nil
	}
	u, err := url.Parse(prefix)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("加速地址需为不含账号、查询参数或片段的 HTTPS 前缀")
	}
	return strings.TrimRight(prefix, "/") + "/", nil
}

// 原始规则地址保留在平台中，仅在订阅输出时应用加速前缀。
func ExternalRuleURLs(source, prefix string) (string, error) {
	if prefix == "" {
		return source, nil
	}
	doc, err := Parse(source)
	if err != nil {
		return "", err
	}
	providers := field(doc.Content[0], "rule-providers")
	changed := false
	for i := 0; i+1 < len(children(providers)); i += 2 {
		provider := providers.Content[i+1]
		if value(field(provider, "type")) != "http" {
			continue
		}
		original := field(provider, "url")
		if original == nil {
			continue
		}
		u, err := url.Parse(original.Value)
		if err != nil || u.User != nil || u.RawQuery != "" || (u.Scheme != "https" && u.Scheme != "http") {
			continue
		}
		host := strings.ToLower(u.Hostname())
		if host != "raw.githubusercontent.com" && host != "github.com" {
			continue
		}
		// 带认证的资源不交给公共加速服务。
		if len(children(field(provider, "header"))) != 0 {
			continue
		}
		original.Value = prefix + original.Value
		changed = true
	}
	if !changed {
		return source, nil
	}
	return encode(doc)
}
