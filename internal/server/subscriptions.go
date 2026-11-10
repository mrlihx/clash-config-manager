package server

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"clash-config-manager/internal/config"
	"clash-config-manager/internal/store"
)

func (s *Server) subscriptionOptions(w http.ResponseWriter, r *http.Request) {
	overseas := []config.RoutingCategory{}
	for _, category := range config.Categories {
		if !category.Direct {
			overseas = append(overseas, category)
		}
	}
	template, err := s.routingPlan(r.URL.Query().Get("plan"))
	if err != nil {
		fail(w, 500, "读取默认规则失败")
		return
	}
	reply(w, 200, map[string]any{"services": overseas, "ruleSets": templateRuleOptions(template)})
}

func (s *Server) createSubscription(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Selections []struct {
			ID    string
			Nodes []string
		}
		Services      []string
		RuleSets      []string
		RoutingPlan   string
		FollowRouting *bool
	}
	if !jsonBody(w, r, &req) {
		return
	}
	template, err := s.routingPlan(req.RoutingPlan)
	if err != nil {
		fail(w, 500, "读取默认规则失败")
		return
	}
	options := templateRuleOptions(template)
	for _, id := range req.RuleSets {
		if !slices.ContainsFunc(options, func(o config.RuleOption) bool { return o.ID == id }) {
			fail(w, 400, "选择了不存在的规则集")
			return
		}
	}
	selections := []config.NodeSelection{}
	syncState := subscriptionSync{RoutingPlan: template.ID, FollowRouting: req.FollowRouting, FollowAllRuleSets: allRuleOptions(options, req.RuleSets)}
	names := []string{}
	s.Store.Lock()
	for _, choice := range req.Selections {
		p, err := s.Store.Get(choice.ID)
		if err != nil || p.Kind != "source" {
			s.Store.Unlock()
			fail(w, 400, "来源配置不存在，请刷新节点列表")
			return
		}
		selections = append(selections, config.NodeSelection{Name: p.Name, Source: p.Draft, Nodes: choice.Nodes})
		data, parseErr := config.Validate(p.Draft)
		if parseErr != nil {
			s.Store.Unlock()
			fail(w, 422, parseErr.Error())
			return
		}
		syncState.Sources = append(syncState.Sources, sourceSelection{ID: p.ID, Name: p.Name, Nodes: choice.Nodes, All: len(choice.Nodes) == len(data["proxies"].([]any))})
		names = append(names, p.Name)
	}
	s.Store.Unlock()
	source, err := config.CollectNodes(selections)
	syncState.Baseline = source
	if err == nil {
		source, err = config.SubscriptionOptions(source, req.Services, req.RuleSets)
		if err == nil {
			source, err = config.ApplyRoutingTemplate(source, template.Draft, req.RuleSets)
		}
	}
	if err == nil {
		_, err = config.Validate(source)
	}
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	if err = s.checkCore(r.Context(), source); err != nil {
		fail(w, 422, err.Error())
		return
	}
	now := time.Now()
	p := &store.Profile{Kind: "subscription", ID: randomToken(16), Name: strings.Join(names, " + ") + " · " + now.Format("01-02 15:04:05"), Slug: randomSlug(), Enabled: true, Revision: 1, Original: source, Draft: source, Published: source, PublishedAt: now}
	syncState.RoutingBaseline = source
	syncJSON, _ := json.Marshal(syncState)
	p.Sync = string(syncJSON)
	s.Store.Lock()
	err = s.Store.Save(p)
	s.Store.Unlock()
	if err != nil {
		fail(w, 500, "保存订阅失败")
		return
	}
	s.profileReply(w, p)
}

func (s *Server) UpgradeSubscriptions() error {
	s.Store.Lock()
	defer s.Store.Unlock()
	list, err := s.Store.List()
	if err != nil {
		return err
	}
	for _, summary := range list {
		if summary.Kind != "subscription" && summary.Kind != "template" && !summary.Published {
			continue
		}
		p, err := s.Store.Get(summary.ID)
		if err != nil {
			return err
		}
		draft, err := config.UpgradeRoutingDefaults(p.Draft)
		if err == nil && p.Kind == "template" {
			draft, err = config.BindUnreferencedRuleProviders(draft)
		}
		if err == nil {
			draft, err = config.NormalizeLayout(draft)
		}
		if err != nil {
			return err
		}
		published := p.Published
		if published != "" {
			published, err = config.UpgradeRoutingDefaults(published)
			if err == nil {
				published, err = config.NormalizeLayout(published)
			}
			if err != nil {
				return err
			}
		}
		if draft == p.Draft && published == p.Published {
			continue
		}

		p.Draft, p.Published = draft, published
		p.Revision++
		if err = s.Store.Save(p); err != nil {
			return err
		}
	}
	return s.syncSubscriptions("")
}

func (s *Server) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	var req struct{ Revision int }
	if !jsonBody(w, r, &req) {
		return
	}
	s.Store.Lock()
	defer s.Store.Unlock()
	p, err := s.Store.Get(r.PathValue("id"))
	if err != nil || (p.Kind != "subscription" && (p.Kind != "source" || p.Published == "")) {
		fail(w, 404, "订阅不存在")
		return
	}
	if p.Revision != req.Revision {
		fail(w, 409, "订阅已更新，请刷新后重试")
		return
	}
	if err = s.Store.DeleteSubscription(p); err != nil {
		fail(w, 500, "删除订阅失败")
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}

func allRuleOptions(options []config.RuleOption, selected []string) bool {
	for _, option := range options {
		if !option.Required && !slices.Contains(selected, option.ID) {
			return false
		}
	}
	return true
}
