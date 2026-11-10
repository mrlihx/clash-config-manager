package server

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"clash-config-manager/internal/config"
	"clash-config-manager/internal/store"
	"github.com/pmezard/go-difflib/difflib"
)

var slugPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func (s *Server) profiles(w http.ResponseWriter, r *http.Request) {
	s.Store.Lock()
	defer s.Store.Unlock()
	list, err := s.Store.List()
	if err != nil {
		fail(w, 500, "读取配置失败")
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "source"
	}
	list = slices.DeleteFunc(list, func(p store.Summary) bool { return p.Kind != kind })
	reply(w, 200, list)
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var req struct{ Name, Slug, YAML, CopyID, SourceURL, Preset string }
	if !jsonBody(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		fail(w, 400, "请输入配置名称")
		return
	}
	if req.SourceURL != "" && req.CopyID == "" {
		source, err := downloadSubscription(r.Context(), req.SourceURL)
		if err != nil {
			fail(w, 422, err.Error())
			return
		}
		req.YAML = source
	}
	s.Store.Lock()
	defer s.Store.Unlock()
	if req.CopyID != "" {
		p, err := s.Store.Get(req.CopyID)
		if err != nil {
			fail(w, 404, "源配置不存在")
			return
		}
		req.YAML = p.Draft
	}
	if req.YAML == "" {
		req.YAML = "proxies: []\n"
	}
	if req.CopyID == "" {
		generated, err := config.NodeSource(req.YAML)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		req.YAML = generated
	}
	if _, err := config.Parse(req.YAML); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if req.Slug == "" {
		req.Slug = randomSlug()
	}
	if !s.checkSlug(w, req.Slug, "") {
		return
	}
	p := &store.Profile{ID: randomToken(16), Name: req.Name, Slug: req.Slug, Enabled: true, Revision: 1, Original: req.YAML, Draft: req.YAML}
	if req.SourceURL != "" && req.CopyID == "" {
		p.SourceURL = req.SourceURL
		p.UpdateMinutes = 1440
		p.LastUpdate = time.Now()
		p.NextUpdate, _ = nextSourceUpdate(time.Now(), 1440, "")
	}
	if err := s.Store.Save(p); err != nil {
		fail(w, 500, "保存配置失败")
		return
	}
	s.profileReply(w, p)
}

func randomSlug() string { return "cfg-" + randomToken(12) }

func (s *Server) checkSlug(w http.ResponseWriter, slug, id string) bool {
	if !slugPattern.MatchString(slug) {
		fail(w, 400, "后缀须为 1–64 位字母、数字、下划线或短横线，且以字母或数字开头")
		return false
	}
	ok, err := s.Store.SlugAvailable(slug, id)
	if err != nil {
		fail(w, 500, "检查订阅后缀失败")
		return false
	}
	if !ok {
		fail(w, 409, "订阅后缀已存在")
		return false
	}
	return true
}

func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	s.Store.Lock()
	defer s.Store.Unlock()
	p, err := s.Store.Get(r.PathValue("id"))
	if err != nil {
		fail(w, 404, "配置不存在")
		return
	}
	s.profileReply(w, p)
}

