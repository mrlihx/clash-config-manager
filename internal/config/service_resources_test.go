package config

import (
	"strings"
	"testing"
)

func TestSubscriptionServiceRulesInline(t *testing.T) {
	source := `rule-providers:
  System_Claude:
    type: http
    behavior: domain
    url: https://example.com/claude.yaml
  System_GFW:
    type: http
    behavior: domain
    url: https://example.com/gfw.yaml
rules:
  - RULE-SET,System_Claude,Claude
  - RULE-SET,System_GFW,Proxy
  - MATCH,Proxy
`
	calls := 0
	out, err := InlineSubscriptionResources(source, func(r Resource) ([]byte, error) {
		calls++
		if r.Name != "System_Claude" {
			t.Fatalf("unexpected fetch: %s", r.Name)
		}
		return []byte("payload:\n  - '+.claude.ai'\n"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || strings.Contains(out, "https://example.com/claude.yaml") || !strings.Contains(out, "claude.ai") || !strings.Contains(out, "https://example.com/gfw.yaml") {
		t.Fatalf("incorrect selective inlining: %s", out)
	}
}
