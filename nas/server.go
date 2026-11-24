package main

import (
	"archive/zip"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	C "github.com/metacubex/mihomo/constant"
	"golang.org/x/crypto/bcrypt"
)

//go:embed web
var webFS embed.FS

const sessionCookie = "flclash_nas_session"

type Server struct {
	svc      *Service
	mu       sync.Mutex
	sessions map[string]time.Time
	core     *httputil.ReverseProxy
}

func NewServer(svc *Service) *Server {
	target, _ := url.Parse("http://" + controllerAddr)
	rp := httputil.NewSingleHostReverseProxy(target)
	rp.FlushInterval = -1
	base := rp.Director
	rp.Director = func(r *http.Request) {
		base(r)
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api/core")
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
		r.URL.RawPath = ""
		r.Header.Set("Authorization", "Bearer "+svc.secret)
		r.Header.Del("Cookie")
		r.Header.Del("Origin")
	}
	return &Server{svc: svc, sessions: map[string]time.Time{}, core: rp}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServerFS(sub))
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.HandleFunc("GET /api/status", s.auth(s.status))
	mux.HandleFunc("POST /api/running", s.auth(s.running))
	mux.HandleFunc("POST /api/mode", s.auth(s.mode))
	mux.HandleFunc("GET /api/settings", s.auth(s.getSettings))
	mux.HandleFunc("PUT /api/settings", s.auth(s.putSettings))
	mux.HandleFunc("POST /api/password", s.auth(s.password))
	mux.HandleFunc("GET /api/profiles", s.auth(s.listProfiles))
	mux.HandleFunc("POST /api/profiles", s.auth(s.addProfile))
	mux.HandleFunc("PATCH /api/profiles/{id}", s.auth(s.patchProfile))
	mux.HandleFunc("DELETE /api/profiles/{id}", s.auth(s.deleteProfile))
	mux.HandleFunc("POST /api/profiles/{id}/update", s.auth(s.updateProfile))
	mux.HandleFunc("POST /api/profiles/{id}/use", s.auth(s.useProfile))
	mux.HandleFunc("GET /api/export", s.auth(s.export))
	mux.HandleFunc("/api/core/", s.auth(s.core.ServeHTTP))
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 16<<20)).Decode(v)
}

func ok(w http.ResponseWriter, err error) {
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) passwordHash() string { return s.svc.store.Snapshot().Settings.PasswordHash }

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.passwordHash() != "" {
			c, err := r.Cookie(sessionCookie)
			s.mu.Lock()
			exp, valid := time.Time{}, false
			if err == nil {
				exp, valid = s.sessions[c.Value]
			}
			s.mu.Unlock()
			if !valid || time.Now().After(exp) {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "需要登录"})
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Password string }
	if err := readJSON(r, &in); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	h := s.passwordHash()
	if h != "" && bcrypt.CompareHashAndPassword([]byte(h), []byte(in.Password)) != nil {
		time.Sleep(600 * time.Millisecond)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "密码不对"})
		return
	}
	s.issueSession(w, false)
	ok(w, nil)
}

// issueSession sets a new session cookie; reset first drops every other
// session, so changing the password signs out other browsers.
func (s *Server) issueSession(w http.ResponseWriter, reset bool) {
	tok := randomID(24)
	s.mu.Lock()
	if reset {
		s.sessions = map[string]time.Time{}
	}
	s.sessions[tok] = time.Now().Add(30 * 24 * time.Hour)
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 30 * 86400})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	ok(w, nil)
}

func requestHost(r *http.Request) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return host
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	st := s.svc.store.Snapshot()
	var cur *Profile
	if p := st.Profile(st.Current); p != nil {
		cur = p
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"running":     s.svc.engine.Running(),
		"mode":        st.Settings.Mode,
		"host":        requestHost(r),
		"mixedPort":   st.Settings.MixedPort,
		"allowLan":    st.Settings.AllowLan,
		"passwordSet": st.Settings.PasswordHash != "",
		"current":     cur,
		"profiles":    len(st.Profiles),
		"uptime":      int(time.Since(s.svc.started).Seconds()),
		"coreVersion": C.Version,
		"version":     appVersion,
	})
}

func (s *Server) running(w http.ResponseWriter, r *http.Request) {
	var in struct{ Running bool }
	if err := readJSON(r, &in); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	ok(w, s.svc.SetRunning(in.Running))
}

func (s *Server) mode(w http.ResponseWriter, r *http.Request) {
	var in struct{ Mode string }
	if err := readJSON(r, &in); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	ok(w, s.svc.SetMode(in.Mode))
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	set := s.svc.store.Snapshot().Settings
	set.PasswordHash = ""
	writeJSON(w, http.StatusOK, set)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var in Settings
	if err := readJSON(r, &in); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	ok(w, s.svc.SaveSettings(in))
}

func (s *Server) password(w http.ResponseWriter, r *http.Request) {
	var in struct{ Password string }
	if err := readJSON(r, &in); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	hash := ""
	if in.Password != "" {
		b, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		hash = string(b)
	}
	err := s.svc.store.Update(func(st *State) error { st.Settings.PasswordHash = hash; return nil })
	if err == nil && hash != "" {
		s.issueSession(w, true)
	}
	ok(w, err)
}

func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request) {
	st := s.svc.store.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{"current": st.Current, "profiles": st.Profiles})
}

func (s *Server) addProfile(w http.ResponseWriter, r *http.Request) {
	var in NewProfile
	if err := readJSON(r, &in); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	p, err := s.svc.AddProfile(r.Context(), in)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) patchProfile(w http.ResponseWriter, r *http.Request) {
	var in ProfilePatch
	if err := readJSON(r, &in); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	ok(w, s.svc.PatchProfile(r.PathValue("id"), in))
}

func (s *Server) deleteProfile(w http.ResponseWriter, r *http.Request) {
	ok(w, s.svc.DeleteProfile(r.PathValue("id")))
}

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	ok(w, s.svc.UpdateProfile(r.Context(), r.PathValue("id")))
}

func (s *Server) useProfile(w http.ResponseWriter, r *http.Request) {
	ok(w, s.svc.UseProfile(r.PathValue("id")))
}

// export bundles state.json (without the password hash) and every profile.
func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	st := s.svc.store.Snapshot()
	st.Settings.PasswordHash = ""
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	sj, _ := json.MarshalIndent(st, "", "  ")
	f, _ := zw.Create("state.json")
	_, _ = f.Write(sj)
	for _, p := range st.Profiles {
		if b, err := os.ReadFile(s.svc.store.ProfilePath(p.ID)); err == nil {
			f, _ := zw.Create("profiles/" + p.ID + ".yaml")
			_, _ = f.Write(b)
		}
	}
	if err := zw.Close(); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="flclash-nas-backup-`+time.Now().Format("20060102")+`.zip"`)
	_, _ = w.Write(buf.Bytes())
}

func serve(ctx context.Context, addr string, h http.Handler) error {
	srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sc, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(sc)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
