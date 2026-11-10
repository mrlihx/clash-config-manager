package server

import (
	"clash-config-manager/internal/config"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *Server) subscription(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("slug")
	slug := strings.TrimSuffix(filename, ".yaml")
	if !slugPattern.MatchString(slug) {
		http.NotFound(w, r)
		return
	}
	s.Store.Lock()
	var syncErr error
	targetID := ""
	name := slug
	updateHours := 24
	resourceProxy := ""
	list, listErr := s.Store.List()
	if listErr == nil {
		for _, item := range list {
			if item.Slug == slug {
				name = item.Name
				if p, e := s.Store.Get(item.ID); e == nil {
					updateHours = p.ClientUpdateHours
					resourceProxy = p.ResourceProxyPrefix
				}
			}
			if item.Slug == slug && item.Kind == "subscription" {
				targetID = item.ID
				syncErr = s.syncSubscriptions("sub:" + item.ID)
				break
			}
		}
	} else {
		syncErr = listErr
	}
	s.Store.Unlock()
	if syncErr == nil && targetID != "" {
		syncErr = s.syncRoutingSubscriptions(r.Context(), "", targetID)
	}
	if syncErr != nil {
		w.Header().Set("X-Subscription-Sync", "failed; serving-last-snapshot")
	}
	source, err := s.Store.Published(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Vary", "User-Agent")
	format := config.ClientFormat(r.UserAgent())
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(3 * time.Minute))
	bundled, stale, e := s.bundleSubscription(r.Context(), source, true)
	if e != nil {
		fail(w, 503, "订阅资源尚未准备好："+e.Error())
		return
	}
	source = bundled
	if stale {
		w.Header().Set("X-Resource-Sync", "stale; serving-last-cache")
	}
	if format != "v2ray" {
		source, err = config.ExternalRuleURLs(source, resourceProxy)
		if err != nil {
			fail(w, 422, err.Error())
			return
		}
	}
	if format == "v2ray" {
		var skipped int
		source, skipped, err = config.ExportLinks(source)
		if err != nil {
			fail(w, 422, err.Error())
			return
		}
		w.Header().Set("X-Subscription-Skipped", strconv.Itoa(skipped))
	}

	w.Header().Set("Profile-Update-Interval", strconv.Itoa(updateHours))
	w.Header().Set("Profile-Title", "base64:"+base64.StdEncoding.EncodeToString([]byte(name)))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": name}))
	_, _ = io.WriteString(w, source)
}

func (s *Server) checkCore(ctx context.Context, source string) error {
	if s.core == "" {
		return errors.New("未找到 Mihomo 内核。请在 config.yaml 配置可执行的 mihomo 路径后重启")
	}
	s.coreMu.Lock()
	defer s.coreMu.Unlock()
	dataDir := filepath.Join(s.root, "data")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return errors.New("无法创建校验目录")
	}
	file, err := os.CreateTemp(dataDir, ".validation-*.yaml")
	if err != nil {
		return errors.New("无法创建临时配置")
	}
	path := file.Name()
	defer os.Remove(path)
	_, writeErr := file.WriteString(source)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return errors.New("无法写入临时配置")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.core, "-t", "-d", dataDir, "-f", path)
	cmd.Dir = dataDir
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return errors.New("Mihomo 校验超时或被取消")
		}
		return errors.New("Mihomo 校验失败。请检查当前内核版本、配置字段和规则集资源；内核原始日志可能含凭据")
	}
	return nil
}
