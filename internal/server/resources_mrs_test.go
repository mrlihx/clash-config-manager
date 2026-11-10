package server

import (
	"context"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var mihomoTest = flag.String("mihomo-test", "", "Mihomo executable for MRS integration test")

func TestMRSResourceConversion(t *testing.T) {
	if *mihomoTest == "" {
		t.Skip("use -mihomo-test to verify real MRS conversion")
	}
	dir := t.TempDir()
	files := map[string][]byte{}
	for behavior, text := range map[string]string{"domain": "+.example.test\n+.second.test\n", "ipcidr": "203.0.113.0/24\n2001:db8::/32\n"} {
		source, target := filepath.Join(dir, behavior+".txt"), filepath.Join(dir, behavior+".mrs")
		if err := os.WriteFile(source, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if err := exec.Command(*mihomoTest, "convert-ruleset", behavior, "text", source, target).Run(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		files["/"+behavior] = data
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(files[r.URL.Path]) }))
	defer upstream.Close()
	s := New(nil, "test", *mihomoTest, dir)
	source := "rule-providers:\n Domains: {type: http, behavior: domain, format: mrs, url: '" + upstream.URL + "/domain'}\n Networks: {type: http, behavior: ipcidr, format: mrs, url: '" + upstream.URL + "/ipcidr'}\nrules: ['RULE-SET,Domains,DIRECT','RULE-SET,Networks,DIRECT,no-resolve','MATCH,DIRECT']\n"
	out, stale, err := s.bundleSubscription(context.Background(), source, false)
	if err != nil || stale {
		t.Fatal("MRS conversion failed", err)
	}
	if !strings.Contains(out, "2001:db8::/32") || !strings.Contains(out, "example.test") || strings.Contains(out, upstream.URL) {
		t.Fatal("converted rules lost contents", out)
	}
	if err = s.checkCore(context.Background(), out); err != nil {
		t.Fatal("Mihomo rejected bundled config", err)
	}
}
