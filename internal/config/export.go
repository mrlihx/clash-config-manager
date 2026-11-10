package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ClientFormat 根据 User-Agent 识别客户端，未识别时返回 Clash。
func ClientFormat(agent string) string {
	agent = strings.ToLower(agent)
	for _, v := range []string{"clash", "mihomo", "stash", "clash-meta"} {
		if strings.Contains(agent, v) {
			return "clash"
		}
	}
	for _, v := range []string{"v2rayng", "v2rayn/", "hiddify", "nekobox", "nekoray", "shadowrocket", "streisand", "happ/"} {
		if strings.Contains(agent, v) {
			return "v2ray"
		}
	}
	return "clash"
}

func ExportLinks(source string) (string, int, error) {
	data, err := Validate(source)
	if err != nil {
		return "", 0, err
	}
	links := []string{}
	skipped := 0
	nodes, _ := data["proxies"].([]any)
	for _, raw := range nodes {
		n := raw.(map[string]any)
		str := func(k string) string {
			if n[k] == nil {
				return ""
			}
			return fmt.Sprint(n[k])
		}
		if str("dialer-proxy") != "" {
			skipped++
			continue
		}
		protocol := str("type")
		u := url.URL{Scheme: protocol, Host: net.JoinHostPort(str("server"), str("port")), Fragment: str("name")}
		q := url.Values{}
		if n["skip-cert-verify"] == true {
			q.Set("insecure", "1")
		}
		if alpn, ok := n["alpn"].([]any); ok {
			parts := []string{}
			for _, v := range alpn {
				parts = append(parts, fmt.Sprint(v))
			}
			q.Set("alpn", strings.Join(parts, ","))
		}
		switch protocol {
		case "vless", "trojan":
			credential := str("uuid")
			if protocol == "trojan" {
				credential = str("password")
			}
			u.User = url.User(credential)
			network := str("network")
			if network == "" {
				network = "tcp"
			}
			if network != "tcp" && network != "ws" && network != "grpc" {
				skipped++
				continue
			}
			q.Set("type", network)
			if protocol == "vless" {
				q.Set("encryption", "none")
			}
			security := "none"
			if n["tls"] == true || protocol == "trojan" {
				security = "tls"
			}
			if reality, ok := n["reality-opts"].(map[string]any); ok {
				security = "reality"
				for k, to := range map[string]string{"public-key": "pbk", "short-id": "sid"} {
					if reality[k] != nil {
						q.Set(to, fmt.Sprint(reality[k]))
					}
				}
			}
			q.Set("security", security)
			if protocol == "trojan" && str("sni") != "" {
				q.Set("sni", str("sni"))
			}
			for k, to := range map[string]string{"servername": "sni", "client-fingerprint": "fp", "flow": "flow"} {
				if str(k) != "" {
					q.Set(to, str(k))
				}
			}
			if ws, ok := n["ws-opts"].(map[string]any); ok {
				if ws["path"] != nil {
					q.Set("path", fmt.Sprint(ws["path"]))
				}
				if headers, ok := ws["headers"].(map[string]any); ok && headers["Host"] != nil {
					q.Set("host", fmt.Sprint(headers["Host"]))
				}
			}
			if grpc, ok := n["grpc-opts"].(map[string]any); ok && grpc["grpc-service-name"] != nil {
				q.Set("serviceName", fmt.Sprint(grpc["grpc-service-name"]))
			}
		case "anytls", "hysteria2":
			u.User = url.User(str("password"))
			if str("sni") != "" {
				q.Set("sni", str("sni"))
			}
			if str("obfs") != "" {
				q.Set("obfs", str("obfs"))
				q.Set("obfs-password", str("obfs-password"))
			}
		case "ss":
			if n["plugin"] != nil {
				skipped++
				continue
			}
			u.User = url.User(base64.RawURLEncoding.EncodeToString([]byte(str("cipher") + ":" + str("password"))))
		case "vmess":
			network := str("network")
			if network == "" {
				network = "tcp"
			}
			if network != "tcp" && network != "ws" && network != "grpc" {
				skipped++
				continue
			}
			v := map[string]any{"v": "2", "ps": str("name"), "add": str("server"), "port": str("port"), "id": str("uuid"), "aid": str("alterId"), "scy": str("cipher"), "net": network, "type": "none", "sni": str("servername"), "fp": str("client-fingerprint")}
			if v["aid"] == "" {
				v["aid"] = "0"
			}
			if v["scy"] == "" {
				v["scy"] = "auto"
			}
			if n["tls"] == true {
				v["tls"] = "tls"
			}
			if opts, ok := n["ws-opts"].(map[string]any); ok {
				v["path"] = opts["path"]
				if h, ok := opts["headers"].(map[string]any); ok {
					v["host"] = h["Host"]
				}
			}
			if opts, ok := n["grpc-opts"].(map[string]any); ok {
				v["path"] = opts["grpc-service-name"]
			}
			b, _ := json.Marshal(v)
			links = append(links, "vmess://"+base64.StdEncoding.EncodeToString(b))
			continue
		case "http", "socks5":
			if protocol == "http" && n["tls"] == true {
				u.Scheme = "https"
			}
			if str("username") != "" {
				u.User = url.UserPassword(str("username"), str("password"))
			}
		default:
			skipped++
			continue
		}
		u.RawQuery = q.Encode()
		links = append(links, u.String())
	}
	if len(links) == 0 {
		return "", skipped, fmt.Errorf("没有可转换的独立节点，链式节点或协议不支持，请使用 Clash 格式")
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n"))), skipped, nil
}
