package server

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"clash-config-manager/internal/config"
	"go.yaml.in/yaml/v3"
)

var resourceClient = &http.Client{Timeout: 30 * time.Second}

func (s *Server) resourcePath(resource config.Resource) string {
	resource.Name = ""
	resource.Interval = 0
	data, _ := json.Marshal(resource)
	return filepath.Join(s.root, "data", "resources", fmt.Sprintf("%x.yaml", sha256.Sum256(data)))
}

func (s *Server) normalizeResource(ctx context.Context, resource config.Resource, data []byte) ([]byte, error) {
	if resource.Section == "proxy-providers" {
		source, err := config.SubscriptionYAML(string(data))
		if err != nil {
			return nil, err
		}
		var parsed struct {
			Proxies []map[string]any `yaml:"proxies"`
		}
		if err = yaml.Unmarshal([]byte(source), &parsed); err != nil || len(parsed.Proxies) == 0 {
			return nil, errors.New("节点集合中没有可用节点")
		}
		return []byte(source), nil
	}
	if resource.Format == "mrs" {
		if s.core == "" {
			return nil, errors.New("MRS 规则转换需要 Mihomo 内核")
		}
		dir, err := os.MkdirTemp(filepath.Join(s.root, "data", "resources"), ".mrs-*")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		from, to := filepath.Join(dir, "source.mrs"), filepath.Join(dir, "rules.txt")
		if err = os.WriteFile(from, data, 0600); err != nil {
			return nil, err
		}
		cmd := exec.CommandContext(ctx, s.core, "convert-ruleset", resource.Behavior, "mrs", from, to)
		if err = cmd.Run(); err != nil {
			return nil, errors.New("MRS 规则转换失败")
		}
		data, err = os.ReadFile(to)
		if err != nil {
			return nil, err
		}
	}
	lines, err := config.RuleText(string(data), resource.Behavior)
	if err != nil {
		return nil, err
	}
	for _, line := range lines {
		if strings.Contains(line, "GEOIP,") || strings.Contains(line, "GEOSITE,") {
			return nil, errors.New("请使用域名或 IP 规则集替代依赖外部数据库的 GEOIP/GEOSITE 规则")
		}
	}
	if s.core != "" {
		candidate, _ := yaml.Marshal(map[string]any{"rule-providers": map[string]any{"ResourceCheck": map[string]any{"type": "inline", "behavior": resource.Behavior, "payload": lines}}, "rules": []string{"RULE-SET,ResourceCheck,DIRECT", "MATCH,DIRECT"}})
		if err = s.checkCore(ctx, string(candidate)); err != nil {
			return nil, err
		}
	}
	return yaml.Marshal(map[string]any{"payload": lines})
}

// 按资源缓存并保留最近有效内容；同一 URL 的并发请求共用一次下载。
func (s *Server) loadResource(ctx context.Context, resource config.Resource) ([]byte, error) {
	path := s.resourcePath(resource)
	lock, _ := s.resourceLocks.LoadOrStore(path, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	cached, cacheErr := os.ReadFile(path)
	if resource.Type == "http" && cacheErr == nil {
		if failed, ok := s.resourceErrors.Load(path); ok {
			if time.Since(failed.(time.Time)) < time.Minute {
				return cached, nil
			}
		} else if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < time.Duration(resource.Interval)*time.Second {
			return cached, nil
		}
	}
	var data []byte
	var err error
	if resource.Type == "file" {
		local := resource.Path
		if !filepath.IsAbs(local) {
			local = filepath.Join(s.root, "data", local)
		}
		relative, e := filepath.Rel(filepath.Join(s.root, "data"), local)
		if e != nil || !filepath.IsLocal(relative) {
			return nil, errors.New("本地资源必须位于 data 目录内")
		}
		data, err = os.ReadFile(local)
	} else {
		u, e := url.Parse(resource.URL)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, errors.New("资源地址需为 HTTP 或 HTTPS")
		}
		req, e := http.NewRequestWithContext(ctx, "GET", resource.URL, nil)
		if e != nil {
			return nil, errors.New("资源地址无效")
		}
		req.Header.Set("User-Agent", "clash-config-manager")
		for key, values := range resource.Header {
			req.Header.Del(key)
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
		response, e := resourceClient.Do(req)
		if e != nil {
			err = errors.New("下载资源失败")
		} else {
			if response.StatusCode != 200 {
				err = fmt.Errorf("资源服务器返回 HTTP %d", response.StatusCode)
			} else {
				data, err = io.ReadAll(io.LimitReader(response.Body, (32<<20)+1))
				if len(data) > 32<<20 {
					err = errors.New("单个资源不能超过 32 MiB")
				}
			}
			response.Body.Close()
		}
	}
	if err == nil {
		data, err = s.normalizeResource(ctx, resource, data)
	}
	if err != nil {
		if cacheErr == nil {
			s.resourceErrors.Store(path, time.Now())
			return cached, nil
		}
		// 兼容升级前由内核下载的规则缓存。
		if resource.Section == "rule-providers" && resource.Type == "http" {
			previous, e := os.ReadFile(filepath.Join(s.root, "data", "rules", fmt.Sprintf("%x", md5.Sum([]byte(resource.URL)))))
			if e == nil {
				if normalized, e := s.normalizeResource(ctx, resource, previous); e == nil {
					os.WriteFile(path, normalized, 0600)
					s.resourceErrors.Store(path, time.Now())
					return normalized, nil
				}
			}
		}
		return nil, errors.New("没有可用的资源缓存，请检查规则来源或平台网络")
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".resource-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return nil, errors.New("保存资源缓存失败")
	}
	if err = os.Rename(file.Name(), path); err != nil {
		return nil, err
	}
	s.resourceErrors.Delete(path)
	return data, nil
}

func (s *Server) bundleSubscription(ctx context.Context, source string, nodesOnly bool) (string, bool, error) {
	stale := false
	inline := config.InlineResources
	if nodesOnly {
		inline = config.InlineNodeProviders
	}
	out, err := inline(source, func(resource config.Resource) ([]byte, error) {
		data, err := s.loadResource(ctx, resource)
		if _, ok := s.resourceErrors.Load(s.resourcePath(resource)); ok {
			stale = true
		}
		return data, err
	})
	return out, stale, err
}

func (s *Server) refreshResources(ctx context.Context) {
	s.Store.Lock()
	list, err := s.Store.List()
	sources := []string{}
	if err == nil {
		for _, item := range list {
			if item.Kind == "template" || item.Kind == "subscription" {
				if p, e := s.Store.Get(item.ID); e == nil {
					sources = append(sources, p.Draft)
					if p.Published != "" && p.Published != p.Draft {
						sources = append(sources, p.Published)
					}
				}
			}
		}
	}
	s.Store.Unlock()
	pending := map[string]config.Resource{}
	for _, source := range sources {
		resources, err := config.ExternalResources(source)
		if err != nil {
			continue
		}
		for _, resource := range resources {
			key := s.resourcePath(resource)
			if previous, ok := pending[key]; !ok || resource.Interval < previous.Interval {
				pending[key] = resource
			}
		}
	}
	jobs := make(chan config.Resource)
	var workers sync.WaitGroup
	for i := 0; i < 4; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for resource := range jobs {
				if ctx.Err() == nil {
					s.loadResource(ctx, resource)
				}
			}
		}()
	}
	for _, resource := range pending {
		select {
		case jobs <- resource:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return
		}
	}
	close(jobs)
	workers.Wait()
}
