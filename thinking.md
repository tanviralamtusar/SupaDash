# Thinking — Self-Hosted Supabase Manager

> Research notes, Q&A findings, and feature ideas for building a self-hosted Supabase management platform.

---

## How Each Project Creates Projects

### SupaConsole — Full Working Pipeline

1. Clones the official [supabase/supabase](https://github.com/supabase/supabase) repo into a `supabase-core/` directory (one-time init)
2. When you create a project, it copies the `docker/` folder from `supabase-core` into a new `supabase-projects/<slug>/` directory
3. Customizes the `docker-compose.yml` — renames all containers (e.g., `supabase-studio` → `myproject-studio`) and generates unique port ranges
4. Generates a `.env` file with random secrets (JWT, Postgres password, etc.) and unique ports
5. Runs `docker compose pull` then `docker compose up -d` — actually spins up a full Supabase stack
6. Saves everything to the SQLite database

Key file: `SupaConsole/src/lib/project.ts` — the entire provisioning logic (~530 lines)

### supa-manager — Partially Implemented

1. Has a `provisioner.Provisioner` **interface** with a Docker implementation (`docker.go`)
2. When you create a project via the API (`POST /platform/projects`), it saves the project to PostgreSQL
3. The provisioner is supposed to generate a docker-compose from templates, allocate ports, generate secrets, and start containers
4. **But** the provisioning step is partially implemented — projects get created in the DB but the actual Supabase infrastructure doesn't fully spin up yet (their README explicitly warns about this)

Key files: `supa-manager/provisioner/docker.go`, `supa-manager/provisioner/provisioner.go`, `supa-manager/api/postPlatformProjects.go`

---

## Can We Re-Patch the Studio?

**Yes, absolutely.** The patching system in supa-manager is designed to be repeatable:

```
studio/
├── build.sh          # Downloads Studio source + applies patches + builds Docker image
├── patch.sh          # Applies the patch files
├── patches/          # The actual diff/patch files
└── files/            # Any replacement files
```

The command is: `./build.sh v1.24.04 supa-manager/studio:v1.24.04 .env`

**What the patches do:** They modify the Studio to talk to the supa-manager API instead of Supabase Cloud. Mainly changing API URLs, auth endpoints (`/auth/token` → Go backend), and removing cloud-only features.

**To patch a newer version:**
1. Update the version tag in `build.sh` (e.g., `v1.25.xx`)
2. Try applying the existing patches — they may apply cleanly or need adjustments
3. Fix any merge conflicts in the patches
4. Rebuild the Docker image

> ⚠️ The risk: newer Studio versions may change their API expectations, requiring new patches and new backend endpoints. But the system is built for exactly this workflow.

---

## Can We Mix Both Projects?

**Yes, and this is actually a very smart idea.** Here's how they complement each other:

| Take from SupaConsole | Take from supa-manager |
|---|---|
| ✅ Working provisioning logic (`project.ts`) | ✅ Patched Studio UI (official Supabase experience) |
| ✅ Port management & conflict avoidance | ✅ Go backend (faster, more scalable) |
| ✅ Pre-flight checks (Docker, internet) | ✅ PostgreSQL + sqlc (production DB) |
| ✅ Cross-platform support (Win/Linux) | ✅ Provisioner interface (extensible) |
| ✅ Detailed Docker error messages | ✅ 51 API endpoints (Studio-compatible) |
| ✅ Team management + SMTP emails | ✅ Security (Argon2, SECURITY.md) |
| | ✅ Helm charts for K8s |

### The Ideal Hybrid Approach

1. **Use supa-manager's Go backend + patched Studio** as the foundation (better architecture)
2. **Port SupaConsole's provisioning logic** into the Go provisioner (it actually works!)
3. **Keep the PostgreSQL database** from supa-manager (production-grade)
4. **Add SupaConsole's UX touches** — pre-flight checks, detailed error messages, SMTP team invitations

---

## Feature Idea: Resource Limiter

> Limit how much RAM, CPU, and storage each Supabase project can use. Scale up or down dynamically.

---

## Feature Idea: Unified Burst RAM Pool

> Every project gets a guaranteed (reserved) RAM. On top of that, a shared "unified pool" of extra RAM exists. During peak times, any project can burst into this shared pool for better performance.

### How It Works — Reservation vs. Limit

Docker natively supports this with two separate settings:
- **Reservation** = guaranteed minimum RAM, always available to the project
- **Limit** = maximum RAM allowed, can burst up to this using the shared pool

```yaml
deploy:
  resources:
    reservations:
      memory: 2G      # GUARANTEED — always yours, no matter what
    limits:
      memory: 4G      # MAX — can burst up to this from the shared pool
```

### Visual Example

```
Server: 32GB Total RAM
├── OS Reserved:      2GB  (always held for the host OS)
├── Unified Pool:     6GB  (shared burst pool for all projects)
├── Project A:        4GB reserved, can burst to 6GB  (+2GB from pool)
├── Project B:        4GB reserved, can burst to 6GB  (+2GB from pool)
├── Project C:        4GB reserved, can burst to 6GB  (+2GB from pool)
├── Project D:        4GB reserved, can burst to 6GB  (+2GB from pool)
                     ─────
                     16GB reserved + 6GB pool + 2GB OS = 24GB used (8GB headroom)
```

- Normal load: each project uses ~4GB (their reservation)
- Peak load: Project A spikes → it borrows from the 6GB unified pool → can use up to 6GB
- If **multiple** projects spike at the same time, they share the pool (first-come-first-served)
- If the pool is exhausted, projects are capped at their reservation

### Docker Implementation

```yaml
# Project A — Basic Plan (2GB reserved, burst to 4GB)
services:
  db:
    deploy:
      resources:
        reservations:
          memory: 1G
        limits:
          memory: 2G
  rest:
    deploy:
      resources:
        reservations:
          memory: 256M
        limits:
          memory: 512M
  # ... other services follow same pattern
```

### Capacity Math

```
Unified Pool = Total Server RAM - OS Reserved - Sum(all project reservations)

Example:
  Server RAM:         32GB
  OS Reserved:         2GB
  Project A reserved:  4GB
  Project B reserved:  4GB
  Project C reserved:  4GB
  ─────────────────────────
  Unified Pool = 32 - 2 - 4 - 4 - 4 = 18GB available for bursting
```

When creating a new project, the system checks:
```
Can we fit this? → Sum(existing reservations) + new reservation + OS ≤ Total RAM
Should we? → also ensure unified pool stays ≥ some minimum (e.g., 2GB)
```

### Priority Burst System (Advanced)

Not all projects should have equal burst access. Add a priority tier:

| Plan | Reserved | Max Burst | Burst Priority |
|------|----------|-----------|----------------|
| Free | 512MB | 1GB | Low (last to get pool access) |
| Basic | 2GB | 4GB | Medium |
| Pro | 4GB | 8GB | High (first to get pool access) |
| Enterprise | 8GB | 16GB | Critical (always gets pool) |

When the unified pool runs low:
1. **Critical** projects keep their burst allocation
2. **High** projects get reduced burst
3. **Medium** projects fall back to reservation only
4. **Low** projects are hard-capped at reservation

Docker doesn't natively support priority-based burst, but we can implement it by:
- Monitoring usage via Docker Stats API
- Dynamically adjusting `--memory` limits with `docker update`
- Running a background goroutine that rebalances every 30 seconds

### OOM (Out of Memory) Behavior

When a container exceeds its **limit**, Docker's OOM killer terminates the process. To handle this gracefully:
- Set `--oom-kill-disable=false` (default) — let Docker handle it
- Use `--memory-swap` equal to `--memory` to prevent swap thrashing
- Monitor OOM events and alert the user: "Project X hit its memory limit. Consider upgrading."
- Auto-restart killed containers with `restart: unless-stopped`

### What the UI Would Show

```
┌─── Server Resources ──────────────────────────────┐
│ Total RAM: 32GB                                    │
│ ████████████████████░░░░░░░░░░░░  24GB / 32GB     │
│                                                    │
│ Reserved by projects:  16GB  ██████████████████     │
│ Unified burst pool:    6GB   ██████                 │
│ OS reserved:           2GB   ██                     │
│ Free headroom:         8GB   ░░░░░░░░               │
│                                                    │
│ ┌── Project A (Pro) ────────────────────────┐      │
│ │ Reserved: 4GB  │ Using: 3.2GB │ Burst: 0  │      │
│ │ ████████████████░░░░                       │      │
│ └────────────────────────────────────────────┘      │
│ ┌── Project B (Basic) ──────────────────────┐      │
│ │ Reserved: 2GB  │ Using: 2.8GB │ Burst: +0.8GB│   │
│ │ ██████████████████████████                  │      │
│ └────────────────────────────────────────────┘      │
└────────────────────────────────────────────────────┘
```

---

## The Problem

When running multiple Supabase instances on one server, there's no way to control how much resources each project consumes. One rogue project can starve others of RAM/CPU/disk — causing crashes or degraded performance across the board.

## The Idea

Add a **resource limiter** that:
- Sets RAM, CPU, and storage caps per project
- Allows **scaling up or down** dynamically
- Prevents one project from consuming the entire server

---

## How Docker Resource Limits Work

Docker natively supports per-container constraints via `deploy.resources`:

```yaml
services:
  supabase-db:
    image: supabase/postgres
    deploy:
      resources:
        limits:
          memory: 2G         # Hard max RAM
          cpus: "1.0"        # Max CPU cores
        reservations:
          memory: 512M       # Guaranteed minimum
          cpus: "0.25"       # Guaranteed minimum
    storage_opt:
      size: "10G"            # Max disk usage (requires specific storage driver)
```

**Live scaling** — Docker allows updating limits on running containers:
```bash
docker update --memory="4g" --memory-swap="4g" myproject-db
docker update --cpus="2.0" myproject-db
```

Or via a compose recreate (safer, applies all changes):
```bash
docker compose up -d --force-recreate
```

---

## Plan / Tier System

### Predefined Plans

| Plan | RAM | CPU | Storage | Use Case |
|------|-----|-----|---------|----------|
| **Free** | 512MB | 0.5 cores | 1GB | Testing, demos |
| **Basic** | 2GB | 1.0 core | 5GB | Small apps, side projects |
| **Pro** | 4GB | 2.0 cores | 20GB | Production apps |
| **Enterprise** | 8GB+ | 4.0+ cores | 50GB+ | Heavy workloads |
| **Custom** | User-defined | User-defined | User-defined | Full control |

### Per-Service Breakdown

A single Supabase stack runs ~12 services. Each needs its own allocation:

| Service | Role | Min RAM | Typical RAM | Notes |
|---------|------|---------|-------------|-------|
| **PostgreSQL** (db) | Database | 256MB | 512MB–2GB | Heaviest — scales with data size |
| **PostgREST** (rest) | REST API | 64MB | 128MB–256MB | Lightweight, stateless |
| **GoTrue** (auth) | Authentication | 64MB | 128MB–256MB | Lightweight |
| **Kong** (gateway) | API Gateway | 128MB | 256MB–512MB | Handles all API traffic |
| **Studio** | Dashboard UI | 256MB | 256MB–512MB | Next.js app |
| **Realtime** | WebSockets | 128MB | 256MB–512MB | Scales with connections |
| **Storage** | File storage | 128MB | 128MB–256MB | S3-compatible |
| **ImgProxy** | Image transforms | 64MB | 128MB–256MB | CPU-intensive when active |
| **Edge Functions** | Serverless | 128MB | 256MB–512MB | Deno runtime |
| **Analytics** | Log collection | 128MB | 128MB–256MB | Logflare-based |
| **Pooler** | Connection pool | 64MB | 64MB–128MB | PgBouncer |
| **Vector** | Embeddings | 64MB | 128MB–256MB | Optional |

**Total minimum for one project:** ~1.5GB RAM
**Recommended for production:** ~3–4GB RAM

---

## Where It Fits in Each Project

### In SupaConsole

**Where:** `src/lib/project.ts` → `createProject()` function

SupaConsole already generates the `docker-compose.yml` during project creation. The resource limits would be injected into the compose file:

1. Add a `ResourcePlan` model to Prisma schema
2. UI: plan selector or sliders on project creation
3. On create → inject `deploy.resources` into each service in the generated compose file
4. On scale → update compose file + run `docker compose up -d --force-recreate`

**Pros:** Simple, works with existing shell-based approach
**Cons:** Editing YAML files is fragile; no live monitoring

### In supa-manager

**Where:** `supa-manager/provisioner/quotas.go` (already exists!)

supa-manager already has a `quotas.go` file — it was designed for this. The Go Docker SDK also supports `container.Resources{}` natively:

```go
resources := container.Resources{
    Memory:     2 * 1024 * 1024 * 1024, // 2GB
    NanoCPUs:   1000000000,              // 1.0 CPU
    MemorySwap: 2 * 1024 * 1024 * 1024,
}
```

**Pros:** Native Docker SDK, cleaner than YAML editing, existing quota infrastructure
**Cons:** Provisioning isn't fully working yet

---

## Full Feature Scope

### 1. Database Model

```sql
CREATE TABLE resource_plans (
    id SERIAL PRIMARY KEY,
    name VARCHAR(64) NOT NULL,            -- "Free", "Basic", "Pro", "Custom"
    total_memory_mb INT NOT NULL,          -- Total RAM across all services
    total_cpu_cores DECIMAL(4,2) NOT NULL, -- Total CPU cores
    total_storage_gb INT NOT NULL,         -- Total disk storage
    is_default BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE project_resources (
    id SERIAL PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    plan_id INT REFERENCES resource_plans(id),
    -- Per-service overrides (nullable = use plan defaults)
    db_memory_mb INT,
    db_cpu_cores DECIMAL(4,2),
    db_storage_gb INT,
    api_memory_mb INT,
    studio_memory_mb INT,
    -- ... other services
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE resource_usage_snapshots (
    id BIGSERIAL PRIMARY KEY,
    project_id TEXT NOT NULL,
    service_name VARCHAR(64) NOT NULL,
    memory_usage_mb INT,
    cpu_usage_percent DECIMAL(5,2),
    disk_usage_mb INT,
    recorded_at TIMESTAMPTZ DEFAULT NOW()
);
```

### 2. API Endpoints

```
GET    /projects/:ref/resources          → Get current limits & usage
PUT    /projects/:ref/resources          → Update limits (scale up/down)
GET    /projects/:ref/resources/usage    → Live resource usage (Docker stats)
GET    /projects/:ref/resources/history  → Historical usage data
GET    /resource-plans                   → List available plans
POST   /resource-plans                   → Create custom plan (admin)
```

### 3. Dashboard UI

- **Project creation** → Plan selector (Free / Basic / Pro / Custom)
- **Project settings** → Resource tab with:
  - Current plan display
  - RAM / CPU / Storage sliders for custom plans
  - "Scale Up" / "Scale Down" buttons
  - Usage graph (current vs. limit)
  - Per-service breakdown
- **Dashboard overview** → Server-wide resource utilization bar
- **Alerts** → Warning when a project hits 80%+ of limits

### 4. Monitoring (Docker Stats API)

```bash
# Get live stats for all containers in a project
docker stats --format json myproject-db myproject-rest myproject-auth ...
```

Go SDK equivalent:
```go
stats, err := cli.ContainerStats(ctx, containerID, true) // stream=true
```

This gives real-time: memory usage, CPU %, network I/O, disk I/O.

### 5. Dynamic Scaling

**Scale Up:**
1. User selects new plan or adjusts sliders
2. API validates the new limits (doesn't exceed server capacity)
3. Update database with new limits
4. Apply via `docker update` (instant, no downtime) or compose recreate
5. Return confirmation

**Scale Down:**
1. Check current usage vs. new limits
2. Warn if current usage exceeds new limits
3. Apply limits
4. Monitor for OOM kills and alert if needed

### 6. Server Capacity Guard

Before allowing a new project or scale-up:
```
Available = Total Server RAM - Sum(all project limits) - OS reserved (1GB)
if requested > available:
    reject("Insufficient resources. Available: X MB")
```

---

## Implementation Priority

| Priority | Task | Effort |
|----------|------|--------|
| P0 | Database schema for plans & resources | 1 day |
| P0 | Inject limits into docker-compose generation | 1–2 days |
| P0 | API endpoints for get/set limits | 1 day |
| P1 | Plan selector UI on project creation | 1 day |
| P1 | Live monitoring via Docker stats | 2 days |
| P1 | Scale up/down API + UI | 2 days |
| P2 | Usage history + graphs | 2–3 days |
| P2 | Server capacity guard | 1 day |
| P2 | Alerts (80% threshold) | 1 day |
| P3 | Per-service custom overrides | 2 days |

**Total estimate:** ~2 weeks for full feature

---

## Feature Idea: Detailed Resource Analysis

> A deep analytics dashboard showing exactly how each project, each service, and each container is consuming resources — with historical trends, anomaly detection, and optimization recommendations.

### Why This Matters

Resource limits and burst pools are **controls**. But without **visibility**, you're flying blind:
- Which service is eating the most RAM? (Is it Postgres or Realtime?)
- When do spikes happen? (3am cron jobs? User traffic peaks?)
- Is a project over-provisioned? (Paying for 4GB but only using 800MB)
- Is something leaking memory? (Gradual increase over days)

### What Docker Gives Us (Data Sources)

Docker Stats API returns real-time data per container:

```json
{
  "container": "myproject-db",
  "memory": {
    "usage": 524288000,        // 500MB current
    "limit": 2147483648,       // 2GB limit
    "percentage": 24.41
  },
  "cpu": {
    "percentage": 12.5         // 12.5% of allocated CPU
  },
  "network": {
    "rx_bytes": 1048576,       // 1MB received
    "tx_bytes": 2097152        // 2MB sent
  },
  "disk": {
    "read_bytes": 5242880,     // 5MB read
    "write_bytes": 10485760    // 10MB written
  }
}
```

Go SDK:
```go
stats, _ := dockerClient.ContainerStats(ctx, containerID, true) // stream=true
// Returns: MemoryStats, CPUStats, NetworkStats, BlkioStats
```

### Analysis Levels

#### Level 1: Server Overview

```
┌─── Server Health ──────────────────────────────────────────┐
│                                                            │
│  CPU ████████████░░░░░░░░░░░░░░  45% (4/8 cores used)    │
│  RAM ██████████████████░░░░░░░░  68% (22GB / 32GB)        │
│  DSK ████████████████░░░░░░░░░░  62% (310GB / 500GB)      │
│  NET ████░░░░░░░░░░░░░░░░░░░░░░  15% (150Mbps / 1Gbps)   │
│                                                            │
│  Active Projects: 5    │  Containers: 60   │  Uptime: 47d  │
└────────────────────────────────────────────────────────────┘
```

#### Level 2: Per-Project Breakdown

```
┌─── Project: my-saas-app (Pro Plan) ────────────────────────┐
│                                                             │
│  Total: 3.2GB / 4GB RAM  │  1.1 / 2.0 CPU  │  8.2 / 20GB  │
│                                                             │
│  Service          RAM        CPU     Disk    Status         │
│  ─────────────────────────────────────────────────────      │
│  postgresql    1,840 MB    42.3%    6.1 GB   ● Healthy      │
│  postgrest       128 MB     3.1%      —      ● Healthy      │
│  gotrue          112 MB     2.8%      —      ● Healthy      │
│  kong            256 MB     8.4%      —      ● Healthy      │
│  realtime        384 MB    12.1%      —      ● Healthy      │
│  studio          310 MB     6.2%    0.3 GB   ● Healthy      │
│  storage          96 MB     2.4%    1.8 GB   ● Healthy      │
│  edge-functions   74 MB     5.1%      —      ● Idle         │
│  analytics       102 MB     3.2%    0.2 GB   ● Healthy      │
│  ─────────────────────────────────────────────────────      │
│  TOTAL         3,302 MB    85.6%    8.4 GB                  │
│                                                             │
│  ⚠ PostgreSQL using 46% of total project RAM                │
│  💡 Recommendation: Consider increasing DB reservation      │
└─────────────────────────────────────────────────────────────┘
```

#### Level 3: Per-Service Deep Dive

For PostgreSQL specifically (the heaviest service):

```
┌─── PostgreSQL: my-saas-app-db ──────────────────────────────┐
│                                                              │
│  RAM:  1,840 MB / 2,048 MB (89.8%) ██████████████████░░     │
│  CPU:  42.3% / 100%                ████████░░░░░░░░░░░░     │
│  Disk: 6.1 GB / 10 GB             ████████████░░░░░░░░░     │
│                                                              │
│  Connections:  43 active / 100 max                           │
│  Queries/sec:  127                                           │
│  Slow queries: 3 (>1s)                                       │
│  Cache hit:    94.2%                                         │
│                                                              │
│  Top Tables by Size:                                         │
│    orders        2.1 GB   ████████████                       │
│    users         1.3 GB   ████████                           │
│    products      0.8 GB   █████                              │
│    sessions      0.4 GB   ███                                │
│    audit_logs    1.2 GB   ███████                            │
│                                                              │
│  ⚠ RAM at 89.8% — approaching limit!                        │
│  ⚠ 3 slow queries detected — see Query Analysis             │
└──────────────────────────────────────────────────────────────┘
```

### Historical Trends & Charts

Store snapshots every 30 seconds, aggregate into:

| Granularity | Retention | Use Case |
|-------------|-----------|----------|
| **30-second** raw | 24 hours | Real-time debugging |
| **5-minute** average | 7 days | Daily pattern analysis |
| **1-hour** average | 30 days | Weekly trends |
| **1-day** average | 1 year | Long-term capacity planning |

Charts to show:
- **RAM usage over time** — line chart per service, stacked area for project total
- **CPU usage over time** — same format
- **Disk growth** — line chart showing storage trend + projected full date
- **Network I/O** — bytes in/out per service
- **Burst pool usage** — how often and how much each project borrows

### Anomaly Detection

Automatically flag unusual patterns:

| Anomaly | Detection Rule | Alert |
|---------|---------------|-------|
| Memory leak | RAM increases >5% per hour consistently for 6+ hours | ⚠ "PostgreSQL RAM growing steadily — possible memory leak" |
| CPU spike | CPU >90% for >5 minutes | ⚠ "Kong API gateway CPU spike — check request volume" |
| Disk filling up | Disk usage growth rate → projected full within 7 days | ⚠ "Storage will be full in ~5 days at current rate" |
| Idle project | All services <5% CPU and <10% RAM for 7+ days | 💡 "Project appears idle — consider pausing to free resources" |
| Burst addiction | Project uses burst pool >50% of the time | 💡 "Project frequently needs burst RAM — consider upgrading plan" |

### Optimization Recommendations

Based on analysis data, auto-generate suggestions:

```
┌─── Optimization Report: my-saas-app ────────────────────────┐
│                                                              │
│  💰 Cost Savings                                             │
│  ─────────────                                               │
│  • Edge Functions using only 74MB of 512MB allocated         │
│    → Reduce to 128MB, save 384MB for other projects          │
│                                                              │
│  • Studio using 310MB — consider disabling in production     │
│    → Save 310MB (access Studio via a shared instance)        │
│                                                              │
│  ⚡ Performance Improvements                                  │
│  ────────────────────────                                    │
│  • PostgreSQL cache hit rate is 94.2%                        │
│    → Increase shared_buffers (add 256MB RAM) for 97%+ hit    │
│                                                              │
│  • 3 slow queries averaging 2.3 seconds                      │
│    → Add indexes on: orders.user_id, products.category_id    │
│                                                              │
│  • Connection pooler (PgBouncer) at 43/100 connections       │
│    → Current config is fine, no changes needed               │
│                                                              │
│  🔄 Rebalancing Suggestions                                  │
│  ─────────────────────────                                   │
│  • Move 256MB from edge-functions → postgresql               │
│  • This project is a good candidate for Pro → Basic          │
│    (only using 3.2GB of 4GB consistently)                    │
└──────────────────────────────────────────────────────────────┘
```

### Database Schema Addition

```sql
-- Add to existing resource tables
CREATE TABLE resource_analysis_snapshots (
    id BIGSERIAL PRIMARY KEY,
    project_id TEXT NOT NULL,
    service_name VARCHAR(64) NOT NULL,
    -- Resource metrics
    memory_usage_bytes BIGINT,
    memory_limit_bytes BIGINT,
    cpu_usage_percent DECIMAL(5,2),
    cpu_limit_cores DECIMAL(4,2),
    disk_read_bytes BIGINT,
    disk_write_bytes BIGINT,
    network_rx_bytes BIGINT,
    network_tx_bytes BIGINT,
    -- Container state
    container_status VARCHAR(32),      -- running, paused, exited
    restart_count INT DEFAULT 0,
    oom_killed BOOLEAN DEFAULT false,
    -- Metadata
    recorded_at TIMESTAMPTZ DEFAULT NOW()
);

-- Aggregated hourly stats (for historical views)
CREATE TABLE resource_analysis_hourly (
    id BIGSERIAL PRIMARY KEY,
    project_id TEXT NOT NULL,
    service_name VARCHAR(64) NOT NULL,
    hour TIMESTAMPTZ NOT NULL,
    -- Aggregated values
    avg_memory_usage_bytes BIGINT,
    max_memory_usage_bytes BIGINT,
    avg_cpu_percent DECIMAL(5,2),
    max_cpu_percent DECIMAL(5,2),
    total_disk_read_bytes BIGINT,
    total_disk_write_bytes BIGINT,
    total_network_rx_bytes BIGINT,
    total_network_tx_bytes BIGINT,
    -- Burst pool
    burst_pool_usage_bytes BIGINT,
    burst_pool_duration_seconds INT,
    -- Anomalies
    oom_kill_count INT DEFAULT 0,
    restart_count INT DEFAULT 0,
    UNIQUE(project_id, service_name, hour)
);

-- Optimization recommendations (generated periodically)
CREATE TABLE resource_recommendations (
    id SERIAL PRIMARY KEY,
    project_id TEXT NOT NULL,
    type VARCHAR(32) NOT NULL,         -- 'cost_saving', 'performance', 'rebalance', 'alert'
    severity VARCHAR(16) NOT NULL,     -- 'info', 'warning', 'critical'
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    potential_savings_mb INT,           -- RAM that could be freed
    is_dismissed BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    dismissed_at TIMESTAMPTZ
);
```

### API Endpoints

```
GET  /projects/:ref/analysis                    → Full analysis summary
GET  /projects/:ref/analysis/realtime           → WebSocket stream of live stats
GET  /projects/:ref/analysis/history?range=7d   → Historical data (1h/1d/7d/30d)
GET  /projects/:ref/analysis/services/:service  → Deep dive into one service
GET  /projects/:ref/analysis/recommendations    → Optimization suggestions
POST /projects/:ref/analysis/recommendations/:id/dismiss → Dismiss a suggestion
GET  /server/analysis                           → Server-wide overview
GET  /server/analysis/capacity                  → Capacity planning data
```

### Background Collector (Go Goroutine)

```go
func (a *AnalysisCollector) Run(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    aggregateTicker := time.NewTicker(1 * time.Hour)
    recommendTicker := time.NewTicker(6 * time.Hour)

    for {
        select {
        case <-ticker.C:
            a.collectSnapshots()     // Collect raw stats from all containers
        case <-aggregateTicker.C:
            a.aggregateHourly()      // Roll up raw → hourly summaries
            a.cleanupOldData()       // Delete raw data older than 24h
        case <-recommendTicker.C:
            a.generateRecommendations()  // Analyze trends, create suggestions
            a.detectAnomalies()          // Flag unusual patterns
        case <-ctx.Done():
            return
        }
    }
}
```

### Exportable Reports

- **PDF report** — Monthly resource usage report per project (for billing/auditing)
- **CSV export** — Raw data export for custom analysis
- **Webhook alerts** — Send anomaly alerts to Slack/Discord/email

---

# 🚀 Production Planning

> The roadmap for building a production-grade self-hosted Supabase management platform — combining the best of SupaConsole and supa-manager with our new features.

---

## Decision: What Are We Building?

### Project Name: **SupaDash**

> Dashboard for managing self-hosted Supabase instances — provisioning, resource control, and analytics in one place.

### Foundation Decision

**Build on supa-manager** (Go backend + patched Studio) and port SupaConsole's working provisioning logic into it.

**Why:**
- Go is faster, compiles to a single binary, handles concurrency natively
- PostgreSQL is production-grade (vs. SQLite)
- Patched Studio gives the real Supabase experience
- 51 existing API endpoints already match Studio expectations
- Provisioner interface is already abstracted — just needs the implementation
- Helm chart = Kubernetes-ready from day one

**What we take from SupaConsole:**
- Proven provisioning logic (clone repo → copy docker → customize → deploy)
- Smart port management
- Pre-flight checks (Docker, internet connectivity)
- Cross-platform Docker commands
- Detailed error handling
- Team management / SMTP emails

---

## Tech Stack (Final)

| Layer | Technology | Why |
|-------|-----------|-----|
| **Backend API** | Go 1.24 + Gin | Fast, compiled, native Docker SDK |
| **Frontend** | Patched Supabase Studio (Next.js) | Official UI, familiar UX |
| **Management DB** | PostgreSQL 15 | Production-grade, replication-ready |
| **DB Queries** | sqlc | Type-safe, no ORM overhead |
| **Auth** | JWT (golang-jwt) + Argon2 | Secure, industry-standard |
| **Provisioning** | Docker SDK (Go) + Docker Compose | Programmatic container management |
| **Monitoring** | Docker Stats API + custom collector | Real-time resource analysis |
| **Deployment** | Docker Compose (dev), Helm/K8s (prod) | Scale from single server to cluster |
| **CI/CD** | GitHub Actions | Automated testing + deployment |

---

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                        Load Balancer / Reverse Proxy             │
│                          (Nginx / Traefik / Caddy)               │
└─────────────────────┬───────────────────┬────────────────────────┘
                      │                   │
            ┌─────────▼──────────┐  ┌─────▼──────────────┐
            │   Studio UI (:3000) │  │   Admin API (:8080) │
            │   (Patched Next.js) │  │   (Go + Gin)        │
            └─────────┬──────────┘  └──┬──────┬───────────┘
                      │                │      │
                      └───────┬────────┘      │
                              │               │
                    ┌─────────▼──────────┐    │
                    │  Management DB      │    │
                    │  (PostgreSQL 15)    │    │
                    └────────────────────┘    │
                                              │
              ┌───────────────────────────────▼─────────────┐
              │            Provisioner Engine                 │
              │  ┌────────────┐  ┌────────────────────────┐ │
              │  │ Docker SDK │  │ Resource Manager        │ │
              │  │            │  │ (Limits + Burst Pool +  │ │
              │  │            │  │  Analysis Collector)    │ │
              │  └──────┬─────┘  └───────────┬────────────┘ │
              └─────────┼────────────────────┼──────────────┘
                        │                    │
         ┌──────────────▼────────────────────▼──────────────┐
         │              Docker Host / Cluster                │
         │                                                   │
         │  ┌─── Project A ──────┐  ┌─── Project B ──────┐ │
         │  │ postgres           │  │ postgres           │  │
         │  │ postgrest          │  │ postgrest          │  │
         │  │ gotrue             │  │ gotrue             │  │
         │  │ kong               │  │ kong               │  │
         │  │ studio             │  │ studio             │  │
         │  │ realtime           │  │ realtime           │  │
         │  │ storage            │  │ storage            │  │
         │  │ ...                │  │ ...                │  │
         │  └────────────────────┘  └────────────────────┘  │
         └──────────────────────────────────────────────────┘
```

---

## Repository Structure

```
supadash/
├── .github/
│   └── workflows/
│       ├── test.yml                 # Run tests on PR
│       ├── build.yml                # Build Docker images
│       └── release.yml              # Tag → deploy
│
├── api/                             # Go backend
│   ├── main.go
│   ├── api/                         # Route handlers (from supa-manager)
│   ├── conf/                        # Configuration
│   ├── database/                    # sqlc generated queries
│   ├── migrations/                  # SQL migration files
│   ├── provisioner/                 # Docker provisioning engine
│   │   ├── provisioner.go           # Interface
│   │   ├── docker.go                # Docker implementation
│   │   ├── quotas.go                # Resource limits & burst pool
│   │   ├── analysis.go              # Resource analysis collector
│   │   └── backup.go                # Backup management
│   ├── queries/                     # sqlc query definitions
│   ├── utils/                       # Shared utilities
│   ├── templates/                   # Docker compose templates
│   ├── Dockerfile
│   ├── go.mod
│   └── go.sum
│
├── studio/                          # Patched Supabase Studio
│   ├── build.sh
│   ├── patch.sh
│   ├── patches/
│   └── files/
│
├── helm/                            # Kubernetes deployment
│   └── supadash-chart/
│
├── docker-compose.yml               # Dev/single-server deployment
├── docker-compose.prod.yml          # Production overrides
├── .env.example
├── SECURITY.md
├── README.md
└── docs/
    ├── architecture.md
    ├── api-reference.md
    ├── deployment-guide.md
    └── resource-management.md
```

---

## Phased Development Roadmap

### Phase 1: Foundation (Weeks 1–2)
> Get the combined project running with basic provisioning.

| # | Task | Source | Effort |
|---|------|--------|--------|
| 1.1 | Fork supa-manager → new repo | — | 1 day |
| 1.2 | Clean up codebase (remove stale TODOs, commented code) | supa-manager | 1 day |
| 1.3 | Port SupaConsole's provisioning logic into Go provisioner | SupaConsole | 3 days |
| 1.4 | Implement: clone Supabase repo → copy docker → customize compose | SupaConsole | 2 days |
| 1.5 | Implement: unique port allocation + container naming | SupaConsole | 1 day |
| 1.6 | Wire up: `POST /platform/projects` → actually provisions | Both | 1 day |
| 1.7 | Test: create project via Studio → full stack spins up | — | 1 day |

**Milestone:** Studio UI can create and spin up real Supabase projects ✅

---

### Phase 2: Project Lifecycle (Weeks 3–4)
> Complete CRUD operations for projects.

| # | Task | Effort |
|---|------|--------|
| 2.1 | Implement `POST /projects/:ref/pause` (docker compose stop) | 1 day |
| 2.2 | Implement `POST /projects/:ref/resume` (docker compose start) | 1 day |
| 2.3 | Implement `DELETE /projects/:ref` (stop + remove + cleanup) | 1 day |
| 2.4 | Add pre-flight checks (Docker available, internet, disk space) | 1 day |
| 2.5 | Add detailed error messages (from SupaConsole patterns) | 1 day |
| 2.6 | Project status tracking (provisioning → active → paused → stopped) | 1 day |
| 2.7 | Service URL generation (Studio, API, DB links per project) | 1 day |
| 2.8 | Environment variable management via Studio UI | 2 days |

**Milestone:** Full project lifecycle: create → configure → pause → resume → delete ✅

---

### Phase 3: Resource Management (Weeks 5–7)
> The core differentiator — resource limits, burst pool, and analysis.

| # | Task | Effort |
|---|------|--------|
| 3.1 | Database tables: `resource_plans`, `project_resources` | 1 day |
| 3.2 | Plan/tier system (Free/Basic/Pro/Enterprise/Custom) | 1 day |
| 3.3 | Inject `deploy.resources` into generated docker-compose | 2 days |
| 3.4 | API: `GET/PUT /projects/:ref/resources` | 1 day |
| 3.5 | Dynamic scaling via `docker update` | 2 days |
| 3.6 | Unified burst pool — reservation vs. limit implementation | 2 days |
| 3.7 | Server capacity guard (prevent over-provisioning) | 1 day |
| 3.8 | Priority burst system (rebalancer goroutine) | 2 days |
| 3.9 | Resource analysis collector (30s snapshots) | 2 days |
| 3.10 | Hourly aggregation + data retention cleanup | 1 day |
| 3.11 | Anomaly detection rules | 2 days |
| 3.12 | Optimization recommendation engine | 2 days |
| 3.13 | Analysis API endpoints (8 endpoints) | 2 days |

**Milestone:** Full resource management with limits, burst, monitoring, and recommendations ✅

---

### Phase 4: Security & Auth (Weeks 8–9)
> Harden everything for production.

| # | Task | Effort |
|---|------|--------|
| 4.1 | JWT token expiration + refresh tokens | 2 days |
| 4.2 | Rate limiting middleware (per-IP, per-user, per-endpoint) | 2 days |
| 4.3 | Input validation on all endpoints | 2 days |
| 4.4 | RBAC: admin / owner / member / viewer roles | 2 days |
| 4.5 | Team management + SMTP email invitations (from SupaConsole) | 2 days |
| 4.6 | Audit logging (all actions recorded) | 1 day |
| 4.7 | Secret rotation procedures | 1 day |
| 4.8 | CORS configuration (restrict to known origins) | 0.5 day |
| 4.9 | HTTPS/TLS setup guide | 0.5 day |

**Milestone:** Production-secure with proper auth, RBAC, rate limiting, and audit trail ✅

---

### Phase 5: Testing & Quality (Weeks 10–12)
> No production deployment without tests.

| # | Task | Effort |
|---|------|--------|
| 5.1 | Testing framework setup (Go test + testcontainers) | 2 days |
| 5.2 | Unit tests for provisioner (port allocation, secrets, templates) | 3 days |
| 5.3 | Unit tests for resource manager (limits, burst, capacity) | 2 days |
| 5.4 | Integration tests for API endpoints (all 50+) | 4 days |
| 5.5 | Integration test: full project lifecycle | 2 days |
| 5.6 | E2E test: Studio → create project → verify running | 2 days |
| 5.7 | CI/CD pipeline (GitHub Actions: test → build → deploy) | 2 days |
| 5.8 | Code coverage target: 80%+ | — |

**Milestone:** 80%+ test coverage, CI/CD pipeline, automated quality gates ✅

---

### Phase 6: Production Deployment (Weeks 13–14)
> Ship it.

| # | Task | Effort |
|---|------|--------|
| 6.1 | Production Docker Compose config | 1 day |
| 6.2 | Helm chart updates for K8s | 2 days |
| 6.3 | Monitoring setup (Prometheus + Grafana dashboards) | 2 days |
| 6.4 | Alerting (Slack/Discord/email for critical events) | 1 day |
| 6.5 | Backup & restore procedures | 1 day |
| 6.6 | Documentation: deployment guide, API reference, admin guide | 3 days |
| 6.7 | README + landing page + demo | 2 days |
| 6.8 | Open-source release preparation | 1 day |

**Milestone:** Production-ready, documented, and deployable ✅

---

## Timeline Summary

```
Week  1-2   ████  Phase 1: Foundation (provisioning works)
Week  3-4   ████  Phase 2: Project Lifecycle (full CRUD)
Week  5-7   ██████ Phase 3: Resource Management (limiter + burst + analysis)
Week  8-9   ████  Phase 4: Security & Auth (production-hardened)
Week 10-12  ██████ Phase 5: Testing & Quality (80%+ coverage)
Week 13-14  ████  Phase 6: Production Deployment (ship it)
            ──────────────────────────────────────────────
            Total: 14 weeks (~3.5 months) for full production release
```

### MVP (Minimum Viable Product) — Week 4

After Phase 2, we'd have a working product:
- Create/pause/resume/delete Supabase projects via Studio
- Automatic provisioning with Docker
- Basic auth and project management

This could be released as an **alpha** for early users.

### Beta Release — Week 9

After Phase 4, we'd have a secure, feature-complete product:
- All resource management features
- Security hardened
- Ready for brave production users

---

## Deployment Strategy

### Single Server (Small Scale — 1-10 projects)

```yaml
# docker-compose.prod.yml
services:
  api:
    image: supadash/api:latest
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./projects:/projects
    environment:
      - DATABASE_URL=postgres://...
      - JWT_SECRET=${JWT_SECRET}
  
  studio:
    image: supadash/studio:latest
    restart: unless-stopped
  
  db:
    image: supabase/postgres:15.1.0.147
    volumes:
      - pgdata:/var/lib/postgresql/data
  
  nginx:
    image: nginx:alpine
    ports:
      - "443:443"
    # SSL termination + reverse proxy
```

### Multi-Server (Medium Scale — 10-50 projects)

```
┌─ Management Server ─────────┐
│ API + Studio + Management DB │
└──────────────┬──────────────┘
               │ Docker API (TLS)
    ┌──────────┼──────────┐
    │          │          │
┌───▼───┐ ┌───▼───┐ ┌───▼───┐
│Worker1│ │Worker2│ │Worker3│
│5 proj │ │5 proj │ │5 proj │
└───────┘ └───────┘ └───────┘
```

### Kubernetes (Large Scale — 50+ projects)

Use Helm chart to deploy on any K8s cluster. Each project gets its own namespace.

---

## Cost Estimation (Self-Hosted)

| Scale | Server Spec | Monthly Cost* | Projects |
|-------|-------------|--------------|----------|
| **Solo** | 4 CPU, 8GB RAM, 100GB SSD | $20-40/mo | 1-2 projects |
| **Small** | 8 CPU, 32GB RAM, 500GB SSD | $80-150/mo | 5-8 projects |
| **Medium** | 16 CPU, 64GB RAM, 1TB SSD | $200-400/mo | 10-20 projects |
| **Large** | 32 CPU, 128GB RAM, 2TB SSD | $500-800/mo | 20-40 projects |
| **Cluster** | 3× Medium servers | $600-1200/mo | 50+ projects |

*Based on typical VPS pricing (Hetzner, DigitalOcean, Vultr)

**Compare to Supabase Cloud:** $25/mo per project (Pro plan). At 10 projects = $250/mo on cloud vs. ~$200/mo self-hosted with full control.

---

## Risk Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Studio patches break on new version | High | Medium | Pin to tested version, test patches in CI before upgrading |
| Docker socket access = security risk | Medium | High | Use Docker TLS, restrict API access, run as non-root |
| Resource limits cause OOM kills | Medium | Medium | Soft warnings before hard limits, auto-restart, burst pool |
| Single server = single point of failure | Medium | High | Phase 6: multi-server + K8s support |
| No test coverage | High (today) | High | Phase 5 addresses this entirely |
| Supabase upstream changes break compatibility | Medium | Medium | Pin Docker image versions, test upgrades in staging |

---

## Open Questions

1. ~~**Project name?**~~ → **SupaDash** ✅
2. **Fixed plans vs. custom sliders?** Fixed plans are simpler, custom gives more control. Could do both.
3. **Should limits be hard or soft?** Hard = OOM kill when exceeded. Soft = warning + throttle.
4. **Storage limiting** — Docker's `storage_opt` requires `overlay2` with `xfs` filesystem and `pquota` mount option. Not universally supported. May need to use Docker volumes with size limits instead.
5. **Who can change limits?** Only project owner? Any admin? Need RBAC integration.
6. **Analysis data retention** — How long to keep raw snapshots vs aggregated data? Proposed: 24h raw, 7d at 5min, 30d at 1h, 1y at 1d.
7. **Recommendation engine** — Simple rule-based (if X then suggest Y) or ML-based (learn patterns over time)?
8. **License** — MIT (maximum adoption) or GPL (keep open)? MIT recommended for wider adoption.
9. **Solo or team?** Timeline assumes 1 dev. With 2-3 devs, could ship in 6-8 weeks.
10. **Monetization?** Open-source core + paid enterprise features? Or fully open-source?
