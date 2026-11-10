package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"time"
)

func (s *Server) signedIn(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	return s.Store.HasSession(s.sessionHash(cookie.Value))
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.signedIn(r) {
			fail(w, 401, "请先登录")
			return
		}
		next(w, r)
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct{ Password string }
	if !jsonBody(w, r, &req) {
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.password)) != 1 {
		fail(w, 401, "密码不正确")
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.Store.DeleteSession(s.sessionHash(c.Value)); err != nil {
			fail(w, 500, "清除登录状态失败")
			return
		}
	}
	token := randomToken(32)
	expires := time.Now().Add(30 * 24 * time.Hour)
	if err := s.Store.SaveSession(s.sessionHash(token), expires); err != nil {
		fail(w, 500, "保存登录状态失败")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", Expires: expires, HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteStrictMode})
	reply(w, 200, map[string]bool{"ok": true})
}

func secure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !jsonBody(w, r, new(struct{})) {
		return
	}
	c, _ := r.Cookie(sessionCookie)
	if err := s.Store.DeleteSession(s.sessionHash(c.Value)); err != nil {
		fail(w, 500, "清除登录状态失败")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteStrictMode})
	reply(w, 200, map[string]bool{"ok": true})
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	reply(w, 200, map[string]bool{"authenticated": s.signedIn(r)})
}

// 使用管理员密码绑定会话，修改密码后旧登录自动失效。
func (s *Server) sessionHash(token string) string {
	mac := hmac.New(sha256.New, []byte(s.password))
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}
