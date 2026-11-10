package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"clash-config-manager/internal/config"
)

const maxSubscription = 4 << 20

var subscriptionTransport = &http.Transport{ForceAttemptHTTP2: true}

func downloadSubscription(ctx context.Context, raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !subscriptionURL(u) {
		return "", errors.New("请输入有效的 HTTP 或 HTTPS 订阅链接")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", errors.New("无法读取订阅地址")
	}
	request.Header.Set("User-Agent", "clash.meta")
	request.Header.Set("Accept", "application/yaml, text/plain, */*")
	client := &http.Client{Transport: subscriptionTransport, Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !subscriptionURL(req.URL) {
			return errors.New("订阅重定向无效")
		}
		// 避免向跳转地址发送包含订阅密钥的 Referer。
		req.Header.Del("Referer")
		return nil
	}}
	response, err := client.Do(request)
	if err != nil {
		return "", errors.New("无法下载订阅，请检查服务器连通性和链接有效期")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("订阅服务器返回 HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxSubscription+1))
	if err != nil {
		return "", errors.New("读取订阅内容失败")
	}
	if len(data) > maxSubscription {
		return "", errors.New("订阅内容超过 4 MiB")
	}
	return config.SubscriptionYAML(string(data))
}

func subscriptionURL(u *url.URL) bool {
	return u != nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https")
}

func (s *Server) importSubscription(w http.ResponseWriter, r *http.Request) {
	var req struct{ URL, YAML string }
	if !jsonBody(w, r, &req) {
		return
	}
	var source string
	var err error
	if req.URL != "" {
		source, err = downloadSubscription(r.Context(), req.URL)
	} else {
		source, err = config.SubscriptionYAML(req.YAML)
	}
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	data, validation := config.Validate(source)
	if data == nil {
		fail(w, 422, validation.Error())
		return
	}
	reply(w, 200, map[string]any{"yaml": source, "data": data})
}
