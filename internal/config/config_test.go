package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

const example = `dns:
  enable: true
proxies:
  - {name: origin, type: http, server: example.test, port: 8080}
proxy-groups:
  - {name: Default, type: select, proxies: [origin]}
rules:
  - DOMAIN,example.test,origin
  - MATCH,Default
`

func TestRenameNodeAndReferences(t *testing.T) {
	output, err := ApplyEdit(example, Edit{Section: "proxies", Key: "origin", Value: map[string]any{"name": "renamed"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"enable: true", "proxies: [renamed]", "DOMAIN,example.test,renamed", "MATCH,Default", "port: 8080"} {
		if !strings.Contains(output, part) {
			t.Errorf("missing %q in %s", part, output)
		}
	}
}

func TestValidationFindsCycleAndShadowedRules(t *testing.T) {
	source := strings.Replace(example, "[origin]", "[Default]", 1) + "  - DOMAIN,other.test,DIRECT\n"
	_, err := Validate(source)
	if err == nil || !strings.Contains(err.Error(), "MATCH") || !strings.Contains(err.Error(), "循环") {
		t.Fatalf("got %v", err)
	}
}

func TestVLESSLinkPreservesTransport(t *testing.T) {
	got, err := NodeLink("vless://01234567-89ab-cdef-0123-456789abcdef@ws.example.test:443?security=tls&type=ws&sni=ws.example.test&host=cdn.example.test&path=%2Fpath#WS")
	if err != nil {
		t.Fatal(err)
	}
	opts := got["ws-opts"].(map[string]any)
	if got["tls"] != true || opts["path"] != "/path" || opts["headers"].(map[string]string)["Host"] != "cdn.example.test" {
		t.Fatal(got)
	}
}

func TestSubscriptionPayloads(t *testing.T) {
	const raw = "anytls://sample-pass@proxy.example.test:443#Test"
	for _, source := range []string{example, raw, base64.RawStdEncoding.EncodeToString([]byte(raw))} {
		source, err := SubscriptionYAML(source)
		if err != nil {
			t.Fatal(err)
		}
		data, err := Validate(source)
		if err != nil || len(data["proxies"].([]any)) != 1 {
			t.Fatalf("invalid subscription: %v", err)
		}
	}
	if _, err := SubscriptionYAML("error: subscription expired"); err == nil {
		t.Fatal("accepted an upstream error document")
	}
}

func TestTrojanAndSSLinks(t *testing.T) {
	trojan, err := NodeLink("trojan://fixture-password@node.test:443?sni=sni.test&type=ws&host=cdn.test&path=%2Fsocket#Trojan")
	if err != nil {
		t.Fatal(err)
	}
	if trojan["type"] != "trojan" || trojan["password"] != "fixture-password" || trojan["sni"] != "sni.test" || trojan["network"] != "ws" {
		t.Fatal("Trojan fields missing")
	}
	for _, link := range []string{
		"ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:fixture:password")) + "@node.test:8388#SS",
		"ss://" + base64.StdEncoding.EncodeToString([]byte("aes-128-gcm:fixture:password@node.test:8388")) + "#SS",
		"ss://aes-128-gcm:fixture%3Apassword@node.test:8388#SS",
	} {
		ss, err := NodeLink(link)
		if err != nil {
			t.Fatal(err)
		}
		if ss["cipher"] != "aes-128-gcm" || ss["password"] != "fixture:password" || ss["port"] != 8388 {
			t.Fatal("SS credentials or endpoint changed")
		}
		yaml, err := SubscriptionYAML(link)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _, err := ExportLinks(yaml)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = NodeLink(string(raw)); err != nil {
			t.Fatal(err)
		}
	}
}
