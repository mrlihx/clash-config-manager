package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"clash-config-manager/internal/server"
	"clash-config-manager/internal/store"
	"go.yaml.in/yaml/v3"
)

var version = "dev"

func main() {
	if run() {
		os.Exit(75)
	}
}

func run() bool {
	configPath := flag.String("config", "config.yaml", "项目配置文件")
	managed := flag.Bool("managed", false, "由启动器管理程序更新和重启")
	flag.Parse()
	file, err := os.Open(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	var settings struct {
		Listen        string
		Database      string
		Mihomo        string
		AdminPassword string `yaml:"admin-password"`
	}
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	err = decoder.Decode(&settings)
	file.Close()
	if err != nil {
		log.Fatal(err)
	}
	if settings.Listen == "" || settings.Database == "" || settings.Mihomo == "" {
		log.Fatal("config.yaml 需要 listen、database、mihomo")
	}
	password := settings.AdminPassword
	if len(password) < 5 {
		log.Fatal("请在 config.yaml 设置至少 5 字节的 admin-password")
	}
	root, err := filepath.Abs(filepath.Dir(*configPath))
	if err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp", settings.Listen)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	if err = server.PrepareCoreDirectory(root); err != nil {
		log.Fatal("迁移内核缓存失败：", err)
	}
	if !filepath.IsAbs(settings.Database) {
		settings.Database = filepath.Join(filepath.Dir(*configPath), settings.Database)
	}
	if err = store.MigrateLegacy(root, settings.Database); err != nil {
		log.Fatal("数据库迁移失败：", err)
	}

	db, err := store.Open(settings.Database)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	core := settings.Mihomo
	if !filepath.IsAbs(core) && strings.ContainsRune(core, filepath.Separator) {
		core = filepath.Join(filepath.Dir(*configPath), core)
	}
	core, _ = exec.LookPath(core)
	if core != "" {
		core, err = filepath.Abs(core)
		if err != nil {
			log.Fatal(err)
		}
	}
	app := server.New(db, password, core, root)
	app.ConfigPath, err = filepath.Abs(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := app.UpgradeSubscriptions(); err != nil {
		log.Fatal("升级默认策略组失败：", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var restarting atomic.Bool
	app.Version = version
	if *managed {
		app.Restart = func() { restarting.Store(true); stop() }
	}
	updaterDone := make(chan struct{})
	go func() { defer close(updaterDone); app.RunSourceUpdater(ctx) }()
	service := &http.Server{Addr: settings.Listen, Handler: app.Routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 75 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("Clash 配置管理器已启动，监听 %s", settings.Listen)
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := service.Shutdown(shutdownCtx); err != nil {
			_ = service.Close()
		}
	}()
	err = service.Serve(listener)
	stop()
	<-shutdownDone
	<-updaterDone
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("HTTP 服务停止：%v", err)
	}
	return restarting.Load()
}
