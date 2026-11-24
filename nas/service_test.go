package main

import (
	"context"
	"os"
	"testing"

	C "github.com/metacubex/mihomo/constant"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	C.SetHomeDir(t.TempDir())
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Update(func(st *State) error { st.Running = false; return nil })
	svc := NewService(store, &Engine{})
	svc.Start()
	return svc
}

// testLinks avoids GEOIP/GEOSITE rules so the tests never download geo data.
const testLinks = "proxies:\n  - {name: HK, type: trojan, server: hk.example.invalid, port: 443, password: p}\nrules:\n  - MATCH,DIRECT\n"

func TestMixedPortFromEnvOnFirstStart(t *testing.T) {
	t.Setenv("FLCLASH_MIXED_PORT", "7777")
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p := store.Snapshot().Settings.MixedPort; p != 7777 {
		t.Fatalf("first start port = %d", p)
	}
	_ = store.Update(func(st *State) error { st.Settings.MixedPort = 8000; return nil })
	again, _ := OpenStore(dir)
	if p := again.Snapshot().Settings.MixedPort; p != 8000 {
		t.Fatalf("saved port must win over env: %d", p)
	}
}

func TestUseProfileRevertsOnBrokenConfig(t *testing.T) {
	svc := newTestService(t)
	good, err := svc.AddProfile(context.Background(), NewProfile{Name: "good", Content: testLinks})
	if err != nil {
		t.Fatal(err)
	}
	bad, err := svc.AddProfile(context.Background(), NewProfile{Name: "bad", Content: testLinks})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(svc.store.ProfilePath(bad.ID), []byte("rules: [NOT-A-RULE]\nproxies: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := svc.UseProfile(bad.ID); err == nil {
		t.Fatal("broken profile must fail to apply")
	}
	if cur := svc.store.Snapshot().Current; cur != good.ID {
		t.Fatalf("current = %q, want reverted to %q", cur, good.ID)
	}
}

func TestFailedUpdateKeepsUpdatedAt(t *testing.T) {
	svc := newTestService(t)
	p, err := svc.AddProfile(context.Background(), NewProfile{Name: "p", Content: testLinks})
	if err != nil {
		t.Fatal(err)
	}
	_ = svc.store.Update(func(st *State) error { st.Profile(p.ID).URL = "http://127.0.0.1:1/sub"; return nil })
	snap := svc.store.Snapshot()
	before := snap.Profile(p.ID).UpdatedAt
	if err := svc.UpdateProfile(context.Background(), p.ID); err == nil {
		t.Fatal("unreachable subscription must fail")
	}
	snap = svc.store.Snapshot()
	got := snap.Profile(p.ID)
	if !got.UpdatedAt.Equal(before) || got.LastTry.IsZero() || got.LastError == "" {
		t.Fatalf("updatedAt=%v (was %v) lastTry=%v lastError=%q", got.UpdatedAt, before, got.LastTry, got.LastError)
	}
}
