package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"clash-config-manager/internal/config"
)

var sourceTimezone = time.FixedZone("Asia/Shanghai", 8*60*60)

func nextSourceUpdate(now time.Time, minutes int, at string) (time.Time, error) {
	if at != "" {
		clock, err := time.Parse("15:04", at)
		if err != nil || clock.Format("15:04") != at {
			return time.Time{}, errors.New("每日更新时间请使用 HH:mm 格式")
		}
		local := now.In(sourceTimezone)
		next := time.Date(local.Year(), local.Month(), local.Day(), clock.Hour(), clock.Minute(), 0, 0, sourceTimezone)
		if !next.After(now) {
			next = next.AddDate(0, 0, 1)
		}
		return next, nil
	}
	if minutes < 0 || minutes > 525600 {
		return time.Time{}, errors.New("更新间隔需要在 1 到 525600 分钟内")
	}
	if minutes == 0 {
		return time.Time{}, nil
	}
	return now.Add(time.Duration(minutes) * time.Minute), nil
}

func (s *Server) editSource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Revision                        int
		Name, URL, YAML, Mode, UpdateAt string
		AutoUpdate                      bool
		UpdateMinutes                   int
	}
	if !jsonBody(w, r, &req) {
		return
	}
	s.Store.Lock()
	p, err := s.Store.Get(r.PathValue("id"))
	s.Store.Unlock()
	if err != nil || p.Kind != "source" {
		fail(w, 404, "来源配置不存在")
		return
	}
	if p.Revision != req.Revision {
		fail(w, 409, "配置已更新，请刷新后重试")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	if req.Name == "" {
		fail(w, 400, "请输入配置名称")
		return
	}
	source := ""
	if req.YAML != "" {
		source, err = config.NodeSource(req.YAML)
		req.URL = ""
	} else if req.URL != "" && req.URL != p.SourceURL {
		source, err = downloadSubscription(r.Context(), req.URL)
		if err == nil {
			source, err = config.NodeSource(source)
		}
	}
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	p.Name, p.SourceURL = req.Name, req.URL
	p.UpdateMinutes, p.UpdateAt = 0, ""
	if req.AutoUpdate && p.SourceURL != "" {
		switch req.Mode {
		case "daily":
			p.UpdateAt = req.UpdateAt
			if p.UpdateAt == "" {
				fail(w, 400, "请选择每日更新时间")
				return
			}
		case "interval":
			p.UpdateMinutes = req.UpdateMinutes
			if p.UpdateMinutes < 1 {
				fail(w, 400, "更新间隔至少为 1 分钟")
				return
			}
		default:
			fail(w, 400, "请选择更新方式")
			return
		}
	}
	now := time.Now()
	p.NextUpdate, err = nextSourceUpdate(now, p.UpdateMinutes, p.UpdateAt)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if source != "" {
		p.Draft, p.Original = source, source
		p.LastUpdate = now
		p.UpdateError = ""
	}
	s.Store.Lock()
	defer s.Store.Unlock()
	current, err := s.Store.Get(p.ID)
	if err != nil || current.Revision != req.Revision {
		fail(w, 409, "配置已更新，请刷新后重试")
		return
	}
	p.Revision++
	if err = s.Store.Save(p); err != nil {
		fail(w, 500, "保存来源配置失败")
		return
	}
	if err = s.syncSubscriptions(p.ID); err != nil {
		fail(w, 422, err.Error())
		return
	}
	s.profileReply(w, p)
}

func (s *Server) deleteSource(w http.ResponseWriter, r *http.Request) {
	var req struct{ Revision int }
	if !jsonBody(w, r, &req) {
		return
	}
	s.Store.Lock()
	defer s.Store.Unlock()
	p, err := s.Store.Get(r.PathValue("id"))
	if err != nil || p.Kind != "source" {
		fail(w, 404, "来源配置不存在")
		return
	}
	if p.Revision != req.Revision {
		fail(w, 409, "配置已更新，请刷新后重试")
		return
	}
	if err = s.Store.DeleteSource(p); err != nil {
		fail(w, 500, "删除来源配置失败")
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}

func (s *Server) refreshSource(ctx context.Context, id string, now time.Time) error {
	s.Store.Lock()
	p, err := s.Store.Get(id)
	s.Store.Unlock()
	if err != nil || p.Kind != "source" || p.SourceURL == "" {
		return errors.New("此配置没有可更新的来源订阅")
	}
	source, downloadErr := downloadSubscription(ctx, p.SourceURL)
	if downloadErr == nil {
		source, downloadErr = config.NodeSource(source)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.Store.Lock()
	defer s.Store.Unlock()
	current, err := s.Store.Get(id)
	if err != nil || current.Revision != p.Revision {
		return errors.New("来源配置已修改，请重新更新")
	}
	originalSource := source
	if downloadErr == nil {
		previous := p.Original
		if previous == "" {
			previous = p.Draft
		}
		source, downloadErr = config.SyncNodes(p.Draft, previous, source)
	}
	p.NextUpdate, _ = nextSourceUpdate(now, p.UpdateMinutes, p.UpdateAt)
	p.UpdateError = ""
	if downloadErr != nil {
		p.UpdateError = downloadErr.Error()
	} else {
		p.Draft, p.Original = source, originalSource
		p.LastUpdate = now
	}
	p.Revision++
	if err = s.Store.Save(p); err != nil {
		return err
	}
	if downloadErr == nil {
		return s.syncSubscriptions(id)
	}
	return downloadErr
}

func (s *Server) updateSource(w http.ResponseWriter, r *http.Request) {
	if !jsonBody(w, r, new(struct{})) {
		return
	}
	if err := s.refreshSource(r.Context(), r.PathValue("id"), time.Now()); err != nil {
		fail(w, 422, err.Error())
		return
	}
	s.detail(w, r)
}

func (s *Server) refreshDueSources(ctx context.Context, now time.Time) {
	s.Store.Lock()
	ids, err := s.Store.DueSources(now)
	s.Store.Unlock()
	if err != nil {
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		_ = s.refreshSource(ctx, id, now)
	}
}

func (s *Server) RunSourceUpdater(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	_ = s.syncRoutingSubscriptions(ctx, "", "")
	s.refreshDueSources(ctx, time.Now())
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.refreshDueSources(ctx, now)
		}
	}
}
