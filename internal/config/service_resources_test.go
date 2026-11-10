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

func TestServiceEditorPreservesSourcesAndManualOverrides(t *testing.T) {
	original := `rule-providers:
  System_Claude:
    type: http
    behavior: domain
    url: https://example.com/claude.yaml
rules:
  - MATCH,DIRECT
`
	load := func(Resource) ([]byte, error) { return []byte("payload:\n  - '+.claude.ai'\n"), nil }
	rendered, err := InlineSubscriptionResources(original, load)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreServiceSources(original, rendered, load)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(restored, "https://example.com/claude.yaml") {
		t.Fatal("unchanged service lost automatic update source")
	}
	edited := strings.ReplaceAll(rendered, "+.claude.ai", "+.anthropic.com")
	saved, err := RestoreServiceSources(original, edited, load)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(saved, "https://example.com/claude.yaml") || !strings.Contains(saved, "+.anthropic.com") {
		t.Fatal("manual service override was lost")
	}
}
