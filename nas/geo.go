package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// geoKinds lists, per bundled file, every name mihomo also accepts for it.
var geoKinds = map[string][]string{
	"Country.mmdb": {"country.mmdb", "geoip.metadb", "geoip.db"},
	"GeoSite.dat":  {"geosite.dat"},
	"ASN.mmdb":     {"asn.mmdb"},
}

func seedGeoData(bundleDir, coreHome string) {
	if bundleDir == "" {
		return
	}
	existing := map[string]bool{}
	entries, _ := os.ReadDir(coreHome)
	for _, e := range entries {
		existing[strings.ToLower(e.Name())] = true
	}
	for name, aliases := range geoKinds {
		present := false
		for _, a := range aliases {
			present = present || existing[a]
		}
		if present {
			continue
		}
		if err := copyFile(filepath.Join(bundleDir, name), filepath.Join(coreHome, name)); err != nil {
			log.Printf("预置 %s 失败，将在需要时在线下载：%v", name, err)
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
