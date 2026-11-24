package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type Service struct {
	store  *Store
	engine *Engine
	secret string
	// applyMu serializes rebuild+apply so the last write wins in order.
	applyMu sync.Mutex
	started time.Time
}

func randomID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func NewService(store *Store, engine *Engine) *Service {
	return &Service{store: store, engine: engine, secret: randomID(16), started: time.Now()}
}

// Reload rebuilds the running config from the current profile and settings.
func (s *Service) Reload() error {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	st := s.store.Snapshot()
	raw := []byte(defaultProfile)
	var ov Override
	if p := st.Profile(st.Current); p != nil {
		b, err := os.ReadFile(s.store.ProfilePath(p.ID))
		if err != nil {
			return fmt.Errorf("读取订阅文件失败：%w", err)
		}
		raw, ov = b, p.Override
	}
	cfg, err := BuildConfig(raw, st.Settings, ov, s.secret)
	if err != nil {
		return err
	}
	return s.engine.Apply(cfg, st.Running)
}

// Start applies the saved state. A broken current profile falls back to the
// built-in default so the UI and proxy port still come up.
func (s *Service) Start() {
	if err := s.Reload(); err != nil {
		log.Printf("当前订阅无法加载，改用内置默认配置：%v", err)
		_ = s.store.Update(func(st *State) error { st.Current = ""; return nil })
		if err := s.Reload(); err != nil {
			log.Fatalf("内置默认配置加载失败：%v", err)
		}
	}
}

type NewProfile struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	AutoHours int    `json:"autoHours"`
	Content   string `json:"content"`
}

