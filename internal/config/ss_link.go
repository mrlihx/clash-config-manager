package config

import (
	"encoding/base64"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

func ssLink(source string) (map[string]any, error) {
	decode := func(text string) (string, error) {
		for _, enc := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding} {
			if b, e := enc.DecodeString(text); e == nil {
				return string(b), nil
			}
		}
		return "", errors.New("SS 链接编码无效")
	}
	content := strings.TrimPrefix(source, "ss://")
	raw, fragment, _ := strings.Cut(content, "#")
	address, query, _ := strings.Cut(raw, "?")
	if !strings.Contains(address, "@") {
		decoded, e := decode(strings.TrimSuffix(address, "/"))
		if e != nil {
			return nil, e
		}
		at := strings.LastIndex(decoded, "@")
		if at < 0 {
			return nil, errors.New("SS 链接缺少服务器")
		}
		credentials, host := decoded[:at], decoded[at+1:]
		cipher, password, ok := strings.Cut(credentials, ":")
		if !ok {
			return nil, errors.New("SS 链接缺少加密方式")
		}
		address = url.UserPassword(cipher, password).String() + "@" + host
	}
	u, e := url.Parse("ss://" + address + "?" + query + "#" + fragment)
	if e != nil || u.Hostname() == "" || u.User == nil {
		return nil, errors.New("SS 链接格式无效")
	}
	port, e := strconv.Atoi(u.Port())
	if e != nil || port < 1 || port > 65535 {
		return nil, errors.New("SS 端口无效")
	}
	cipher, password := u.User.Username(), ""
	if p, ok := u.User.Password(); ok {
		password = p
	} else {
		decoded, e := decode(cipher)
		if e != nil {
			return nil, e
		}
		var ok bool
		cipher, password, ok = strings.Cut(decoded, ":")
		if !ok {
			return nil, errors.New("SS 链接认证信息无效")
		}
	}
	if cipher == "" || password == "" {
		return nil, errors.New("SS 加密方式和密码不能为空")
	}
	name := u.Fragment
	if name == "" {
		name = u.Hostname()
	}
	node := map[string]any{"name": name, "type": "ss", "server": u.Hostname(), "port": port, "cipher": cipher, "password": password, "udp": true}
	q, e := url.ParseQuery(u.RawQuery)
	if e != nil {
		return nil, errors.New("SS 链接参数无效")
	}
	if plugin := q.Get("plugin"); plugin != "" {
		parts := strings.Split(plugin, ";")
		kind := parts[0]
		opts := map[string]any{}
		if kind == "obfs-local" || kind == "simple-obfs" {
			kind = "obfs"
		}
		if kind != "obfs" && kind != "v2ray-plugin" {
			return nil, errors.New("此 SS 插件请使用 YAML 导入")
		}
		for _, part := range parts[1:] {
			key, value, has := strings.Cut(part, "=")
			if key == "obfs" {
				key = "mode"
			}
			if key == "obfs-host" {
				key = "host"
			}
			if has {
				opts[key] = value
			} else if key != "" {
				opts[key] = true
			}
		}
		node["plugin"], node["plugin-opts"] = kind, opts
	}
	return node, nil
}
