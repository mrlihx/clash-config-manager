package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"clash-config-manager/internal/store"
	"clash-config-manager/web"
	"github.com/skip2/go-qrcode"
)

const maxBody = 8 << 20
const sessionCookie = "clash_session"

type Server struct {
	Store      *store.Store
	root       string
	coreMu     sync.Mutex
	password   string
	passwordMu sync.RWMutex
	ConfigPath string
	core       string
}

func New(db *store.Store, password, core, root string) *Server {
	return &Server{Store: db, password: password, core: core, root: root}
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/password", s.auth(s.changePassword))
	mux.HandleFunc("GET /api/backup", s.auth(s.backup))
	mux.HandleFunc("GET /api/session", s.session)
	mux.HandleFunc("POST /api/logout", s.auth(s.logout))
	mux.HandleFunc("GET /api/subscription-qr", s.auth(func(w http.ResponseWriter, r *http.Request) {
		png, err := qrcode.Encode(r.URL.Query().Get("url"), qrcode.Medium, 256)
		if err != nil {
			http.Error(w, "订阅地址无法生成二维码", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(png)
	}))
	mux.HandleFunc("GET /api/routing-plans", s.auth(s.routingPlans))
	mux.HandleFunc("POST /api/routing-plans", s.auth(s.createRoutingPlan))
	mux.HandleFunc("DELETE /api/routing-plans/{id}", s.auth(s.deleteRoutingPlan))
	mux.HandleFunc("GET /api/system-template", s.auth(s.systemTemplate))
	mux.HandleFunc("GET /api/subscription-options", s.auth(s.subscriptionOptions))
	mux.HandleFunc("DELETE /api/subscriptions/{id}", s.auth(s.deleteSubscription))
	mux.HandleFunc("POST /api/subscriptions", s.auth(s.createSubscription))
	mux.HandleFunc("GET /api/node-groups", s.auth(s.nodeGroups))
	mux.HandleFunc("POST /api/sources/{id}", s.auth(s.editSource))
	mux.HandleFunc("DELETE /api/sources/{id}", s.auth(s.deleteSource))
	mux.HandleFunc("POST /api/sources/{id}/refresh", s.auth(s.updateSource))
	mux.HandleFunc("GET /api/profiles", s.auth(s.profiles))
	mux.HandleFunc("POST /api/profiles", s.auth(s.create))
	mux.HandleFunc("GET /api/profiles/{id}", s.auth(s.detail))
	mux.HandleFunc("POST /api/profiles/{id}", s.auth(s.change))
	mux.HandleFunc("POST /api/import", s.auth(s.importSubscription))
	mux.HandleFunc("POST /api/node-link", s.auth(s.nodeLink))
	mux.HandleFunc("GET /s/{slug}", s.subscription)
	mux.HandleFunc("GET /sub/{slug}", s.subscription)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, web.Assets, "html/index.html")
	})
	mux.Handle("GET /", http.FileServerFS(web.Assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") || (strings.HasPrefix(r.URL.Path, "/sub/") || strings.HasPrefix(r.URL.Path, "/s/")) {
			w.Header().Set("Cache-Control", "no-store")
		}
		mux.ServeHTTP(w, r)
	})
}

func jsonBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		fail(w, 415, "需要 JSON 请求")
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host {
			fail(w, 403, "请求来源不匹配")
			return false
		}
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	if err := d.Decode(dst); err != nil || d.Decode(new(any)) != io.EOF {
		fail(w, 400, "请求内容无效或超出大小限制")
		return false
	}
	return true
}

func reply(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func fail(w http.ResponseWriter, status int, message string) {
	reply(w, status, map[string]string{"error": message})
}
