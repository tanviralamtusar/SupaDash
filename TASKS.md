# SupaDash — Remaining Tasks

All skipped or partially implemented features, ordered from easiest to hardest.

---

## 🟢 Easy (1–2 hours each)

- [x] **Grafana dashboard JSON** — Create pre-built dashboard for the existing `/v1/metrics` data (goroutines, memory, GC)
- [ ] **Hourly aggregation + data retention** — Aggregate analysis snapshots hourly, prune old data with a cron goroutine
- [ ] **Per-user rate limiting** — Extend `api/middleware.go` to rate-limit by user ID (from JWT), not just IP
- [ ] **Per-endpoint rate limiting** — Allow different rate limits on sensitive endpoints (e.g., `/auth/token`)
- [ ] **Admin guide documentation** — Write `docs/admin-guide.md` covering ops tasks: backup, monitoring, scaling, troubleshooting
- [ ] **SMTP email sending** — Wire actual SMTP transport into `api/team.go` invite flow (endpoint exists, mail sending needs testing)
- [ ] **Add `supadash_active_projects` gauge** — Query DB for project count and expose in `/v1/metrics`
- [ ] **Add request duration histogram** — Gin middleware to track `supadash_api_request_duration_seconds`
- [ ] **`.gitignore` cleanup** — Remove any remaining temp files, ensure all generated files are ignored

---

## 🟡 Medium (3–8 hours each)

- [ ] **Integration tests (auth)** — `api/auth_test.go` — test token refresh, logout (needs test DB setup)
- [ ] **Integration tests (team)** — `api/team_test.go` — test invite, update role, remove member
- [ ] **Integration tests (audit)** — `api/audit_test.go` — test audit log creation and retrieval
- [ ] **Integration tests (secrets)** — `api/secrets_api_test.go` — test secret rotation
- [ ] **Full lifecycle integration test** — Create → configure → pause → resume → delete in one test
- [ ] **testcontainers setup** — Use testcontainers-go for real PostgreSQL in CI
- [ ] **Dynamic scaling via `docker update`** — Call Docker API to live-update CPU/memory limits without restart
- [ ] **Alerting (Slack webhook)** — POST to Slack/Discord webhook on critical events (project down, disk full)
- [ ] **Alerting (email)** — Send alert emails via SMTP on critical events
- [ ] **Helm chart for Kubernetes** — K8s deployment manifests for multi-node scaling
- [ ] **DNS hook integration** — Actually create/delete DNS records when projects are created/removed
- [ ] **Real ConfigCat integration** — Connect to real feature flag service instead of stub
- [ ] **Custom hostname assignment** — Implement real hostname assignment for projects via reverse proxy config

---

## 🔴 Hard (1–3 days each)

- [ ] **SupaConsole frontend integration** — Connect the forked React frontend to SupaDash API as the dashboard UI
- [ ] **E2E test suite** — Browser-based test: Studio UI → create project → verify it's running → delete
- [ ] **Landing page** — Marketing site at root domain with features, pricing, demo
- [ ] **Real billing (Stripe)** — Replace stub billing endpoints with actual Stripe integration
- [ ] **Real integrations (GitHub/Vercel)** — OAuth flow + repo connection for deployment triggers
- [ ] **Multi-server mode** — Manage remote Docker hosts via TLS, distribute projects across workers
- [ ] **CLI tool** — `supadash` command-line tool for managing projects from terminal
- [ ] **Plugin system** — Extensible hooks for custom provisioning logic
- [ ] **Multi-region deployment** — Deploy projects to different geographic regions
- [ ] **Project backup/restore** — Backup and restore individual project databases (not just management DB)
- [ ] **Usage-based billing** — Metered billing based on actual CPU/memory/storage consumption
- [ ] **Project templates** — Pre-configured project types (SaaS starter, mobile backend, etc.)
