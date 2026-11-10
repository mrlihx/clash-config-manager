package server

import (
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const releaseAPI = "https://api.github.com/repos/mrlihx/clash-config-manager/releases/"

var releaseClient = &http.Client{Timeout: 30 * time.Second}
var downloadClient = &http.Client{Timeout: 10 * time.Minute}

type appRelease struct {
	Tag    string `json:"tag_name"`
	Commit string `json:"target_commitish"`
	URL    string `json:"html_url"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func readRelease(endpoint string) (*appRelease, error) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "clash-config-manager")
	response, err := releaseClient.Do(req)
	if err != nil {
		return nil, errors.New("无法连接 GitHub Releases，请检查服务器网络")
	}
	defer response.Body.Close()
	if response.StatusCode == 404 {
		return nil, errors.New("暂未发布此版本的可执行程序")
	}
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub Releases 返回 %d，请稍后重试", response.StatusCode)
	}
	var release appRelease
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release); err != nil {
		return nil, errors.New("版本信息无效")
	}
	return &release, nil
}

func (s *Server) appVersion(w http.ResponseWriter, r *http.Request) {
	reply(w, 200, map[string]any{"version": s.Version, "canUpdate": s.Restart != nil && runtime.GOOS == "linux"})
}

func (s *Server) checkUpdate(w http.ResponseWriter, r *http.Request) {
	release, err := readRelease(releaseAPI + "latest")
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	reply(w, 200, map[string]any{"version": s.Version, "tag": release.Tag, "commit": release.Commit, "url": release.URL, "available": release.Commit != s.Version})
}

func (s *Server) applyUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct{ Tag string }
	if !jsonBody(w, r, &req) {
		return
	}
	if s.Restart == nil || runtime.GOOS != "linux" {
		fail(w, 409, "当前启动方式不支持在线更新，请先使用新版 Docker 镜像部署")
		return
	}
	if !strings.HasPrefix(req.Tag, "build-") || len(req.Tag) > 64 || strings.ContainsAny(req.Tag, "/\\?#") {
		fail(w, 400, "请选择已发布的程序版本")
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(12 * time.Minute))
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	release, err := readRelease(releaseAPI + "tags/" + req.Tag)
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	if release.Commit == s.Version {
		fail(w, 409, "当前已经是此版本")
		return
	}
	filename := "clash-config-manager_linux_" + runtime.GOARCH
	binaryURL, checksumsURL := "", ""
	for _, asset := range release.Assets {
		if asset.Name == filename {
			binaryURL = asset.URL
		}
		if asset.Name == "SHA256SUMS" {
			checksumsURL = asset.URL
		}
	}
	if binaryURL == "" || checksumsURL == "" {
		fail(w, 422, "该版本缺少当前架构的程序或校验文件")
		return
	}
	response, err := releaseClient.Get(checksumsURL)
	if err != nil {
		fail(w, 502, "下载校验文件失败")
		return
	}
	checksums, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if response.StatusCode != 200 || readErr != nil {
		fail(w, 502, "下载校验文件失败")
		return
	}
	expected := ""
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == filename {
			expected = strings.ToLower(fields[0])
			break
		}
	}
	if decoded, err := hex.DecodeString(expected); err != nil || len(decoded) != sha256.Size {
		fail(w, 422, "程序校验信息无效")
		return
	}
	dir := filepath.Join(s.root, "data", "bin")
	if err = os.MkdirAll(dir, 0700); err != nil {
		fail(w, 500, "创建程序目录失败")
		return
	}
	file, err := os.CreateTemp(dir, ".update-*")
	if err != nil {
		fail(w, 500, "创建更新文件失败")
		return
	}
	defer os.Remove(file.Name())
	response, err = downloadClient.Get(binaryURL)
	if err != nil {
		file.Close()
		fail(w, 502, "下载程序失败，请检查服务器网络")
		return
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, (96<<20)+1))
	response.Body.Close()
	closeErr := file.Close()
	if response.StatusCode != 200 || copyErr != nil || closeErr != nil || size > 96<<20 {
		fail(w, 502, "程序下载失败或文件过大")
		return
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		fail(w, 422, "程序校验失败，已取消更新")
		return
	}
	executable, err := elf.Open(file.Name())
	if err != nil {
		fail(w, 422, "下载文件不是有效的 Linux 程序")
		return
	}
	machine := map[string]elf.Machine{"amd64": elf.EM_X86_64, "arm64": elf.EM_AARCH64}[runtime.GOARCH]
	compatible := executable.Machine == machine
	executable.Close()
	if !compatible {
		fail(w, 422, "程序架构不匹配，已取消更新")
		return
	}
	if err = os.Chmod(file.Name(), 0700); err != nil {
		fail(w, 500, "设置程序权限失败")
		return
	}
	s.Store.Lock()
	_, err = s.saveBackup("before-update")
	s.Store.Unlock()
	if err != nil {
		fail(w, 500, "更新前数据库备份失败，已取消更新")
		return
	}
	if err = os.Rename(file.Name(), filepath.Join(dir, "clash-config-manager")); err != nil {
		fail(w, 500, "替换程序失败，原程序保持不变")
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	s.Restart()
}
