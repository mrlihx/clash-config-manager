package config

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestLegacyProviderUpgradePreservesEditedPayload(t *testing.T) {
	// 使用虚构快照验证升级分支，避免把上游完整规则重新带入测试。
	original := legacyProviderHashes["Ads"]
	defer func() { legacyProviderHashes["Ads"] = original }()
	data, _ := json.Marshal([]string{"+.legacy.example"})
	legacyProviderHashes["Ads"] = fmt.Sprintf("%x", sha256.Sum256(data))
	for _, tc := range []struct{ name, domain, want string }{
		{"unchanged", "+.legacy.example", "http"},
		{"edited", "+.custom.example", "inline"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := `proxy-groups:
 - {name: 🚀 手动选择, type: select, proxies: [DIRECT]}
rule-providers:
 Ads: {type: inline, behavior: domain, payload: [DOMAIN]}
rules: ["MATCH,🚀 手动选择"]
x-manager: {preset: balanced}
`
			upgraded, err := UpgradeRoutingDefaults(strings.ReplaceAll(source, "DOMAIN", tc.domain))
			if err != nil {
				t.Fatal(err)
			}
			doc, err := Parse(upgraded)
			if err != nil {
				t.Fatal(err)
			}
			provider := field(field(doc.Content[0], "rule-providers"), "Ads")
			if value(field(provider, "type")) != tc.want {
				t.Fatalf("wrong provider migration: %s", upgraded)
			}
			if tc.want == "inline" && !strings.Contains(upgraded, tc.domain) {
				t.Fatal("custom payload lost")
			}
			if tc.want == "http" && (field(provider, "payload") != nil || value(field(provider, "url")) == "") {
				t.Fatal("remote provider not configured")
			}
		})
	}
}