func (s *Server) profileReply(w http.ResponseWriter, p *store.Profile) {
	data, err := config.Validate(p.Draft)
	preset, builtinCount := config.ManagedInfo(p.Draft)
	out := map[string]any{"clientUpdateHours": p.ClientUpdateHours, "kind": p.Kind, "id": p.ID, "name": p.Name, "slug": p.Slug, "enabled": p.Enabled, "revision": p.Revision, "yaml": p.Draft, "data": data,
		"published": p.Published != "", "publishedAt": p.PublishedAt, "dirty": p.Draft != p.Published,
		"preset": preset, "builtinCount": builtinCount, "systemGroups": config.SystemGroups(), "sourceURL": p.SourceURL, "updateMinutes": p.UpdateMinutes, "updateAt": p.UpdateAt, "nextUpdate": p.NextUpdate, "lastUpdate": p.LastUpdate, "updateError": p.UpdateError, "subscription": "/s/" + p.Slug}
	updated := map[string]int64{}
	if providers, ok := data["rule-providers"].(map[string]any); ok {
		for name, raw := range providers {
			provider := raw.(map[string]any)
			path, _ := provider["path"].(string)
			if provider["type"] == "http" {
				url, _ := provider["url"].(string)
				path = filepath.Join("rules", fmt.Sprintf("%x", md5.Sum([]byte(url))))
			}
			clean := filepath.Clean(path)
			if !filepath.IsLocal(clean) {
				continue
			}
			if info, e := os.Stat(filepath.Join(s.root, "data", clean)); e == nil && !info.IsDir() {
				updated[name] = info.ModTime().Unix()
			}
		}
	}
	if resources, e := config.ExternalResources(p.Draft); e == nil {
		failed := map[string]bool{}
		for _, resource := range resources {
			if resource.Section != "rule-providers" {
				continue
			}
			if info, e := os.Stat(s.resourcePath(resource)); e == nil {
				updated[resource.Name] = info.ModTime().Unix()
			}
			if _, ok := s.resourceErrors.Load(s.resourcePath(resource)); ok {
				failed[resource.Name] = true
			}
		}
		out["ruleProviderStale"] = failed
	}
	out["ruleProviderUpdated"] = updated
	if p.Kind == "subscription" {
		if state, e := routingState(p); e == nil {
			out["routingPlan"] = state.RoutingPlan
			out["followRouting"] = state.FollowRouting == nil || *state.FollowRouting
			out["syncError"] = p.UpdateError
			out["routingOverrides"] = config.RoutingOverrideSections(p.Draft, state.RoutingBaseline)
		}
	}
	if err != nil {
		out["validation"] = err.Error()
	}
	reply(w, 200, out)
}

type changeRequest struct {
	ClientUpdateHours *int
	Preview           bool
	Action            string
	Revision          int
	YAML              string
	Edit              config.Edit
	Source            string
	Selection         map[string][]string
	Slug              string
	Enabled           *bool
	Name              string
	Preset            string
	RuleTarget        string
	RoutingPlan       string
	FollowRouting     *bool
	Services          []string
	RuleSets          []string
}

