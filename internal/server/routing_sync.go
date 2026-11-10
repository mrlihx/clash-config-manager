package server

import (
	"clash-config-manager/internal/config"
	"clash-config-manager/internal/store"
	"context"
	"encoding/json"
	"errors"
	"time"
)

func routingState(p *store.Profile) (subscriptionSync, error) {
	var state subscriptionSync
	if p.Sync != "" {
		if err := json.Unmarshal([]byte(p.Sync), &state); err != nil {
			return state, err
		}
	}
	if state.RoutingPlan == "" {
		state.RoutingPlan = templateID
	}
	if state.RoutingBaseline == "" {
		_, sets, e := config.RoutingSelections(p.Original)
		if e == nil {
			state.FollowAllRuleSets = allRuleOptions(config.RuleOptions, sets)
		}
		state.RoutingBaseline = p.Original
		if state.RoutingBaseline == "" {
			state.RoutingBaseline = p.Draft
		}
	}
	return state, nil
}

// 校验在数据库锁外完成；每份订阅独立发布，失败时保留上次可用内容。
func (s *Server) syncRoutingSubscriptions(ctx context.Context, planID, subscriptionID string) error {
	s.Store.Lock()
	list, err := s.Store.List()
	s.Store.Unlock()
	if err != nil {
		return err
	}
	for _, item := range list {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if item.Kind != "subscription" || (subscriptionID != "" && item.ID != subscriptionID) {
			continue
		}
		s.Store.Lock()
		p, e := s.Store.Get(item.ID)
		s.Store.Unlock()
		if e != nil {
			return e
		}
		state, e := routingState(p)
		if e != nil {
			return e
		}
		if (state.FollowRouting != nil && !*state.FollowRouting) || (planID != "" && planID != state.RoutingPlan) {
			continue
		}
		template, e := s.routingPlan(state.RoutingPlan)
		var next, baseline string
		if e == nil {
			services, sets, parseErr := config.RoutingSelections(p.Draft)
			e = parseErr
			if state.FollowAllRuleSets {
				sets = nil
				for _, option := range templateRuleOptions(template) {
					sets = append(sets, option.ID)
				}
			}
			if e == nil {
				baseline, e = config.BuildRouting(p.Draft, template.Draft, services, sets)
			}
			if e == nil && baseline == state.RoutingBaseline && p.UpdateError == "" {
				continue
			}
			if e == nil {
				next, e = config.SyncRouting(p.Draft, state.RoutingBaseline, baseline)
			}
			if e == nil {
				e = s.checkCore(ctx, next)
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.Store.Lock()
		current, readErr := s.Store.Get(p.ID)
		if readErr != nil {
			s.Store.Unlock()
			return readErr
		}
		if current.Revision != p.Revision {
			s.Store.Unlock()
			continue
		}
		if e != nil {
			message := "规则同步失败：" + e.Error()
			if p.UpdateError != message {
				p.UpdateError = message
				p.Revision++
				err = s.Store.Save(p)
			}
		} else {
			state.RoutingBaseline = baseline
			b, _ := json.Marshal(state)
			p.Sync = string(b)
			p.UpdateError = ""
			p.Draft = next
			if p.Published != "" {
				p.Published = next
				p.PublishedAt = time.Now()
			}
			p.Revision++
			err = s.Store.Save(p)
		}
		s.Store.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) routingPlan(id string) (*store.Profile, error) {
	if id == "" || id == templateID {
		return s.routingTemplate()
	}
	s.Store.Lock()
	defer s.Store.Unlock()
	p, err := s.Store.Get(id)
	if err != nil || p.Kind != "template" {
		return nil, errors.New("规则方案不存在")
	}
	return p, nil
}
