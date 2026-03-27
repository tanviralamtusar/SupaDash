# SupaDash — Project Status

> Self-hosted Supabase management platform.  
> Repo: [github.com/tanviralamtusar/SupaDash](https://github.com/tanviralamtusar/SupaDash)  
> Live: [supadash.botbhai.net](https://supadash.botbhai.net)

---

## What Was Built

### Phase 1: Foundation ✅
| Feature | Status | Details |
|---------|--------|---------|
| Go API server (Gin) | ✅ Done | `main.go`, `api/api.go` |
| PostgreSQL management DB | ✅ Done | sqlc-generated queries in `database/` |
| Docker provisioner | ✅ Done | `provisioner/docker.go` — talks to Docker Engine |
| Port allocator | ✅ Done | `provisioner/ports.go` — dynamic port assignment |
| Secret generator | ✅ Done | `provisioner/secrets.go` — JWT, API keys, passwords |
| Project slug generator | ✅ Done | `utils/slug.go` |
| Docker Compose templating | ✅ Done | `templates/project-compose.tmpl.yml` |

### Phase 2: Project Lifecycle ✅
| Feature | Status | Details |
|---------|--------|---------|
| Create project | ✅ Done | `POST /platform/projects` |
| List projects | ✅ Done | `GET /platform/projects` |
| Get project details | ✅ Done | `GET /platform/projects/:ref` |
| Delete project | ✅ Done | `DELETE /projects/:ref` |
| Pause/Resume project | ✅ Done | `POST /projects/:ref/pause`, `/resume` |
| Project status & health | ✅ Done | `GET /projects/:ref/status`, `/health` |
| Env var management | ✅ Done | `GET/PUT /projects/:ref/env` |
| Database metadata (pg-meta) | ✅ Done | Tables, columns, types, queries, publications |
| Organization management | ✅ Done | `POST /platform/organizations` |
| Billing stubs | ✅ Done | Subscription, usage, invoices, addons |
| Integration stubs | ✅ Done | Connections, authorization, repositories |
| ConfigCat stub | ✅ Done | Feature flag configuration endpoint |

### Phase 3: Resource Management ✅
| Feature | Status | Details |
|---------|--------|---------|
| Resource limits per project | ✅ Done | `GET/PUT /projects/:ref/resources` |
| Server capacity tracking | ✅ Done | `GET /server/resources`, `/capacity` |
| Burst pool manager | ✅ Done | `provisioner/burst.go` |
| Resource analysis collector | ✅ Done | 30-second snapshot goroutine |
| Analysis history | ✅ Done | `GET /projects/:ref/analysis/history` |
| Anomaly detection | ✅ Done | Rule-based detection |
| Optimization recommendations | ✅ Done | `GET /projects/:ref/analysis/recommendations` |
| Recommendation dismissal | ✅ Done | `POST /projects/:ref/analysis/recommendations/:id/dismiss` |
| Analytics endpoints | ✅ Done | Usage counts, request counts |

### Phase 4: Security & Auth ✅
| Feature | Status | Details |
|---------|--------|---------|
| JWT auth (access + refresh tokens) | ✅ Done | `api/auth.go`, 1h access / 7d refresh |
| Rate limiting (IP-based) | ✅ Done | `api/middleware.go`, sliding window |
| RBAC middleware | ✅ Done | `RequireOrgRole`, `RequireProjectRole` |
| Team management | ✅ Done | Invite, update role, remove members |
| Audit logging | ✅ Done | `GET /projects/:ref/audit` |
| Secret rotation | ✅ Done | `POST /projects/:ref/secrets/rotate` |
| CORS configuration | ✅ Done | Configurable via `ALLOWED_ORIGINS` |
| Input validation | ✅ Done | On all mutative endpoints |

### Phase 5: Testing & Quality ✅
| Feature | Status | Details |
|---------|--------|---------|
| Provisioner unit tests | ✅ Done | 17 tests (`secrets_test.go`, `ports_test.go`) |
| Utils unit tests | ✅ Done | 5 tests, 76.9% coverage (`slug_test.go`) |
| Middleware tests | ✅ Done | 6 tests (`middleware_test.go`) |
| CI pipeline (GitHub Actions) | ✅ Done | `.github/workflows/test.yml` |
| Build pipeline (GitHub Actions) | ✅ Done | `.github/workflows/build.yml` |

### Phase 6: Production Deployment ✅
| Feature | Status | Details |
|---------|--------|---------|
| Production Dockerfile | ✅ Done | Multi-stage, health check, Go 1.25 |
| Docker Compose (Coolify) | ✅ Done | `docker-compose.yaml` — one-click deploy |
| Docker Compose (standalone) | ✅ Done | `docker-compose.prod.yml` + Caddyfile |
| Prometheus metrics | ✅ Done | `/v1/metrics` — goroutines, memory, GC |
| Backup/Restore scripts | ✅ Done | `scripts/backup.sh`, `scripts/restore.sh` |
| README | ✅ Done | Architecture, quick start, config table |
| API Reference | ✅ Done | `docs/api-reference.md` (~55 endpoints) |
| Deployment Guide | ✅ Done | `docs/deployment-guide.md` |
| LICENSE (MIT) | ✅ Done | `LICENSE` |
| SECURITY.md | ✅ Done | `SECURITY.md` |
| CONTRIBUTING.md | ✅ Done | `CONTRIBUTING.md` |
| Deployed to Coolify | ✅ Done | `supadash.botbhai.net` |
| **Phase 7: Studio Platform Integration** | ✅ Done | Forked Supabase Studio, patched API urls, stripped cloud UI |
| **Phase 8: Real-time Infrastructure** | ✅ Done | Gorilla WebSocket Hub, project status broadcasting, `useRealtime` hook |
| **Phase 9: Branding & Polish** | ✅ Done | Final audit: Logos, metadata, error pages, log templates |
| **Phase 10: Production Persistence** | ✅ Done | Migrated all custom Studio branding to the `studio/files` overlay system for stable self-hosting. |

---

## What Was Skipped / Not Added

### ❌ Not Implemented

| Item | Planned In | Why Skipped | Difficulty |
|------|-----------|-------------|------------|
| **Helm chart for Kubernetes** | Phase 6.2 | Not needed for current single-server deployment | Medium |
| **Grafana dashboards** | Phase 6.3 | Prometheus endpoint exists but no pre-built dashboard JSON | Easy |
| **Alerting (Slack/Discord/email)** | Phase 6.4 | No webhook integration built | Medium |
| **E2E tests** | Phase 5.6 | No Studio → create project → verify flow test | Hard |
| **Integration tests (DB-dependent)** | Phase 5.4 | Auth, team, audit, secrets API tests need a test DB | Medium |
| **Full lifecycle integration test** | Phase 5.5 | Create → configure → pause → resume → delete test | Medium |
| **testcontainers setup** | Phase 5.1 | Tests use in-memory mocks, not real containers | Medium |
| **Per-user / per-endpoint rate limiting** | Phase 4.2 | Only IP-based rate limiting implemented | Easy |
| **Dynamic scaling via `docker update`** | Phase 3.5 | Resource limits are set at creation, not dynamically updated | Medium |
| **Hourly aggregation + data retention** | Phase 3.10 | Analysis snapshots collected but not aggregated/pruned | Easy |
| **Admin guide documentation** | Phase 6.6 | Only deployment guide + API reference, no admin ops guide | Easy |
| **SMTP email invitations (actual sending)** | Phase 4.5 | Team invite endpoint exists but SMTP sending may not be wired | Easy |

### ⚠️ Partially Implemented

| Item | What Exists | What's Missing |
|------|------------|----------------|
| **Test coverage (target: 80%)** | 76.9% utils, 10% provisioner, 1.9% api | Need more API and provisioner tests |
| **Monitoring** | `/v1/metrics` returns Go runtime stats | No `supadash_active_projects` gauge or request duration histogram |
| **Billing** | Stub endpoints return mock data | No real Stripe integration |
| **Integrations** | Stub endpoints exist | No real GitHub/Vercel integration |
| **DNS hook** | Config variables exist | No actual DNS record creation |
| **ConfigCat** | Endpoint exists | Returns static config, no real feature flags |
| **Custom hostname** | Endpoint exists | Returns stub, no real hostname assignment |

### 🔮 Future Enhancements (Not in Original Roadmap)

| Item | Description |
|------|-------------|
| **Multi-server mode** | Manage worker nodes via Docker TLS API |
| **Project templates** | Pre-configured project types (SaaS, mobile, etc.) |
| **Usage-based billing** | Metered billing based on actual resource consumption |
| **Project backups** | Backup/restore individual project databases |
| **Log viewer** | Stream project container logs through the API |
| **CLI tool** | `supadash` CLI for managing projects from terminal |
| **Plugin system** | Extensible hooks for custom provisioning logic |
| **Multi-region** | Deploy projects to different geographic regions |
| **Landing Page** | High-conversion marketing landing page |

---

## API Endpoint Count

| Category | Count |
|----------|-------|
| Health & Monitoring | 4 |
| Auth | 3 |
| Profile | 3 |
| Organizations & Teams | 7 |
| Projects (CRUD + lifecycle) | 12 |
| Resources & Analysis | 8 |
| Secrets & Audit | 2 |
| Database (pg-meta) | 8 |
| Analytics | 4 |
| Billing | 4 |
| Integrations | 3 |
| Server | 2 |
| Misc (ConfigCat, Props) | 3 |
| WebSocket (Real-time) | 5 |
| **Total** | **~68** |

---

| Layer | Technology |
|-------|-----------|
| Frontend | React 18, Next.js 14, Tailwind v4 (Studio Fork) |
| Core | Go 1.25 (Gin Framework) |
| Language | Go 1.25 |
| Web framework | Gin |
| Database | PostgreSQL 15 |
| ORM / Queries | sqlc |
| Auth | JWT (HS256) with refresh tokens |
| Container mgmt | Docker Engine API |
| Reverse proxy | Caddy (auto-HTTPS) |
| CI/CD | GitHub Actions |
| Deployment | Coolify (Docker Compose) |
| Monitoring | Prometheus-compatible `/metrics` |
| Testing | Go test + testify |

---

## File Structure

```
SupaDash/
├── api/              # HTTP handlers, middleware, metrics
├── conf/             # Config loading (env vars)
├── database/         # sqlc-generated DB queries
├── docs/             # Deployment guide, API reference, TLS guide
├── migrations/       # SQL migration files
├── monitoring/       # Prometheus config
├── permissions/      # RBAC permission definitions
├── provisioner/      # Docker provisioner, ports, secrets, resources, burst
├── queries/          # sqlc SQL query definitions
├── scripts/          # Backup/restore shell scripts
├── templates/        # Docker Compose template for projects
├── utils/            # Slug generator
├── .github/workflows # CI/CD pipelines
├── docker-compose.yaml       # Coolify one-click deploy
├── docker-compose.prod.yml   # Standalone production deploy
├── Dockerfile
├── Caddyfile
└── main.go
```
