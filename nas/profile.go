package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/metacubex/mihomo/common/convert"
	"go.yaml.in/yaml/v3"
)

type SubInfo struct {
	Upload, Download, Total, Expire int64
}

// ParseSubInfo reads a subscription-userinfo header such as
// "upload=1; download=2; total=3; expire=1700000000".
func ParseSubInfo(h string) (SubInfo, bool) {
	var info SubInfo
	found := false
	for _, part := range strings.Split(h, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "upload":
			info.Upload, found = int64(n), true
		case "download":
			info.Download, found = int64(n), true
		case "total":
			info.Total, found = int64(n), true
		case "expire":
			info.Expire, found = int64(n), true
		}
	}
	return info, found
}

// NameFromDisposition extracts the file name a subscription advertises,
// preferring the RFC 5987 filename* form that carries non-ASCII names.
func NameFromDisposition(h string) string {
	if h == "" {
		return ""
	}
	if _, params, err := mime.ParseMediaType(h); err == nil {
		if n := params["filename"]; n != "" {
			return strings.TrimSuffix(strings.TrimSuffix(n, ".yaml"), ".yml")
		}
	}
	for _, part := range strings.Split(h, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.EqualFold(k, "filename*") {
			if i := strings.Index(v, "''"); i >= 0 {
				v = v[i+2:]
			}
			if s, err := url.QueryUnescape(v); err == nil {
				return strings.TrimSuffix(strings.TrimSuffix(s, ".yaml"), ".yml")
			}
		}
	}
	return ""
}

type Fetched struct {
	Body      []byte
	Name      string
	Info      SubInfo
	HasInfo   bool
	AutoHours int
}

func FetchSubscription(ctx context.Context, rawURL, ua string) (*Fetched, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("订阅服务器返回 HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	f := &Fetched{Body: body, Name: NameFromDisposition(resp.Header.Get("Content-Disposition"))}
	f.Info, f.HasInfo = ParseSubInfo(resp.Header.Get("Subscription-Userinfo"))
	if h, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Profile-Update-Interval"))); err == nil && h > 0 {
		f.AutoHours = h
	}
	return f, nil
}

// NormalizeProfile returns a Clash YAML document. Plain share-link lists
// (optionally base64 encoded) are converted into a minimal config.
func NormalizeProfile(body []byte) ([]byte, int, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(body, &doc); err == nil && doc != nil {
		if _, ok := doc["proxies"]; ok {
			return body, countProxies(doc), nil
		}
		if _, ok := doc["proxy-providers"]; ok {
			return body, countProxies(doc), nil
		}
	}
	links := body
	if dec, err := decodeBase64(strings.TrimSpace(string(body))); err == nil {
		links = dec
	}
	proxies, err := convert.ConvertsV2Ray(links)
	if err != nil || len(proxies) == 0 {
		return nil, 0, errors.New("无法识别的订阅内容：既不是 Clash 配置，也不是节点链接列表")
	}
	names := make([]any, 0, len(proxies))
	list := make([]any, 0, len(proxies))
	for _, p := range proxies {
		names = append(names, p["name"])
		list = append(list, p)
	}
	out := map[string]any{
		"proxies": list,
		"proxy-groups": []any{
			map[string]any{"name": "节点选择", "type": "select", "proxies": append([]any{"自动选择", "DIRECT"}, names...)},
			map[string]any{"name": "自动选择", "type": "url-test", "proxies": names, "interval": 300},
		},
		"rules": []any{"GEOIP,LAN,DIRECT,no-resolve", "GEOSITE,cn,DIRECT", "GEOIP,CN,DIRECT", "MATCH,节点选择"},
	}
	b, err := yaml.Marshal(out)
	return b, len(proxies), err
}

func decodeBase64(s string) ([]byte, error) {
	s = strings.NewReplacer("\n", "", "\r", "", " ", "").Replace(s)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, errors.New("not base64")
}

func countProxies(doc map[string]any) int {
	if l, ok := doc["proxies"].([]any); ok {
		return len(l)
	}
	return 0
}