func (s *Server) change(w http.ResponseWriter, r *http.Request) {
	var req changeRequest
	if !jsonBody(w, r, &req) {
		return
	}
	id := r.PathValue("id")
	s.Store.Lock()
	p, err := s.Store.Get(id)
	s.Store.Unlock()
	if err != nil {
		fail(w, 404, "配置不存在")
		return
	}
	if p.Revision != req.Revision {
		fail(w, 409, "配置已更新，请刷新后重试")
		return
	}
	var next string
	switch req.Action {
	case "inspect-subscription":
		data, e := config.Validate(req.YAML)
		if e != nil {
			fail(w, 422, e.Error())
			return
		}
		reply(w, 200, map[string]any{"data": data})
		return
	case "subscription-routing", "preview-subscription-routing":
		if p.Kind != "subscription" {
			fail(w, 400, "仅支持编辑已生成的订阅")
			return
		}
		planID := req.RoutingPlan
		if planID == "" {
			state, e := routingState(p)
			if e != nil {
				fail(w, 500, "读取订阅关联失败")
				return
			}
			planID = state.RoutingPlan
		}
		template, e := s.routingPlan(planID)
		if e != nil {
			fail(w, 500, "读取系统规则失败")
			return
		}
		options := templateRuleOptions(template)
		for _, id := range req.RuleSets {
			if !slices.ContainsFunc(options, func(o config.RuleOption) bool { return o.ID == id }) {
				fail(w, 400, "规则集不存在")
				return
			}
		}
		base := p.Draft
		if req.Action == "preview-subscription-routing" {
			base = req.YAML
		}
		state, e := routingState(p)
		if e != nil {
			fail(w, 500, "读取订阅关联失败")
			return
		}
		if template.ID != state.RoutingPlan {
			baseline, e := config.BuildRouting(base, template.Draft, req.Services, req.RuleSets)
			err = e
			if err == nil {
				next, err = config.SyncRouting(base, state.RoutingBaseline, baseline)
			}
		} else {
			next, err = config.ConfigureSubscriptionRouting(base, template.Draft, req.Services, req.RuleSets)
		}
		if req.Action == "preview-subscription-routing" {
			if err != nil {
				fail(w, 422, err.Error())
				return
			}
			data, _ := config.Validate(next)
			reply(w, 200, map[string]any{"yaml": next, "data": data})
			return
		}

	case "generate":
		next, err = config.Generate(p.Draft, req.Preset, false)
	case "edit":
		_, builtinCount := config.ManagedInfo(p.Draft)
		if builtinCount > 0 && req.Edit.Section == "proxy-groups" && slices.Contains(config.SystemGroups(), req.Edit.Key) {
			fields, _ := req.Edit.Value.(map[string]any)
			if req.Edit.Delete || (fields["name"] != nil && fields["name"] != req.Edit.Key) {
				fail(w, 400, "内置服务策略组可以调整出口，名称由系统维护")
				return
			}
		}

		if req.Edit.Section == "rule-providers" {
			next, err = config.ApplyRuleProvider(p.Draft, req.Edit)
		} else {
			next, err = config.ApplyEdit(p.Draft, req.Edit)
		}
	case "merge":
		source := req.Source
		if req.RuleTarget != "" {
			source, err = config.SelectedRules(source, req.Selection, req.RuleTarget)
		}
		if err == nil {
			next, err = config.Merge(p.Draft, source, req.Selection)
		}
	case "save", "preview", "save-subscription":
		next, err = config.NormalizeLayout(req.YAML)
		if err == nil && p.Kind == "subscription" && req.RoutingPlan != "" {
			state, e := routingState(p)
			if e != nil {
				fail(w, 500, "读取订阅关联失败")
				return
			}
			if state.RoutingPlan != req.RoutingPlan {
				plan, e := s.routingPlan(req.RoutingPlan)
				err = e
				if err == nil {
					services, sets, e := config.RoutingSelections(next)
					err = e
					if err == nil {
						baseline, e := config.BuildRouting(next, plan.Draft, services, sets)
						err = e
						if err == nil {
							next, err = config.SyncRouting(next, state.RoutingBaseline, baseline)
						}
					}
				}
			}
		}
	case "diff":
		base := p.Published
		if base == "" {
			base = p.Original
		}
		showDiff(w, base, p.Draft)
		return
	case "validate", "publish":
		if _, err = config.Validate(p.Draft); err != nil {
			fail(w, 422, err.Error())
			return
		}
		if err = s.checkCore(r.Context(), p.Draft); err != nil {
			fail(w, 422, err.Error())
			return
		}
		if req.Action == "validate" {
			reply(w, 200, map[string]bool{"ok": true})
			return
		}
		p.Published, p.PublishedAt = p.Draft, time.Now()
	case "subscription":
		if req.Slug == "" {
			req.Slug = randomSlug()
		}

		p.Slug = req.Slug
		if req.Enabled != nil {
			p.Enabled = *req.Enabled
		}
	case "rename":
		p.Name = strings.TrimSpace(req.Name)
		if p.Name == "" {
			fail(w, 400, "请输入配置名称")
			return
		}
	default:
		fail(w, 400, "未知操作")
		return
	}

	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if p.Kind == "subscription" && (req.Action == "save-subscription" || req.Action == "subscription") {
		if req.ClientUpdateHours != nil {
			if *req.ClientUpdateHours < 1 || *req.ClientUpdateHours > 720 {
				fail(w, 400, "客户端更新间隔需在 1 到 720 小时之间")
				return
			}
			p.ClientUpdateHours = *req.ClientUpdateHours
		}
		if req.Name != "" {
			p.Name = strings.TrimSpace(req.Name)
			if p.Name == "" {
				fail(w, 400, "请输入订阅名称")
				return
			}
		}
	}
	if req.Action == "preview" || req.Preview {
		showDiff(w, p.Draft, next)
		return
	}
	if next != "" && p.Kind == "template" {
		if _, err = config.Validate(next); err != nil {
			fail(w, 422, err.Error())
			return
		}
		if err = s.checkCore(r.Context(), next); err != nil {
			fail(w, 422, err.Error())
			return
		}
	}
	// 订阅保存即更新已发布快照，校验失败时保留原有链接内容。
	if next != "" && (p.Kind == "subscription" || req.Action == "save-subscription") {
		if _, err = config.Validate(next); err != nil {
			fail(w, 422, err.Error())
			return
		}
		if err = s.checkCore(r.Context(), next); err != nil {
			fail(w, 422, err.Error())
			return
		}
		p.Published = next
		p.PublishedAt = time.Now()
	}
	if next != "" && next != p.Draft {
		p.Draft = next
	}
	if p.Kind == "subscription" {
		state, e := routingState(p)
		if e != nil {
			fail(w, 500, "读取订阅关联失败")
			return
		}
		if req.RoutingPlan != "" {
			state.RoutingPlan = req.RoutingPlan
		}
		if req.FollowRouting != nil {
			state.FollowRouting = req.FollowRouting
		}
		if next != "" {
			template, e := s.routingPlan(state.RoutingPlan)
			if e != nil {
				fail(w, 400, e.Error())
				return
			}
			services, sets, e := config.RoutingSelections(next)
			if e != nil {
				fail(w, 422, e.Error())
				return
			}
			baseline, e := config.BuildRouting(next, template.Draft, services, sets)
			if e != nil {
				fail(w, 422, e.Error())
				return
			}
			state.RoutingBaseline = baseline
			state.FollowAllRuleSets = allRuleOptions(templateRuleOptions(template), sets)
		}
		b, _ := json.Marshal(state)
		p.Sync = string(b)
		p.UpdateError = ""
	}
	s.Store.Lock()
	current, err := s.Store.Get(id)
	if err != nil || current.Revision != req.Revision {
		s.Store.Unlock()
		fail(w, 409, "配置已更新，请刷新后重试")
		return
	}
	if req.Action == "subscription" && !s.checkSlug(w, p.Slug, p.ID) {
		s.Store.Unlock()
		return
	}
	p.Revision++
	if err = s.Store.Save(p); err != nil {
		s.Store.Unlock()
		fail(w, 500, "保存配置失败")
		return
	}
	if p.Kind == "source" {
		err = s.syncSubscriptions(p.ID)
	}
	s.Store.Unlock()
	if p.Kind == "template" {
		err = s.syncRoutingSubscriptions(r.Context(), p.ID, "")
	}
	if err != nil {
		fail(w, 500, "配置已保存，部分同步失败，请查看订阅状态")
		return
	}
	s.profileReply(w, p)
}

