package config

import "testing"

func TestTemplateGroupMemberSelection(t *testing.T) {
	source := "proxies: [{name: A, type: http, server: a.test, port: 80}, {name: B, type: http, server: b.test, port: 80}]"
	generated, err := Generate(source, "balanced", true)
	if err != nil {
		t.Fatal(err)
	}
	template, err := Generate("proxies: []", "balanced", true)
	if err != nil {
		t.Fatal(err)
	}
	template, err = ApplyEdit(template, Edit{Section: "proxy-groups", Value: map[string]any{"name": "Chosen", "type": "select", "proxies": []string{"DIRECT"}, "x-node-members": []string{"B"}}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := ApplyRoutingTemplate(generated, template, []string{})
	if err != nil {
		t.Fatal(err)
	}
	out, err = NormalizeLayout(out)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Validate(out)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, raw := range data["proxy-groups"].([]any) {
		g := raw.(map[string]any)
		if g["name"] == "Chosen" {
			found = true
			members := g["proxies"].([]any)
			if len(members) != 1 || members[0] != "B" {
				t.Fatal("group member selection not applied", members)
			}
		}
	}
	if !found {
		t.Fatal("custom group missing")
	}
}
