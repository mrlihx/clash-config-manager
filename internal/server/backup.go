package server

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"clash-config-manager/internal/store"
)

func (s *Server) backupPath(id string) (string, error) {
	if !filepath.IsLocal(id) || strings.ContainsAny(id, "/\\") || !strings.HasSuffix(id, ".db") {
		return "", os.ErrNotExist
	}
	return filepath.Join(s.root, "data", "backups", id), nil
}

// 调用方持有 Store 锁，让恢复前快照与随后恢复的操作保持连续。
func (s *Server) saveBackup(prefix string) (string, error) {
	dir := filepath.Join(s.root, "data", "backups")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	id := prefix + "-" + time.Now().Format("20060102-150405") + "-" + randomToken(6) + ".db"
	if err := s.Store.Backup(filepath.Join(dir, id)); err != nil {
		os.Remove(filepath.Join(dir, id))
		return "", err
	}
	return id, nil
}

func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	s.Store.Lock()
	id, err := s.saveBackup("manual")
	s.Store.Unlock()
	if err != nil {
		fail(w, 500, "创建备份失败")
		return
	}
	r.SetPathValue("id", id)
	s.downloadBackup(w, r)
}

func (s *Server) backups(w http.ResponseWriter, r *http.Request) {
	type item struct {
		ID        string    `json:"id"`
		Size      int64     `json:"size"`
		CreatedAt time.Time `json:"createdAt"`
		Source    string    `json:"source"`
	}
	result := []item{}
	entries, err := os.ReadDir(filepath.Join(s.root, "data", "backups"))
	if err != nil && !os.IsNotExist(err) {
		fail(w, 500, "读取备份列表失败")
		return
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".db") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			fail(w, 500, "读取备份信息失败")
			return
		}
		source := "手动备份"
		if strings.HasPrefix(entry.Name(), "upload-") {
			source = "上传备份"
		}
		if strings.HasPrefix(entry.Name(), "before-update-") {
			source = "更新前自动备份"
		}
		if strings.HasPrefix(entry.Name(), "before-restore-") {
			source = "恢复前自动备份"
		}
		result = append(result, item{entry.Name(), info.Size(), info.ModTime(), source})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	reply(w, 200, result)
}

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	if !jsonBody(w, r, new(struct{})) {
		return
	}
	s.Store.Lock()
	id, err := s.saveBackup("manual")
	s.Store.Unlock()
	if err != nil {
		fail(w, 500, "创建备份失败")
		return
	}
	reply(w, 200, map[string]string{"id": id})
}

func (s *Server) uploadBackup(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/octet-stream" {
		fail(w, 415, "请上传数据库文件")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host {
			fail(w, 403, "请求来源不匹配")
			return
		}
	}
	dir := filepath.Join(s.root, "data", "backups")
	if err := os.MkdirAll(dir, 0700); err != nil {
		fail(w, 500, "保存备份失败")
		return
	}
	file, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		fail(w, 500, "保存备份失败")
		return
	}
	defer os.Remove(file.Name())
	_, copyErr := io.Copy(file, http.MaxBytesReader(w, r.Body, 128<<20))
	closeErr := file.Close()
	if copyErr != nil {
		fail(w, 400, "上传失败，备份不能超过 128 MB")
		return
	}
	if closeErr != nil {
		fail(w, 500, "保存备份失败")
		return
	}
	if err = store.ValidateBackup(file.Name()); err != nil {
		fail(w, 422, "备份无效，请上传本系统导出的完整数据库文件")
		return
	}
	id := "upload-" + time.Now().Format("20060102-150405") + "-" + randomToken(6) + ".db"
	if err = os.Rename(file.Name(), filepath.Join(dir, id)); err != nil {
		fail(w, 500, "保存备份失败")
		return
	}
	reply(w, 200, map[string]string{"id": id})
}

func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	path, err := s.backupPath(r.PathValue("id"))
	if err != nil {
		fail(w, 404, "备份不存在")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(path)+`"`)
	http.ServeFile(w, r, path)
}

func (s *Server) deleteBackup(w http.ResponseWriter, r *http.Request) {
	if !jsonBody(w, r, new(struct{})) {
		return
	}
	path, err := s.backupPath(r.PathValue("id"))
	if err != nil {
		fail(w, 404, "备份不存在")
		return
	}
	if err = os.Remove(path); err != nil {
		fail(w, 404, "备份不存在或无法删除")
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	if !jsonBody(w, r, new(struct{})) {
		return
	}
	path, err := s.backupPath(r.PathValue("id"))
	if err != nil {
		fail(w, 404, "备份不存在")
		return
	}
	source, err := os.Open(path)
	if err != nil {
		fail(w, 404, "备份不存在")
		return
	}
	defer source.Close()
	dir, err := os.MkdirTemp("", "clash-restore-")
	if err != nil {
		fail(w, 500, "准备恢复失败")
		return
	}
	defer os.RemoveAll(dir)
	stagedPath := filepath.Join(dir, "restore.db")
	stagedFile, err := os.OpenFile(stagedPath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		fail(w, 500, "准备恢复失败")
		return
	}
	_, copyErr := io.Copy(stagedFile, source)
	closeErr := stagedFile.Close()
	if copyErr != nil || closeErr != nil {
		fail(w, 500, "准备恢复失败")
		return
	}
	// 在独立副本上迁移旧版结构，原备份保持原样。
	staged, err := store.Open(stagedPath)
	if err != nil {
		fail(w, 422, "备份数据库版本不兼容")
		return
	}
	err = staged.Close()
	if err != nil {
		fail(w, 500, "准备恢复失败")
		return
	}
	s.Store.Lock()
	defer s.Store.Unlock()
	safety, err := s.saveBackup("before-restore")
	if err != nil {
		fail(w, 500, "恢复前备份失败，已取消导入")
		return
	}
	if err = s.Store.Restore(stagedPath); err != nil {
		fail(w, 500, "恢复失败，原数据未改变")
		return
	}
	reply(w, 200, map[string]string{"backup": safety})
}
