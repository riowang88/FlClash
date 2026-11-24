package main

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestParseSubInfo(t *testing.T) {
	info, ok := ParseSubInfo("upload=1073741824; download=2147483648; total=10737418240; expire=1796054400")
	if !ok || info.Upload != 1<<30 || info.Download != 2<<30 || info.Total != 10<<30 || info.Expire != 1796054400 {
		t.Fatalf("got %+v ok=%v", info, ok)
	}
	if _, ok := ParseSubInfo(""); ok {
		t.Fatal("empty header must not report info")
	}
	info, _ = ParseSubInfo("upload=1.5e3;download=0;total=0;expire=")
	if info.Upload != 1500 {
		t.Fatalf("float upload: %+v", info)
	}
}

func TestNameFromDisposition(t *testing.T) {
	cases := map[string]string{
		`attachment; filename="Airport.yaml"`:                  "Airport",
		`attachment; filename*=UTF-8''%E4%B8%BB%E5%8A%9B.yaml`: "主力",
		``: "",
	}
	for in, want := range cases {
		if got := NameFromDisposition(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestNormalizeProfileKeepsClashYAML(t *testing.T) {
	src := "proxies:\n  - {name: a, type: ss, server: 1.1.1.1, port: 1, cipher: aes-128-gcm, password: x}\nrules: [MATCH,DIRECT]\n"
	out, n, err := NormalizeProfile([]byte(src))
	if err != nil || n != 1 || string(out) != src {
		t.Fatalf("n=%d err=%v out=%s", n, err, out)
	}
}

func TestNormalizeProfileConvertsShareLinks(t *testing.T) {
	links := "trojan://pass@example.com:443?sni=example.com#HK%2001\n"
	for _, body := range []string{links, "dHJvamFuOi8vcGFzc0BleGFtcGxlLmNvbTo0NDM/c25pPWV4YW1wbGUuY29tI0hLJTIwMDEK"} {
		out, n, err := NormalizeProfile([]byte(body))
		if err != nil || n != 1 {
			t.Fatalf("n=%d err=%v", n, err)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(out, &doc); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), "HK 01") || doc["proxy-groups"] == nil {
			t.Fatalf("unexpected output:\n%s", out)
		}
	}
	if _, _, err := NormalizeProfile([]byte("<html>not a subscription</html>")); err == nil {
		t.Fatal("garbage must be rejected")
	}
}
