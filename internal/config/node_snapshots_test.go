package config

import (
	"strings"
	"testing"
)

func TestSnapshotMergeDoesNotDuplicateChainFront(t *testing.T) {
	primary := "proxies: [{name: Front, type: http, server: front.test, port: 80}]"
	snapshot := `proxies:
 - {name: Front, type: http, server: front.test, port: 80}
 - {name: Exit, type: http, server: exit.test, port: 80, dialer-proxy: Front}
`
	out, err := AppendNodeSnapshots(primary, snapshot, []string{"Exit"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := Validate(out)
	if err != nil {
		t.Fatal(err)
	}
	nodes := data["proxies"].([]any)
	if len(nodes) != 2 || strings.Contains(out, "订阅自有节点") {
		t.Fatal("snapshot created artificial source names")
	}
	if nodes[1].(map[string]any)["dialer-proxy"] != "Front" {
		t.Fatal("front dependency lost")
	}
}

func TestLegacySnapshotNamesAreCleanedWithReferences(t *testing.T) {
	for _, server := range []string{"front.test", "different.test"} {
		source := `proxies:
 - {name: Front, type: http, server: front.test, port: 80}
 - {name: Front · 订阅自有节点, type: http, server: REPLACE, port: 80}
 - {name: Exit, type: http, server: exit.test, port: 80, dialer-proxy: Front · 订阅自有节点}
proxy-groups:
 - {name: Choice, type: select, proxies: [Front · 订阅自有节点]}
`
		out, err := NormalizeLayout(strings.ReplaceAll(source, "REPLACE", server))
		if err != nil {
			t.Fatal(err)
		}
		data, err := Validate(out)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "订阅自有节点") {
			t.Fatal("legacy suffix remains")
		}
		nodes := data["proxies"].([]any)
		if server == "front.test" && len(nodes) != 2 {
			t.Fatal("duplicate dependency not removed")
		}
		if server != "front.test" && len(nodes) != 3 {
			t.Fatal("distinct endpoint was removed")
		}
	}
}
