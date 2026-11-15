package server

import (
	"clash-config-manager/internal/config"
	"clash-config-manager/internal/store"
	"errors"
	"slices"
)

// 勾选仅控制订阅成员，节点信息仍由来源配置维护。
func (s *Server) selectSubscriptionNodes(p *store.Profile, source string, choices []sourceSelection, local []string) (string, subscriptionSync, error) {
	state, err := routingState(p)
	if err != nil {
		return "", state, err
	}
	// 已有关联来源保留顺序，新来源按本次选择顺序追加。
	order := map[string]int{}
	for i, ref := range state.Sources {
		order[ref.ID] = i
	}
	choices = slices.Clone(choices)
	slices.SortStableFunc(choices, func(a, b sourceSelection) int {
		ai, aok := order[a.ID]
		bi, bok := order[b.ID]
		if !aok {
			ai = len(order)
		}
		if !bok {
			bi = len(order)
		}
		return ai - bi
	})
	selections := []config.NodeSelection{}
	refs := []sourceSelection{}
	for _, choice := range choices {
		if len(choice.Nodes) == 0 {
			continue
		}
		s.Store.Lock()
		src, e := s.Store.Get(choice.ID)
		s.Store.Unlock()
		if e != nil || src.Kind != "source" {
			return "", state, errors.New("节点来源不存在，请刷新后重试")
		}
		selections = append(selections, config.NodeSelection{Name: src.Name, Source: src.Draft, Nodes: choice.Nodes})
		refs = append(refs, sourceSelection{ID: src.ID, Name: src.Name, Nodes: choice.Nodes, All: false})
	}
	baseline := "proxies: []\n"
	if len(selections) > 0 {
		baseline, err = config.CollectNodes(selections)
		if err != nil {
			return "", state, err
		}
	}
	if len(selections) == 0 && len(local) == 0 {
		return "", state, errors.New("请至少选择一个订阅节点")
	}
	selected := baseline
	if len(local) > 0 {
		selected, err = config.AppendNodeSnapshots(baseline, source, local)
		if err != nil {
			return "", state, err
		}
	}
	previous, err := config.NodeSource(source)
	if err != nil {
		return "", state, err
	}
	next, err := config.SyncNodes(source, previous, selected)
	state.Sources = refs
	state.Baseline = baseline
	return next, state, err
}
