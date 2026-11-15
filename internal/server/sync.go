package server

import (
	"clash-config-manager/internal/config"
	"clash-config-manager/internal/store"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
)

type sourceSelection struct {
	ID, Name string
	Nodes    []string
	All      bool
}
type subscriptionSync struct {
	Sources           []sourceSelection
	Baseline          string
	RoutingPlan       string
	RoutingBaseline   string
	FollowRouting     *bool
	FollowAllRuleSets bool
	PruneSnapshots    bool
}

// 调用方持有 Store 锁，保证同步结果不会覆盖同时发生的人工编辑。
func (s *Server) syncSubscriptions(sourceID string) error {
	list, err := s.Store.List()
	if err != nil {
		return err
	}
	sources := []*store.Profile{}
	for _, item := range list {
		if item.Kind == "source" {
			src, e := s.Store.Get(item.ID)
			if e != nil {
				return e
			}
			sources = append(sources, src)
		}
	}
	for _, item := range list {
		if item.Kind != "subscription" || (strings.HasPrefix(sourceID, "sub:") && item.ID != strings.TrimPrefix(sourceID, "sub:")) {
			continue
		}
		p, err := s.Store.Get(item.ID)
		if err != nil {
			return err
		}
		if err = s.bindLegacySubscription(p, sources); err != nil {
			return err
		}
		if p.Sync == "" {
			continue
		}
		var state subscriptionSync
		if err = json.Unmarshal([]byte(p.Sync), &state); err != nil {
			return err
		}
		if sourceID != "" && !strings.HasPrefix(sourceID, "sub:") && !slices.ContainsFunc(state.Sources, func(v sourceSelection) bool { return v.ID == sourceID }) {
			continue
		}
		if len(state.Sources) == 0 {
			continue
		}
		selections := []config.NodeSelection{}
		refs := []sourceSelection{}
		for _, ref := range state.Sources {
			src, e := s.Store.Get(ref.ID)
			if errors.Is(e, sql.ErrNoRows) || (e == nil && src.Kind != "source") {
				continue
			}
			if e != nil {
				return e
			}
			data, e := config.Validate(src.Draft)
			if e != nil {
				return e
			}
			names := []string{}
			for _, raw := range data["proxies"].([]any) {
				name := raw.(map[string]any)["name"].(string)
				if ref.All || slices.Contains(ref.Nodes, name) {
					names = append(names, name)
				}
			}
			if len(names) > 0 {
				selections = append(selections, config.NodeSelection{Name: ref.Name, Source: src.Draft, Nodes: names})
				ref.Nodes = names
				refs = append(refs, ref)
			}
		}
		baseline := "proxies: []\n"
		var e error
		if len(selections) > 0 {
			baseline, e = config.CollectNodes(selections)
			if e != nil {
				return e
			}
		}
		draft, e := config.SyncSubscriptionNodes(p.Draft, baseline)
		if e != nil {
			return fmt.Errorf("订阅 %s 节点同步失败: %w", p.Name, e)
		}
		published := p.Published
		if published != "" {
			published, e = config.SyncSubscriptionNodes(published, baseline)
			if e != nil {
				return e
			}
		}
		if draft == p.Draft && published == p.Published && baseline == state.Baseline {
			continue
		}

		if !state.PruneSnapshots {
			if _, e = s.saveBackup("before-node-cleanup"); e != nil {
				return e
			}
			state.PruneSnapshots = true
		}
		state.Baseline = baseline
		state.Sources = refs
		b, _ := json.Marshal(state)
		p.Sync = string(b)
		p.Draft, p.Published = draft, published
		p.Revision++
		p.PublishedAt = time.Now()
		if err = s.Store.Save(p); err != nil {
			return err
		}
	}
	return nil
}

// 旧订阅没有来源关系时，仅在原始节点能唯一对应来源时恢复关联。
func (s *Server) bindLegacySubscription(p *store.Profile, sources []*store.Profile) error {
	var state subscriptionSync
	if p.Sync != "" {
		if err := json.Unmarshal([]byte(p.Sync), &state); err != nil {
			return err
		}
		if len(state.Sources) > 0 || state.Baseline != "" {
			return nil
		}
	}
	original, err := config.Validate(p.Draft)
	if err != nil {
		return nil
	}
	nodes, _ := original["proxies"].([]any)
	if len(nodes) == 0 {
		return nil
	}
	refs := map[string]*sourceSelection{}
	sourceOrder := []string{}
	for _, raw := range nodes {
		node := raw.(map[string]any)
		var matched *store.Profile
		originalName := ""
		for _, src := range sources {
			data, e := config.Validate(src.Draft)
			if e != nil {
				continue
			}
			candidates, _ := data["proxies"].([]any)
			for _, candidate := range candidates {
				n := candidate.(map[string]any)
				a, b := map[string]any{}, map[string]any{}
				for k, v := range node {
					if k != "name" {
						a[k] = v
					}
				}
				for k, v := range n {
					if k != "name" {
						b[k] = v
					}
				}
				sameName := node["name"] == n["name"] || node["name"] == fmt.Sprint(n["name"])+" · "+src.Name
				if !sameName && !reflect.DeepEqual(a, b) {
					continue
				}
				if matched != nil {
					return nil
				}
				matched = src
				originalName = n["name"].(string)
			}
		}
		if matched == nil {
			continue
		}
		if refs[matched.ID] == nil {
			refs[matched.ID] = &sourceSelection{ID: matched.ID, Name: matched.Name}
			sourceOrder = append(sourceOrder, matched.ID)
		}
		refs[matched.ID].Nodes = append(refs[matched.ID].Nodes, originalName)
	}
	if len(refs) == 0 {
		return nil
	}
	state.Baseline = p.Draft
	for _, id := range sourceOrder {
		for _, src := range sources {
			if src.ID != id {
				continue
			}
			ref := refs[id]
			data, _ := config.Validate(src.Draft)
			ref.All = len(ref.Nodes) == len(data["proxies"].([]any))
			state.Sources = append(state.Sources, *ref)
		}
	}
	// 来源唯一匹配后重建关联；下一步同步会清除未匹配的旧快照。
	selections := []config.NodeSelection{}
	for _, ref := range state.Sources {
		for _, src := range sources {
			if src.ID == ref.ID {
				selections = append(selections, config.NodeSelection{Name: ref.Name, Source: src.Draft, Nodes: ref.Nodes})
			}
		}
	}
	baseline, e := config.CollectNodes(selections)
	if e != nil {
		return nil
	}
	state.Baseline = baseline
	b, _ := json.Marshal(state)
	p.Sync = string(b)
	return s.Store.Save(p)
}
