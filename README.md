# ocserv-exporter

[![CI](https://github.com/mogilevich/ocserv_exporter/workflows/CI/badge.svg)](https://github.com/mogilevich/ocserv_exporter/actions/workflows/ci.yml)
[![Release](https://github.com/mogilevich/ocserv_exporter/workflows/Release/badge.svg)](https://github.com/mogilevich/ocserv_exporter/actions/workflows/release.yml)
[![Docker](https://github.com/mogilevich/ocserv_exporter/workflows/Docker/badge.svg)](https://github.com/mogilevich/ocserv_exporter/actions/workflows/docker.yml)
[![GitHub release](https://img.shields.io/github/v/release/mogilevich/ocserv_exporter)](https://github.com/mogilevich/ocserv_exporter/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Prometheus exporter for OpenConnect VPN Server (ocserv). Collects metrics from systemd journal logs and occtl status.

## Features

- Real-time log parsing from journald
- Multi-server support (e.g., `ocserv`, `ocserv-ru`)
- Per-user session tracking
- Traffic statistics (rx/tx bytes)
- Reconnect detection (login within 5 min of disconnect)
- Problematic session tracking (< 60s with error)
- Failed authentication attempts
- GeoIP support (optional)
- **occtl integration** (optional) - real-time server stats, VPN client types
- Ready-to-use Grafana dashboard

## Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `ocserv_active_sessions` | Gauge | server, username | Current active VPN sessions |
| `ocserv_connections_total` | Counter | server, username, client_ip | Total connections |
| `ocserv_disconnections_total` | Counter | server, username, reason | Total disconnections by reason |
| `ocserv_received_bytes_total` | Counter | server, username | Bytes received from clients |
| `ocserv_sent_bytes_total` | Counter | server, username | Bytes sent to clients |
| `ocserv_session_duration_seconds` | Histogram | server, username | Session duration distribution |
| `ocserv_reconnects_total` | Counter | server, username | Rapid reconnections (< 5 min) |
| `ocserv_problematic_sessions_total` | Counter | server, username, reason | Short sessions with errors |
| `ocserv_session_info` | Gauge | server, username, vpn_ip, country, client_type | Active session details (value is start timestamp) |
| `ocserv_auth_failed_total` | Counter | server, username, client_ip, country, country_code | Failed authentication attempts |
| `ocserv_connections_by_country_total` | Counter | server, username, country, country_code | Connections by country (GeoIP) |
| `ocserv_last_event_timestamp_seconds` | Gauge | - | Last processed log event timestamp |
| `ocserv_exporter_info` | Gauge | version | Exporter information |

### occtl metrics (optional)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `ocserv_server_rx_bytes_total` | Gauge | server | Total bytes received by server (real-time) |
| `ocserv_server_tx_bytes_total` | Gauge | server | Total bytes sent by server (real-time) |
| `ocserv_server_active_sessions` | Gauge | server | Active sessions from occtl |
| `ocserv_server_total_sessions` | Gauge | server | Total sessions since stats reset |
| `ocserv_server_latency_median_seconds` | Gauge | server | Median server latency |
| `ocserv_server_latency_stdev_seconds` | Gauge | server | Latency standard deviation |
| `ocserv_server_uptime_seconds` | Gauge | server | Server uptime |
| `ocserv_server_avg_session_time_seconds` | Gauge | server | Average session time |
| `ocserv_sessions_by_client_type` | Gauge | server, client_type | Sessions by VPN client type |
| `ocserv_user_concurrent_sessions` | Gauge | server, username | Current concurrent sessions per user |

## Installation

### From dist package

```bash
# Copy dist/ folder to server
scp -r dist/* user@vpn-server:/tmp/ocserv-exporter/

# Run install script
ssh user@vpn-server "cd /tmp/ocserv-exporter && ./install.sh"
```

### Manual installation

```bash
# Copy binary
sudo cp ocserv-exporter /usr/local/bin/
sudo chmod +x /usr/local/bin/ocserv-exporter

# Create service user
sudo useradd -r -s /sbin/nologin -G systemd-journal ocserv-exporter

# Copy systemd service
sudo cp ocserv-exporter.service /etc/systemd/system/

# Start service
sudo systemctl daemon-reload
sudo systemctl enable --now ocserv-exporter
```

## Configuration

### Command-line flags

```
--web.listen-address=":9617"    HTTP endpoint (default: :9617)
--web.telemetry-path="/metrics" Metrics path (default: /metrics)
--journal.unit="ocserv"         systemd unit to read (can be repeated)
--journal.since="24h"           Initial lookback period (default: 24h)
--geoip.db=""                   Path to GeoLite2-Country.mmdb (optional)
--log.file=""                   Read from file instead of journald (for testing)
--occtl.enabled                 Enable occtl polling for real-time server stats
--occtl.socket="name:path"      occtl socket (can be repeated, see below)
--occtl.interval="30s"          Polling interval (default: 30s)
```

### Systemd service

Edit `/etc/systemd/system/ocserv-exporter.service`:

```ini
ExecStart=/usr/local/bin/ocserv-exporter \
    --web.listen-address=:9617 \
    --journal.unit=ocserv \
    --journal.unit=ocserv-ru \
    --journal.since=24h
#   --geoip.db=/etc/ocserv-exporter/GeoLite2-Country.mmdb
```

### Multiple servers

If you have multiple ocserv instances, add `SyslogIdentifier` to each systemd service:

```ini
# /etc/systemd/system/ocserv-ru.service
[Service]
SyslogIdentifier=ocserv-ru
```

Then add both units to exporter:
```
--journal.unit=ocserv --journal.unit=ocserv-ru
```

## Prometheus configuration

Add to `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'ocserv'
    static_configs:
      - targets: ['vpn-server:9617']
```

## Grafana dashboard

Import `grafana/dashboard.json` to Grafana.

Dashboard includes:
- Active sessions overview
- Connections/disconnections over time
- Traffic by user
- Disconnect reasons pie chart
- Reconnects & problematic sessions
- Top users by traffic
- Failed authentication attempts
- Connections by country (GeoIP)

## GeoIP support

1. Register at https://www.maxmind.com/en/geolite2/signup
2. Download GeoLite2-Country.mmdb
3. Place it at `/etc/ocserv-exporter/GeoLite2-Country.mmdb`
4. Uncomment `--geoip.db` line in systemd service
5. Restart: `sudo systemctl restart ocserv-exporter`

## occtl integration (optional)

The exporter can poll `occtl` for real-time server statistics that are not available in logs:
- **Server RX/TX traffic** - total bytes in real-time (no spikes on disconnect)
- **Active sessions** - accurate count from server
- **Server latency** - median and stdev
- **VPN client types** - AnyConnect, OpenConnect-GUI, mobile clients, etc.

### Configuration

Socket format: `name` (for default socket) or `name:path` (for custom socket path).

```ini
ExecStart=/usr/local/bin/ocserv-exporter \
    --journal.unit=ocserv \
    --journal.unit=ocserv-ru \
    --occtl.enabled \
    --occtl.socket=ocserv \
    --occtl.socket=ocserv-ru:/var/run/ocserv-ru.socket \
    --occtl.interval=30s
```

### Permissions setup

The exporter uses `sudo` to run `occtl` (socket access requires root). Configure passwordless sudo for the service user:

```bash
# Find occtl path
which occtl  # typically /usr/local/bin/occtl

# Create sudoers file
sudo visudo -f /etc/sudoers.d/ocserv-exporter
```

Add this line (adjust path if needed):

```text
ocserv-exporter ALL=(root) NOPASSWD: /usr/local/bin/occtl *
```

Verify it works:
```bash
sudo -u ocserv-exporter sudo -n occtl show status
```

### Note on traffic metrics

Per-user traffic (`ocserv_received_bytes_total`, `ocserv_sent_bytes_total`) is only available at disconnect time - this is a limitation of ocserv logging, not the exporter. The `occtl` integration provides **server-level** traffic in real-time via `ocserv_server_rx_bytes_total` and `ocserv_server_tx_bytes_total`.

## Building

Requires Docker for cross-compilation (builds Linux amd64 binary):

```bash
./build.sh
```

Output in `dist/` folder.

## Verify installation

```bash
# Check service status
sudo systemctl status ocserv-exporter

# View logs
sudo journalctl -u ocserv-exporter -f

# Test metrics endpoint
curl localhost:9617/metrics | grep ocserv_
```

## License

MIT
