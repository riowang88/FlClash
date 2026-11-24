package main

import (
	"sync"

	"github.com/metacubex/mihomo/adapter/inbound"
	"github.com/metacubex/mihomo/config"
	"github.com/metacubex/mihomo/hub"
	"github.com/metacubex/mihomo/hub/executor"
	"github.com/metacubex/mihomo/listener"
	"github.com/metacubex/mihomo/tunnel"
)

// Engine owns the embedded mihomo instance. The FlClash fork of mihomo leaves
// listener creation out of ApplyConfig, so the engine starts them itself, as
// core/lifecycle.go does for the desktop app.
type Engine struct {
	mu      sync.Mutex
	cfg     *config.Config
	running bool
}

func Validate(buf []byte) error {
	_, err := executor.ParseWithBytes(buf)
	return err
}

func (e *Engine) Apply(buf []byte, running bool) error {
	cfg, err := executor.ParseWithBytes(buf)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	hub.ApplyConfig(cfg)
	e.cfg = cfg
	e.running = running
	e.syncListeners()
	return nil
}

func (e *Engine) SetRunning(running bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.running = running
	e.syncListeners()
}

func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

func (e *Engine) SetMode(mode string) {
	if m, ok := tunnel.ModeMapping[mode]; ok {
		tunnel.SetMode(m)
	}
}

func (e *Engine) syncListeners() {
	if e.cfg == nil || !e.running {
		listener.StopListener()
		return
	}
	g := e.cfg.General
	listener.SetAllowLan(g.AllowLan)
	inbound.SetSkipAuthPrefixes(g.SkipAuthPrefixes)
	inbound.SetAllowedIPs(g.LanAllowedIPs)
	inbound.SetDisAllowedIPs(g.LanDisAllowedIPs)
	listener.SetBindAddress(g.BindAddress)
	listener.ReCreateHTTP(0, tunnel.Tunnel)
	listener.ReCreateSocks(0, tunnel.Tunnel)
	listener.ReCreateMixed(g.MixedPort, tunnel.Tunnel)
}

func (e *Engine) Shutdown() {
	e.mu.Lock()
	defer e.mu.Unlock()
	listener.StopListener()
	executor.Shutdown()
}
