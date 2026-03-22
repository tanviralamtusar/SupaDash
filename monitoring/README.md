# SupaDash Monitoring Setup

## Quick Start

### 1. Add Prometheus + Grafana to your stack

Add to your `docker-compose.yaml` or run separately:

```yaml
services:
  prometheus:
    image: prom/prometheus:latest
    volumes:
      - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml
    ports:
      - "9090:9090"

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3001:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana-data:/var/lib/grafana

volumes:
  grafana-data:
```

### 2. Import the Dashboard

1. Open Grafana at `http://your-server:3001`
2. Login (default: `admin` / `admin`)
3. Go to **Connections** → **Data sources** → Add **Prometheus** → URL: `http://prometheus:9090`
4. Go to **Dashboards** → **Import** → Upload `monitoring/grafana-dashboard.json`
5. Select your Prometheus data source → **Import**

### 3. Dashboard Panels

| Panel | Type | Metric |
|-------|------|--------|
| **Uptime** | Stat | `supadash_uptime_seconds` |
| **Goroutines** | Stat + Graph | `supadash_go_goroutines` |
| **Heap Allocated** | Stat | `supadash_go_memstats_alloc_bytes` |
| **GC Cycles** | Stat | `supadash_go_gc_completed_total` |
| **Goroutines Over Time** | Time series | `supadash_go_goroutines` |
| **Memory Usage** | Time series | `alloc_bytes` vs `sys_bytes` |
| **GC Rate** | Bar chart | `rate(gc_completed_total[5m])` |
| **GC Cumulative** | Time series | `supadash_go_gc_completed_total` |
| **Heap Usage %** | Gauge | `alloc / sys * 100` |

### Thresholds

- **Goroutines**: Green < 100, Yellow < 500, Red ≥ 500
- **Heap Usage**: Green < 60%, Yellow < 80%, Orange < 90%, Red ≥ 90%
- **Heap Allocated**: Green < 100MB, Yellow < 500MB, Red ≥ 500MB
