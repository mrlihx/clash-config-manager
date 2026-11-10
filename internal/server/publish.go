package server

import (
	"clash-config-manager/internal/config"
	"context"
	"errors"
	"io"
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
	list, listErr := s.Store.List()
	if listErr == nil {
		for _, item := range list {
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
	if format == "v2ray" {
		var skipped int
		source, skipped, err = config.ExportLinks(source)
		if err != nil {
			fail(w, 422, err.Error())
			return
		}
		w.Header().Set("X-Subscription-Skipped", strconv.Itoa(skipped))
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "inline")
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
