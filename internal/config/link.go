package config

import (
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

func NodeLink(source string) (map[string]any, error) {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "ss://") {
		return ssLink(source)
	}
	u, err := url.Parse(source)
	if err != nil || u.Hostname() == "" {
		return nil, errors.New("请输入有效的节点链接")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, errors.New("节点链接参数格式不正确")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("节点链接端口无效")
	}
	protocol := u.Scheme
	if protocol == "hy2" {
		protocol = "hysteria2"
	}
	if protocol == "https" {
		protocol = "http"
	}
	if !slices.Contains([]string{"vless", "trojan", "anytls", "hysteria2", "http"}, protocol) {
		return nil, errors.New("当前支持 VLESS、Trojan、SS、AnyTLS、Hysteria2 和 HTTP 节点链接")
	}
	name := u.Fragment
	if name == "" {
		name = u.Hostname()
	}
	node := map[string]any{"name": name, "type": protocol, "server": u.Hostname(), "port": port}
	add := func(key, input string) {
		if input != "" {
			node[key] = input
		}
	}
	username, password := "", ""
	if u.User != nil {
		username = u.User.Username()
		password, _ = u.User.Password()
	}
	sni := q.Get("sni")
	if sni == "" {
		sni = q.Get("peer")
	}
	insecure := q.Get("insecure")
	if insecure == "" {
		insecure = q.Get("allowInsecure")
	}
	if insecure != "" {
		node["skip-cert-verify"] = insecure == "1" || insecure == "true"
	}
	if alpn := q.Get("alpn"); alpn != "" {
		node["alpn"] = strings.Split(alpn, ",")
	}
	switch protocol {
	case "http":
		add("username", username)
		add("password", password)
		add("sni", sni)
		node["tls"] = u.Scheme == "https"
	case "anytls", "hysteria2":
		if username == "" {
			return nil, errors.New("节点链接缺少密码")
		}
		if password != "" {
			username += ":" + password
		}
		node["password"] = username
		add("sni", sni)
		if protocol == "hysteria2" {
			add("obfs", q.Get("obfs"))
			add("obfs-password", q.Get("obfs-password"))
		}
	case "vless", "trojan":
		if username == "" {
			return nil, errors.New("节点链接缺少 UUID")
		}
		if protocol == "trojan" {
			if password != "" {
				username += ":" + password
			}
			node["password"] = username
			add("sni", sni)
		} else {
			node["uuid"] = username
		}
		if protocol == "vless" {
			add("servername", sni)
		}
		add("flow", q.Get("flow"))
		add("client-fingerprint", q.Get("fp"))
		add("encryption", q.Get("encryption"))
		network := q.Get("type")
		if network == "" {
			network = "tcp"
		}
		if !slices.Contains([]string{"tcp", "ws", "grpc"}, network) {
			return nil, errors.New("此 VLESS 链接的传输类型暂不支持导入，请使用 YAML 导入")
		}
		node["network"] = network
		security := q.Get("security")
		if !slices.Contains([]string{"", "none", "tls", "reality"}, security) {
			return nil, errors.New("VLESS 安全类型无效")
		}
		if protocol == "vless" {
			node["tls"] = security == "tls" || security == "reality"
		}
		if security == "reality" {
			if protocol == "trojan" {
				return nil, errors.New("Trojan 不支持此 Reality 链接")
			}
			if q.Get("pbk") == "" {
				return nil, errors.New("Reality 链接缺少公钥")
			}
			reality := map[string]string{"public-key": q.Get("pbk"), "short-id": q.Get("sid")}
			if q.Get("pqv") != "" {
				return nil, errors.New("此 Reality 扩展参数请使用 YAML 导入")
			}
			node["reality-opts"] = reality
		}
		if network == "ws" {
			path := q.Get("path")
			if path == "" {
				path = "/"
			}
			opts := map[string]any{"path": path}
			if host := q.Get("host"); host != "" {
				opts["headers"] = map[string]string{"Host": host}
			}
			node["ws-opts"] = opts
		}
		if network == "grpc" {
			node["grpc-opts"] = map[string]string{"grpc-service-name": q.Get("serviceName")}
		}
	}
	return node, nil
}
