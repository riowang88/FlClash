package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Settings struct {
	MixedPort    int      `json:"mixedPort"`
	AllowLan     bool     `json:"allowLan"`
	IPv6         bool     `json:"ipv6"`
	LogLevel     string   `json:"logLevel"`
	Mode         string   `json:"mode"`
	DNS          []string `json:"dns"`
	FakeIP       bool     `json:"fakeIP"`
	UserAgent    string   `json:"userAgent"`
	TestURL      string   `json:"testUrl"`
	PasswordHash string   `json:"passwordHash,omitempty"`
}

type Override struct {
	PrependRules  []string `json:"prependRules"`
	ExcludeFilter string   `json:"excludeFilter"`
}

type Profile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url,omitempty"`
	AutoHours int       `json:"autoHours"`
	UpdatedAt time.Time `json:"updatedAt"`
	LastTry   time.Time `json:"lastTry"`
	Upload    int64     `json:"upload"`
	Download  int64     `json:"download"`
	Total     int64     `json:"total"`
	Expire    int64     `json:"expire"`
	Nodes     int       `json:"nodes"`
	LastError string    `json:"lastError,omitempty"`
	Override  Override  `json:"override"`
}

type State struct {
	Settings Settings   `json:"settings"`
	Profiles []*Profile `json:"profiles"`
	Current  string     `json:"current"`
	Running  bool       `json:"running"`
}

type Store struct {
	mu   sync.Mutex
	dir  string
	data State
}

func defaultState() State {
	return State{
		Settings: Settings{
			MixedPort: 7890,
			AllowLan:  true,
			LogLevel:  "info",
			Mode:      "rule",
			DNS:       []string{"223.5.5.5", "119.29.29.29"},
			FakeIP:    true,
			UserAgent: "clash-verge/v2.0.0 FlClash-NAS",
			TestURL:   "https://www.gstatic.com/generate_204",
		},
		Running: true,
	}
}

func OpenStore(dir string) (*Store, error) {
	s := &Store{dir: dir, data: defaultState()}
	if err := os.MkdirAll(filepath.Join(dir, "profiles"), 0o755); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(s.statePath())
	if errors.Is(err, os.ErrNotExist) {
		return s, s.saveLocked()
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) statePath() string { return filepath.Join(s.dir, "state.json") }

func (s *Store) ProfilePath(id string) string {
	return filepath.Join(s.dir, "profiles", id+".yaml")
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.statePath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.statePath())
}

// Update runs fn on the state under the lock and persists the result.
func (s *Store) Update(fn func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(&s.data); err != nil {
		return err
	}
	return s.saveLocked()
}

// Snapshot returns a deep copy of the state.
func (s *Store) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(s.data)
	var out State
	_ = json.Unmarshal(b, &out)
	return out
}

func (st *State) Profile(id string) *Profile {
	for _, p := range st.Profiles {
		if p.ID == id {
			return p
		}
	}
	return nil
}
