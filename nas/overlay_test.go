package main

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

const sampleSub = `port: 7891
socks-port: 7892
external-controller: 0.0.0.0:9999
tun: {enable: true}
proxies:
  - {name: HK 01, type: ss, server: 1.1.1.1, port: 1, cipher: aes-128-gcm, password: x}
  - {name: 剩余流量 100G, type: ss, server: 1.1.1.1, port: 1, cipher: aes-128-gcm, password: x}
proxy-groups:
  - {name: PROXY, type: select, proxies: [HK 01, 剩余流量 100G]}
  - {name: ONLY-INFO, type: select, proxies: [剩余流量 100G]}
rules:
  - MATCH,PROXY
`

func build(t *testing.T, o Override) map[string]any {
	t.Helper()
	s := defaultState().Settings
	out, err := BuildConfig([]byte(sampleSub), s, o, "secret")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if err := Validate(out); err != nil {
		t.Fatalf("built config rejected by mihomo: %v\n%s", err, out)
	}
	return doc
}

func TestBuildConfigForcesNASInbound(t *testing.T) {
	doc := build(t, Override{})
	for _, k := range []string{"port", "socks-port", "tun"} {
		if _, ok := doc[k]; ok {
			t.Errorf("%s must be stripped", k)
		}
	}
	if doc["mixed-port"] != 7890 || doc["allow-lan"] != true || doc["external-controller"] != controllerAddr || doc["secret"] != "secret" {
		t.Fatalf("inbound not forced: %v", doc)
	}
	dns := doc["dns"].(map[string]any)
	if dns["enhanced-mode"] != "fake-ip" {
		t.Fatalf("dns: %v", dns)
	}
}

func TestBuildConfigOverride(t *testing.T) {
	doc := build(t, Override{PrependRules: []string{"# note", "DOMAIN-SUFFIX,nas.home.arpa,DIRECT", ""}, ExcludeFilter: "剩余"})
	rules := doc["rules"].([]any)
	if len(rules) != 2 || rules[0] != "DOMAIN-SUFFIX,nas.home.arpa,DIRECT" {
		t.Fatalf("rules: %v", rules)
	}
	if n := len(doc["proxies"].([]any)); n != 1 {
		t.Fatalf("proxies left: %d", n)
	}
	groups := doc["proxy-groups"].([]any)
	if got := groups[1].(map[string]any)["proxies"].([]any); len(got) != 1 || got[0] != "DIRECT" {
		t.Fatalf("emptied group must fall back to DIRECT: %v", got)
	}
}

func TestBuildConfigGeoSource(t *testing.T) {
	doc := build(t, Override{})
	if g := doc["geox-url"].(map[string]any)["geosite"]; g != geoMirror+"geosite.dat" {
		t.Fatalf("default geo source: %v", g)
	}
	out, err := BuildConfig([]byte("geox-url: {geosite: https://own.example/geosite.dat}\n"+sampleSub), defaultState().Settings, Override{}, "")
	if err != nil {
		t.Fatal(err)
	}
	var own map[string]any
	_ = yaml.Unmarshal(out, &own)
	if g := own["geox-url"].(map[string]any)["geosite"]; g != "https://own.example/geosite.dat" {
		t.Fatalf("subscription geo source must win: %v", g)
	}
}

func TestBuildConfigRejectsBadRegexp(t *testing.T) {
	if _, err := BuildConfig([]byte(sampleSub), defaultState().Settings, Override{ExcludeFilter: "("}, ""); err == nil {
		t.Fatal("bad regexp must fail")
	}
}

func TestDefaultProfileIsValid(t *testing.T) {
	out, err := BuildConfig([]byte(defaultProfile), defaultState().Settings, Override{}, "s")
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(out); err != nil {
		t.Fatal(err)
	}
}
