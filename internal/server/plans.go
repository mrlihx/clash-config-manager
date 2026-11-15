package server

import (
	"clash-config-manager/internal/store"
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) routingPlans(w http.ResponseWriter, r *http.Request) {
	if _, err := s.routingTemplate(); err != nil {
		fail(w, 500, "读取规则方案失败")
		return
	}
	s.Store.Lock()
	defer s.Store.Unlock()
	list, err := s.Store.List()
	if err != nil {
		fail(w, 500, "读取规则方案失败")
		return
	}
	result := []map[string]string{}
	for _, item := range list {
		if item.Kind == "template" {
			result = append(result, map[string]string{"id": item.ID, "name": item.Name})
		}
	}
	reply(w, 200, result)
}

func (s *Server) createRoutingPlan(w http.ResponseWriter, r *http.Request) {
	var req struct{ Name, CopyID string }
	if !jsonBody(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		fail(w, 400, "请输入方案名称")
		return
	}
	base, err := s.routingPlan(req.CopyID)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	p := &store.Profile{ID: "plan-" + randomToken(12), Kind: "template", Name: req.Name, Slug: randomSlug(), Revision: 1, Original: base.Draft, Draft: base.Draft}
	s.Store.Lock()
	err = s.Store.Save(p)
	s.Store.Unlock()
	if err != nil {
		fail(w, 500, "保存规则方案失败")
		return
	}
	s.profileReply(w, p)
}

func (s *Server) deleteRoutingPlan(w http.ResponseWriter, r *http.Request) {
	var req struct{ Revision int }
	if !jsonBody(w, r, &req) {
		return
	}
	id := r.PathValue("id")
	if id == templateID {
		fail(w, 400, "系统默认方案不能删除")
		return
	}
	s.Store.Lock()
	defer s.Store.Unlock()
	p, err := s.Store.Get(id)
	if err != nil || p.Kind != "template" {
		fail(w, 404, "规则方案不存在")
		return
	}
	if p.Revision != req.Revision {
		fail(w, 409, "方案已更新，请刷新")
		return
	}
	list, err := s.Store.List()
	if err != nil {
		fail(w, 500, "读取订阅失败")
		return
	}
	for _, item := range list {
		if item.Kind != "subscription" {
			continue
		}
		sub, e := s.Store.Get(item.ID)
		if e != nil {
			fail(w, 500, "读取订阅失败")
			return
		}
		var state subscriptionSync
		if sub.Sync != "" {
			_ = json.Unmarshal([]byte(sub.Sync), &state)
		}
		if state.RoutingPlan == id {
			fail(w, 400, "该方案仍被订阅引用，请先更换订阅方案")
			return
		}
	}
	if err = s.Store.DeletePlan(id); err != nil {
		fail(w, 500, "删除规则方案失败")
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}
