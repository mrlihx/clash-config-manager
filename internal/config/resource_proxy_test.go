package config

import (
	"strings"
	"testing"
)

func TestExternalRuleURLAcceleration(t *testing.T) {
	source := `rule-providers:
 Public: {type: http, behavior: domain, url: 'https://raw.githubusercontent.com/example/rules/main/public.mrs', format: mrs}
 Private: {type: http, behavior: domain, url: 'https://raw.githubusercontent.com/example/rules/main/private.yaml', header: {Authorization: ['Bearer fixture']}}
 Query: {type: http, behavior: domain, url: 'https://github.com/example/rules/raw/main/query.yaml?token=fixture'}
 Other: {type: http, behavior: domain, url: 'https://rules.example/rules.yaml'}
 Manual: {type: inline, behavior: domain, payload: ['+.example.test']}
rules: ['RULE-SET,Public,DIRECT', 'MATCH,DIRECT']
`
	out, err := ExternalRuleURLs(source, "https://accelerator.example/")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "https://accelerator.example/") != 1 || !strings.Contains(out, "https://accelerator.example/https://raw.githubusercontent.com/example/rules/main/public.mrs") {
		t.Fatal("unexpected rewrite", out)
	}
	if !strings.Contains(out, "format: mrs") || !strings.Contains(out, "type: inline") {
		t.Fatal("existing rule settings changed")
	}
	unchanged, err := ExternalRuleURLs(source, "")
	if err != nil || unchanged != source {
		t.Fatal("disabled acceleration modified output")
	}
	for _, value := range []string{"http://unsafe.example/", "https://user:pass@example.com/", "https://example.com/?key=fixture", "invalid"} {
		if _, err = NormalizeResourceProxy(value); err == nil {
			t.Fatal("accepted invalid prefix", value)
		}
	}
	if normalized, err := NormalizeResourceProxy(" https://accelerator.example "); err != nil || normalized != "https://accelerator.example/" {
		t.Fatal("prefix not normalized", normalized, err)
	}
}
