package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSeedGeoData(t *testing.T) {
	bundle, home := t.TempDir(), t.TempDir()
	for _, n := range []string{"Country.mmdb", "GeoSite.dat", "ASN.mmdb"} {
		if err := os.WriteFile(filepath.Join(bundle, n), []byte("bundled "+n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "geoip.metadb"), []byte("user"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedGeoData(bundle, home)
	if _, err := os.Stat(filepath.Join(home, "Country.mmdb")); !os.IsNotExist(err) {
		t.Fatal("an existing MMDB under another name must not be shadowed")
	}
	for _, n := range []string{"GeoSite.dat", "ASN.mmdb"} {
		if b, err := os.ReadFile(filepath.Join(home, n)); err != nil || string(b) != "bundled "+n {
			t.Fatalf("%s not seeded: %v %q", n, err, b)
		}
	}
	seedGeoData("", home)
}
