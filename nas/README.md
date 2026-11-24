# FlClash NAS

A single-container, headless build of FlClash for a NAS: the mihomo core from
`core/Clash.Meta`, subscription management ported from the FlClash app, and a
web UI. Other devices on the LAN use it as an HTTP/SOCKS5 proxy.

It replaces the `clash` + `yacd` pair: one image, one data directory, and
subscriptions are added, updated and overridden from the browser.

## Run

```yaml
# docker-compose.yml (also in this directory)
services:
  flclash-nas:
    image: ghcr.io/riowang88/flclash-nas:latest
    restart: unless-stopped
    ports: ["8080:8080", "7890:7890"]
    volumes: ["./data:/data"]
```

Open `http://<nas>:8080`, add a subscription, then point devices at
`http://<nas>:7890` (HTTP and SOCKS5 share the port).

| Variable | Default | Purpose |
|---|---|---|
| `FLCLASH_DATA` | `/data` | State, subscriptions, core cache and geo data |
| `FLCLASH_UI_ADDR` | `:8080` | Web UI listen address |
| `FLCLASH_PASSWORD` | – | Sets the UI password on first start if none is set |
| `FLCLASH_SUB_URL` | – | Subscription imported on first start |

GeoIP/GeoSite data downloads from the jsDelivr mirror of MetaCubeX/meta-rules-dat
unless the subscription sets its own `geox-url`; GitHub is often unreachable from
mainland China before the proxy is up.

The image on ghcr.io is private until the package visibility is set to public
in GitHub (Packages → flclash-nas → Package settings); otherwise log in to
ghcr.io on the NAS first.

## Layout

| File | Role |
|---|---|
| `engine.go` | Applies configs to the embedded mihomo and starts the mixed listener (the FlClash fork leaves listeners out of `ApplyConfig`) |
| `overlay.go` | Builds the running config: strips inbound/controller keys from the subscription, forces NAS settings, adds override rules and node exclusion |
| `profile.go` | Downloads subscriptions, reads `subscription-userinfo`, converts share-link lists |
| `service.go` | Profile lifecycle, auto update, settings, rollback when a config fails to apply |
| `server.go` | Session auth, `/api/*`, and `/api/core/*` reverse-proxied to the loopback mihomo controller |
| `web/` | The UI, embedded into the binary |

## Develop

```bash
make submodules            # once, from the repository root
cd nas
go test -tags with_gvisor ./...
FLCLASH_DATA=./data FLCLASH_UI_ADDR=:8080 go run -tags with_gvisor .
docker build -f nas/Dockerfile .   # from the repository root
```

Not included yet: TUN / transparent gateway mode, JavaScript override scripts,
and backup import (export exists).
