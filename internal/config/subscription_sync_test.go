package config

import (
	"strings"
	"testing"
)

func TestSubscriptionRefreshRemovesOldSnapshotsAndFronts(t *testing.T) {
	current := `proxies:
 - {name: OldFront, type: http, server: old.test, port: 80}
 - {name: Active, type: http, server: obsolete.test, port: 80}
 - {name: OldExit, type: http, server: retired.test, port: 80, dialer-proxy: OldGroup}
proxy-groups:
 - {name: OldGroup, type: select, proxies: [OldFront]}
 - {name: Choice, type: select, proxies: [OldExit, Active]}
rules: ['DOMAIN,retired.test,OldExit','MATCH,Choice']
`
	next := "proxies: [{name: Active, type: http, server: current.test, port: 443}]"
	out, err := SyncSubscriptionNodes(current, next)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Validate(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(data["proxies"].([]any)) != 1 || strings.Contains(out, "old.test") || strings.Contains(out, "obsolete.test") || strings.Contains(out, "retired.test:80") {
		t.Fatal("stale node data survived refresh")
	}
	if strings.Contains(out, "OldGroup") || strings.Contains(out, "OldExit") {
		t.Fatal("stale references survived")
	}
	again, err := SyncSubscriptionNodes(out, next)
	if err != nil || again != out {
		t.Fatal("cleanup is not stable")
	}
}
