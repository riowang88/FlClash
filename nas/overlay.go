package main

import (
	"fmt"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

const controllerAddr = "127.0.0.1:9090"

// geoMirror replaces mihomo's GitHub download source, which is often
// unreachable from a NAS in mainland China before any proxy is running.
const geoMirror = "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/"

const defaultProfile = `proxies: []
proxy-groups:
  - name: 节点选择
    type: select
    proxies: [DIRECT]
rules:
  - MATCH,节点选择
`

// strippedKeys are inbound and controller settings a subscription must not
// control on the NAS: only the mixed port listens, and the controller stays
// on loopback behind the web UI.
var strippedKeys = []string{
	"port", "socks-port", "redir-port", "tproxy-port", "tun", "listeners",
	"external-controller-tls", "external-controller-unix", "external-controller-pipe",
	"external-ui", "external-ui-url", "external-ui-name", "authentication",
}

// BuildConfig mirrors FlClash's _makeRealProfileTask for the NAS.
func BuildConfig(raw []byte, s Settings, o Override, secret string) ([]byte, error) {
	doc := map[string]any{}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("订阅不是合法的 YAML：%w", err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	for _, k := range strippedKeys {
		delete(doc, k)
	}
	doc["mixed-port"] = s.MixedPort
	doc["allow-lan"] = s.AllowLan
	doc["bind-address"] = "*"
	doc["ipv6"] = s.IPv6
	doc["mode"] = s.Mode
	doc["log-level"] = s.LogLevel
	doc["external-controller"] = controllerAddr
	doc["secret"] = secret
	doc["profile"] = map[string]any{"store-selected": true, "store-fake-ip": s.FakeIP}
	if _, ok := doc["geox-url"]; !ok {
		doc["geox-url"] = map[string]any{
			"geoip":   geoMirror + "geoip.dat",
			"geosite": geoMirror + "geosite.dat",
			"mmdb":    geoMirror + "country.mmdb",
			"asn":     geoMirror + "GeoLite2-ASN.mmdb",
		}
	}

	if len(s.DNS) > 0 {
		mode := "redir-host"
		if s.FakeIP {
			mode = "fake-ip"
		}
		dns := map[string]any{
			"enable":        true,
			"ipv6":          s.IPv6,
			"enhanced-mode": mode,
			"nameserver":    s.DNS,
		}
		if s.FakeIP {
			dns["fake-ip-range"] = "198.18.0.1/16"
			dns["fake-ip-filter"] = []string{"*.lan", "*.local", "*.home.arpa", "+.msftconnecttest.com", "time.*.com", "ntp.*.com"}
		}
		doc["dns"] = dns
	}

	if o.ExcludeFilter != "" {
		re, err := regexp.Compile(o.ExcludeFilter)
		if err != nil {
			return nil, fmt.Errorf("排除节点的正则无效：%w", err)
		}
		excludeProxies(doc, re)
	}
	if rules := cleanLines(o.PrependRules); len(rules) > 0 {
		existing, _ := doc["rules"].([]any)
		merged := make([]any, 0, len(rules)+len(existing))
		for _, r := range rules {
			merged = append(merged, r)
		}
		doc["rules"] = append(merged, existing...)
	}
	return yaml.Marshal(doc)
}

func excludeProxies(doc map[string]any, re *regexp.Regexp) {
	list, _ := doc["proxies"].([]any)
	removed := map[string]bool{}
	kept := list[:0]
	for _, p := range list {
		m, _ := p.(map[string]any)
		name, _ := m["name"].(string)
		if name != "" && re.MatchString(name) {
			removed[name] = true
			continue
		}
		kept = append(kept, p)
	}
	if list != nil {
		doc["proxies"] = kept
	}
	groups, _ := doc["proxy-groups"].([]any)
	for _, g := range groups {
		m, _ := g.(map[string]any)
		members, _ := m["proxies"].([]any)
		if members == nil {
			continue
		}
		out := members[:0]
		for _, n := range members {
			if s, _ := n.(string); !removed[s] {
				out = append(out, n)
			}
		}
		if len(out) == 0 {
			out = append(out, "DIRECT")
		}
		m["proxies"] = out
	}
}

func cleanLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out
}
