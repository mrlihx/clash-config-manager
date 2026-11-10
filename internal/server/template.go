package server

import (
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strings"

	"clash-config-manager/internal/config"
	"clash-config-manager/internal/store"
)

const templateID = "system-routing-template"

func (s *Server) routingTemplate() (*store.Profile, error) {
	s.Store.Lock()
	defer s.Store.Unlock()
	p, err := s.Store.Get(templateID)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	source, err := config.Generate("proxies: []\n", "balanced", true)
	if err != nil {
		return nil, err
	}
	p = &store.Profile{Kind: "template", ID: templateID, Name: "系统默认规则", Slug: randomSlug(), Revision: 1, Original: source, Draft: source}
	return p, s.Store.Save(p)
}

func (s *Server) systemTemplate(w http.ResponseWriter, r *http.Request) {
	p, err := s.routingPlan(r.URL.Query().Get("id"))
	if err != nil {
		fail(w, 500, "读取系统默认模板失败")
		return
	}
	s.profileReply(w, p)
}

func templateRuleOptions(p *store.Profile) []config.RuleOption {
	options := slices.Clone(config.RuleOptions)
	data, err := config.Validate(p.Draft)
	if err != nil {
		return options
	}
	providers, _ := data["rule-providers"].(map[string]any)
	options = slices.DeleteFunc(options, func(option config.RuleOption) bool {
		key := "System_" + option.ID
		if option.ID == "Ads" {
			key = "Ads"
		}
		_, ok := providers[key]
		return !ok
	})
	for name := range providers {
		if strings.HasPrefix(name, "System_") || name == "Ads" {
			continue
		}
		options = append(options, config.RuleOption{ID: name, Name: name})
	}
	slices.SortFunc(options, func(a, b config.RuleOption) int { return strings.Compare(a.ID, b.ID) })
	return options
}
