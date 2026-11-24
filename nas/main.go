// Command flclash-nas runs a headless mihomo core with a web UI for NAS use:
// one container, one proxy port, subscriptions managed from the browser.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	C "github.com/metacubex/mihomo/constant"
	"golang.org/x/crypto/bcrypt"
)

var appVersion = "dev"

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	dataDir := env("FLCLASH_DATA", "/data")
	uiAddr := env("FLCLASH_UI_ADDR", ":8080")

	coreHome := filepath.Join(dataDir, "core")
	if err := os.MkdirAll(coreHome, 0o755); err != nil {
		log.Fatal(err)
	}
	C.SetHomeDir(coreHome)
	C.SetConfig(filepath.Join(coreHome, "config.yaml"))

	store, err := OpenStore(dataDir)
	if err != nil {
		log.Fatalf("打开数据目录失败：%v", err)
	}
	if pw := os.Getenv("FLCLASH_PASSWORD"); pw != "" && store.Snapshot().Settings.PasswordHash == "" {
		h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		if err != nil {
			log.Fatal(err)
		}
		_ = store.Update(func(st *State) error { st.Settings.PasswordHash = string(h); return nil })
	}

	engine := &Engine{}
	svc := NewService(store, engine)
	svc.Start()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if url := os.Getenv("FLCLASH_SUB_URL"); url != "" && len(store.Snapshot().Profiles) == 0 {
		if _, err := svc.AddProfile(ctx, NewProfile{URL: url, AutoHours: 24}); err != nil {
			log.Printf("导入 FLCLASH_SUB_URL 失败：%v", err)
		}
	}
	go svc.AutoUpdate(ctx)

	st := store.Snapshot()
	log.Printf("FlClash NAS %s（mihomo %s）管理界面 %s，代理端口 %d", appVersion, C.Version, uiAddr, st.Settings.MixedPort)
	if err := serve(ctx, uiAddr, NewServer(svc).Handler()); err != nil {
		log.Fatal(err)
	}
	engine.Shutdown()
}