func showDiff(w http.ResponseWriter, base, next string) {
	diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{A: difflib.SplitLines(base), B: difflib.SplitLines(next), FromFile: "原配置", ToFile: "修改后", Context: 3})
	reply(w, 200, map[string]string{"diff": diff})
}

func (s *Server) nodeLink(w http.ResponseWriter, r *http.Request) {
	var req struct{ Link string }
	if !jsonBody(w, r, &req) {
		return
	}
	node, err := config.NodeLink(req.Link)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	reply(w, 200, node)
}

// 节点页一次读取来源配置的节点，不混入已生成的订阅。
func (s *Server) nodeGroups(w http.ResponseWriter, r *http.Request) {
	s.Store.Lock()
	defer s.Store.Unlock()
	list, err := s.Store.List()
	if err != nil {
		fail(w, 500, "读取配置分组失败")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, summary := range list {
		if summary.Kind != "source" {
			continue
		}
		p, err := s.Store.Get(summary.ID)
		if err != nil {
			fail(w, 500, "读取配置失败")
			return
		}
		doc, err := config.Parse(p.Draft)
		if err != nil {
			fail(w, 422, "配置无法解析")
			return
		}
		var data map[string]any
		_ = doc.Decode(&data)
		nodes := []map[string]any{}
		items, _ := data["proxies"].([]any)
		for _, raw := range items {
			node, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			nodes = append(nodes, map[string]any{"name": node["name"], "type": node["type"], "server": node["server"], "port": node["port"], "dialer-proxy": node["dialer-proxy"]})
		}
		out = append(out, map[string]any{"id": p.ID, "name": p.Name, "nodes": nodes})
	}
	reply(w, 200, out)
}
