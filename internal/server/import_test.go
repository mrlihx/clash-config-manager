package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadSubscriptionUsesClashUserAgent(t *testing.T) {
	const yaml = "proxies: []\nrules: [\"MATCH,DIRECT\"]\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "clash.meta" {
			w.WriteHeader(400)
			return
		}
		_, _ = io.WriteString(w, yaml)
	}))
	defer upstream.Close()
	got, err := downloadSubscription(context.Background(), upstream.URL+"/subscription?token=fixture")
	if err != nil || got != yaml {
		t.Fatalf("downloaded unexpected configuration: %v", err)
	}
	_, err = downloadSubscription(context.Background(), "file:///tmp/private")
	if err == nil || strings.Contains(err.Error(), "/tmp/private") {
		t.Fatal("invalid link was accepted or echoed")
	}
}