func (s *Service) AddProfile(ctx context.Context, in NewProfile) (*Profile, error) {
	p := &Profile{ID: randomID(6), Name: strings.TrimSpace(in.Name), URL: strings.TrimSpace(in.URL), AutoHours: in.AutoHours}
	var body []byte
	if p.URL != "" {
		f, err := FetchSubscription(ctx, p.URL, s.store.Snapshot().Settings.UserAgent)
		if err != nil {
			return nil, fmt.Errorf("下载订阅失败：%w", err)
		}
		body = f.Body
		if p.Name == "" {
			p.Name = f.Name
		}
		if f.HasInfo {
			p.Upload, p.Download, p.Total, p.Expire = f.Info.Upload, f.Info.Download, f.Info.Total, f.Info.Expire
		}
		if in.AutoHours < 0 && f.AutoHours > 0 {
			p.AutoHours = f.AutoHours
		}
	} else if strings.TrimSpace(in.Content) != "" {
		body = []byte(in.Content)
		p.AutoHours = 0
	} else {
		return nil, errors.New("请填写订阅链接或粘贴配置内容")
	}
	if p.AutoHours < 0 {
		p.AutoHours = 24
	}
	if p.Name == "" {
		p.Name = "订阅 " + time.Now().Format("01-02 15:04")
	}
	norm, n, err := NormalizeProfile(body)
	if err != nil {
		return nil, err
	}
	if err := s.validate(norm); err != nil {
		return nil, err
	}
	p.Nodes, p.UpdatedAt = n, time.Now()
	if err := os.WriteFile(s.store.ProfilePath(p.ID), norm, 0o600); err != nil {
		return nil, err
	}
	first := false
	err = s.store.Update(func(st *State) error {
		st.Profiles = append(st.Profiles, p)
		if st.Current == "" {
			st.Current, first = p.ID, true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if first {
		return p, s.Reload()
	}
	return p, nil
}

func (s *Service) validate(raw []byte) error {
	st := s.store.Snapshot()
	cfg, err := BuildConfig(raw, st.Settings, Override{}, s.secret)
	if err != nil {
		return err
	}
	if err := Validate(cfg); err != nil {
		return fmt.Errorf("配置校验失败：%w", err)
	}
	return nil
}

func (s *Service) UpdateProfile(ctx context.Context, id string) error {
	st := s.store.Snapshot()
	p := st.Profile(id)
	if p == nil {
		return errors.New("订阅不存在")
	}
	if p.URL == "" {
		return errors.New("本地配置没有订阅链接，无法更新")
	}
	f, err := FetchSubscription(ctx, p.URL, st.Settings.UserAgent)
	if err == nil {
		var norm []byte
		var n int
		if norm, n, err = NormalizeProfile(f.Body); err == nil {
			if err = s.validate(norm); err == nil {
				err = os.WriteFile(s.store.ProfilePath(id), norm, 0o600)
				p.Nodes = n
			}
		}
	}
	uerr := s.store.Update(func(st *State) error {
		q := st.Profile(id)
		if q == nil {
			return nil
		}
		q.LastTry = time.Now()
		if err != nil {
			q.LastError = err.Error()
			return nil
		}
		q.LastError, q.UpdatedAt, q.Nodes = "", time.Now(), p.Nodes
		if f.HasInfo {
			q.Upload, q.Download, q.Total, q.Expire = f.Info.Upload, f.Info.Download, f.Info.Total, f.Info.Expire
		}
		return nil
	})
	if err != nil {
		return err
	}
	if uerr != nil {
		return uerr
	}
	if st.Current == id {
		return s.Reload()
	}
	return nil
}

func (s *Service) UseProfile(id string) error {
	prev := ""
	return s.applyOrRevert(func(st *State) error {
		if st.Profile(id) == nil {
			return errors.New("订阅不存在")
		}
		prev, st.Current = st.Current, id
		return nil
	}, func(st *State) error { st.Current = prev; return nil })
}

// applyOrRevert persists mutate and reloads; when the new config fails to
// apply, revert restores the state so the last working config keeps running.
func (s *Service) applyOrRevert(mutate, revert func(*State) error) error {
	if err := s.store.Update(mutate); err != nil {
		return err
	}
	if err := s.Reload(); err != nil {
		_ = s.store.Update(revert)
		_ = s.Reload()
		return err
	}
	return nil
}

func (s *Service) DeleteProfile(id string) error {
	err := s.store.Update(func(st *State) error {
		if st.Current == id {
			return errors.New("正在使用的订阅不能删除，请先切换到其他订阅")
		}
		out := st.Profiles[:0]
		for _, p := range st.Profiles {
			if p.ID != id {
				out = append(out, p)
			}
		}
		st.Profiles = out
		return nil
	})
	if err != nil {
		return err
	}
	_ = os.Remove(s.store.ProfilePath(id))
	return nil
}

type ProfilePatch struct {
	Name      *string   `json:"name"`
	AutoHours *int      `json:"autoHours"`
	Override  *Override `json:"override"`
}

func (s *Service) PatchProfile(id string, in ProfilePatch) error {
	if in.Override != nil && in.Override.ExcludeFilter != "" {
		if _, err := BuildConfig([]byte(defaultProfile), Settings{}, *in.Override, ""); err != nil {
			return err
		}
	}
	var old Override
	mutate := func(st *State) error {
		p := st.Profile(id)
		if p == nil {
			return errors.New("订阅不存在")
		}
		old = p.Override
		if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
			p.Name = strings.TrimSpace(*in.Name)
		}
		if in.AutoHours != nil && *in.AutoHours >= 0 {
			p.AutoHours = *in.AutoHours
		}
		if in.Override != nil {
			p.Override = *in.Override
		}
		return nil
	}
	if in.Override == nil || s.store.Snapshot().Current != id {
		return s.store.Update(mutate)
	}
	return s.applyOrRevert(mutate, func(st *State) error {
		if p := st.Profile(id); p != nil {
			p.Override = old
		}
		return nil
	})
}

func (s *Service) SaveSettings(in Settings) error {
	if in.MixedPort <= 0 || in.MixedPort > 65535 {
		return errors.New("代理端口需在 1–65535 之间")
	}
	if _, ok := map[string]bool{"rule": true, "global": true, "direct": true}[in.Mode]; !ok {
		in.Mode = "rule"
	}
	var old Settings
	return s.applyOrRevert(func(st *State) error {
		old = st.Settings
		in.PasswordHash = st.Settings.PasswordHash
		st.Settings = in
		return nil
	}, func(st *State) error { st.Settings = old; return nil })
}

func (s *Service) SetMode(mode string) error {
	if _, ok := map[string]bool{"rule": true, "global": true, "direct": true}[mode]; !ok {
		return errors.New("未知模式")
	}
	s.engine.SetMode(mode)
	return s.store.Update(func(st *State) error { st.Settings.Mode = mode; return nil })
}

func (s *Service) SetRunning(running bool) error {
	s.engine.SetRunning(running)
	return s.store.Update(func(st *State) error { st.Running = running; return nil })
}

// AutoUpdate refreshes subscriptions whose interval has elapsed.
func (s *Service) AutoUpdate(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		st := s.store.Snapshot()
		for _, p := range st.Profiles {
			if p.URL == "" || p.AutoHours <= 0 {
				continue
			}
			due := p.UpdatedAt.Add(time.Duration(p.AutoHours) * time.Hour)
			if p.LastError != "" {
				due = p.LastTry.Add(30 * time.Minute)
			}
			if time.Now().Before(due) {
				continue
			}
			if err := s.UpdateProfile(ctx, p.ID); err != nil {
				log.Printf("自动更新订阅 %s 失败：%v", p.Name, err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
