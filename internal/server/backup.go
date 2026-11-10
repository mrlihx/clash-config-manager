package server

import (
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	dir, err := os.MkdirTemp("", "clash-backup-")
	if err != nil {
		fail(w, 500, "创建备份失败")
		return
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "clash.db")
	s.Store.Lock()
	err = s.Store.Backup(path)
	s.Store.Unlock()
	if err != nil {
		fail(w, 500, "导出数据库失败")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="clash-backup-`+time.Now().Format("20060102-150405")+`.db"`)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, path)
}
