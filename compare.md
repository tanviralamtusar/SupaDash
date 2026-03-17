# SupaConsole vs supa-manager — Detailed Comparison

> Both projects aim to **manage self-hosted Supabase instances**, but they differ radically in architecture, maturity, and scope.

---

## At a Glance

| Dimension | **SupaConsole** | **supa-manager** |
|---|---|---|
| **Architecture** | Full-stack monolith (Next.js) | Microservices (Go API + patched Studio) |
| **Backend Language** | TypeScript (Next.js API routes) | Go (Gin framework) |
| **Frontend** | Custom React UI (shadcn/ui) | Patched official Supabase Studio v1.24.04 |
| **Database** | SQLite via Prisma ORM | PostgreSQL via sqlc |
| **Auth** | Custom JWT + NextAuth | Custom JWT + Argon2 password hashing |
| **API Endpoints** | ~10 Next.js API routes | **51 Go handlers** |
| **Provisioning** | ✅ Working (clone repo → docker compose) | ⚠️ Interface built, Docker SDK integrated, but **not fully functional** |
| **Docker Integration** | Shells out to `docker compose` CLI | Docker SDK (Go native) + Docker Compose |
| **Deployment** | `npm run build && npm start` | `docker compose up -d` (all-in-one) |
| **Helm / K8s** | ❌ None | ✅ Helm chart included |
| **Documentation** | README only (~260 lines) | **25+ doc files** (roadmap, security, architecture, wiki) |
| **License** | MIT | GPL v3 |
| **Tests** | None | None (planned for Phase 4) |
| **Maturity** | MVP — works end-to-end | Beta — API-complete, provisioning WIP |

---

## 🏗️ Architecture

### SupaConsole — Monolith
```
Next.js 15 App (TypeScript)
├── React Frontend (shadcn/ui, Tailwind CSS)
├── API Routes (/api/*)
├── Prisma ORM → SQLite
└── Shells out to Docker Compose CLI
```
- **Single process** — everything runs in one Next.js server.
- Frontend and backend are tightly coupled.
- Uses Prisma for database access with a simple SQLite database.
- Calls `docker compose` via `child_process.exec()`.

### supa-manager — Microservices
```
Docker Compose Orchestration
├── Go API Server (Gin, :8080)     ← 51 endpoints
├── Patched Supabase Studio (:3000) ← official UI, patched
├── PostgreSQL (:5432)              ← management DB
├── (planned) Version Service
└── (planned) DNS Service
```
- **Three separate services** communicate via internal Docker network.
- The Studio UI is the actual Supabase Studio, patched to talk to the custom Go backend.
- Uses sqlc for type-safe SQL queries with PostgreSQL.
- Docker SDK integration for programmatic container management.

---

## ✅ SupaConsole — Pros

| # | Pro | Details |
|---|-----|---------|
| 1 | **Working end-to-end provisioning** | Creates projects, generates unique ports, deploys Docker stacks, pauses, and deletes — all functional today. |
| 2 | **Dead-simple setup** | `npm install → npm run dev` — no Docker required for the dashboard itself. |
| 3 | **Familiar tech stack** | TypeScript + Next.js + React — easy for most web developers. |
| 4 | **Custom UI with good UX** | Purpose-built dashboard with shadcn/ui components, dark theme, env var editor, service URL shortcuts. |
| 5 | **Lightweight** | SQLite = zero external DB dependency. Entire app is one process. |
| 6 | **Cross-platform** | Handles Windows (`xcopy`) and Linux (`cp -r`) natively. |
| 7 | **Team management** | Built-in user registration, team member invitations, password reset via SMTP. |
| 8 | **Smart port allocation** | Auto-generates unique port ranges per project to avoid conflicts. |
| 9 | **Robust error handling** | Detailed, user-friendly error messages for Docker failures (network, permissions, buffer overflows). |
| 10 | **MIT license** | Maximum freedom for commercial use and modification. |

## ❌ SupaConsole — Cons

| # | Con | Details |
|---|-----|---------|
| 1 | **No real Supabase Studio** | Custom UI — not the official Studio. Missing DB editor, SQL editor, RLS policies, etc. |
| 2 | **SQLite limitations** | No concurrent writes, no replication, not production-grade for multi-user scenarios. |
| 3 | **Shell-based Docker** | Runs `docker compose` via `exec()` — fragile, no streaming output, buffer size limits. |
| 4 | **Mock JWT generation** | [generateJWT()](file:///d:/Coding/supadash/SupaConsole/src/lib/project.ts#19-31) uses random strings for signatures — not cryptographically secure. |
| 5 | **No API documentation** | No OpenAPI spec, no endpoint docs. |
| 6 | **Monolith scaling** | Cannot scale frontend and backend independently. |
| 7 | **No tests** | Zero unit/integration tests. |
| 8 | **No Kubernetes support** | Docker Compose only — no path to K8s deployment. |
| 9 | **No security docs** | No security guidelines, secret rotation procedures, or hardening docs. |
| 10 | **Limited API surface** | ~10 API routes vs. the 51 endpoints Studio expects — won't work with official Studio. |

---

## ✅ supa-manager — Pros

| # | Pro | Details |
|---|-----|---------|
| 1 | **Official Studio UI** | Uses patched Supabase Studio — familiar UX, DB editor, SQL editor, all Studio features. |
| 2 | **Go backend = performance** | Compiled Go binary, Gin framework — significantly faster than Node.js for API workloads. |
| 3 | **Production-grade DB** | PostgreSQL with proper migrations, typed queries (sqlc), and connection pooling (pgxpool). |
| 4 | **51 API endpoints** | Comprehensive API covering projects, orgs, billing stubs, analytics, pg-meta, notifications, integrations. |
| 5 | **Docker SDK integration** | Programmatic Docker management via Go SDK — more reliable than CLI shelling. |
| 6 | **Kubernetes-ready** | Helm chart included for K8s deployment. |
| 7 | **Extensive documentation** | SECURITY.md, PROJECT_ROADMAP.md (8-phase plan), ARCHITECTURE.md, LESSONS_LEARNED.md, wiki with 25+ guides. |
| 8 | **Provisioner interface** | Clean abstraction (`provisioner.Provisioner`) — easy to swap Docker for K8s or cloud providers. |
| 9 | **Strong security** | Argon2 password hashing, proper JWT validation via `golang-jwt`, secret rotation docs, SECURITY.md with checklist. |
| 10 | **Professional project structure** | Clear separation: `api/`, `database/`, `conf/`, `provisioner/`, `migrations/`, `queries/`. |
| 11 | **Backup & quota systems** | Built-in backup and quota management modules ([backup.go](file:///d:/Coding/supadash/supa-manager/supa-manager/provisioner/backup.go), [quotas.go](file:///d:/Coding/supadash/supa-manager/supa-manager/provisioner/quotas.go)). |
| 12 | **Self-contained deployment** | Single `docker compose up -d` brings up everything (DB + API + Studio). |

## ❌ supa-manager — Cons

| # | Con | Details |
|---|-----|---------|
| 1 | **Provisioning not fully working** | Projects created in DB but actual Supabase infrastructure **not spun up yet** (their own README warns about this). |
| 2 | **Complex setup** | Must build patched Studio image first (5–10 min Docker build), then run Compose. |
| 3 | **GPL v3 license** | Copyleft — all derivative works must also be GPL. Not suitable for proprietary/commercial forks. |
| 4 | **Stub endpoints** | Many endpoints return mock/empty data (analytics, pg-meta, billing). |
| 5 | **No tests** | Zero test files despite 60 Go source files — "critical gap" acknowledged in roadmap. |
| 6 | **Docker-as-dependency** | The dashboard itself runs in Docker — heavier footprint than a simple Node app. |
| 7 | **Linux-focused** | Build scripts ([build.sh](file:///d:/Coding/supadash/supa-manager/studio/build.sh), [patch.sh](file:///d:/Coding/supadash/supa-manager/studio/patch.sh)) are bash scripts — harder to develop on Windows. |
| 8 | **Alpha-stage maturity** | Despite extensive planning docs, the actual functionality is incomplete. |
| 9 | **CORS wide open** | `AllowOrigins: ["*"]` — fine for dev, security risk in production. |
| 10 | **Some hardcoded secrets remain** | README default `JWT_SECRET: secret` in docker-compose, though documented for dev only. |

---

## 🔬 Deep Comparison

### Database Design

| Aspect | SupaConsole | supa-manager |
|--------|-------------|--------------|
| **Engine** | SQLite | PostgreSQL 15 |
| **ORM/Query** | Prisma (auto-generated client) | sqlc (type-safe raw SQL) |
| **Models** | User, Session, Project, ProjectEnvVar, TeamMember, PasswordResetToken | Account, Organization, OrgMember, Project (with infra columns) |
| **Migrations** | `prisma db push` (schema-driven) | Manual SQL migration files |
| **Multi-tenancy** | User-owned projects | Organization → Project hierarchy |
| **Scalability** | ❌ Single-file DB | ✅ Fully replicate-able PostgreSQL |

### Provisioning & Docker

| Aspect | SupaConsole | supa-manager |
|--------|-------------|--------------|
| **Method** | `exec("docker compose up -d")` | Docker SDK (Go native client) |
| **Template** | Clones entire Supabase repo, copies `docker/` | Template-based docker-compose generation |
| **Port Management** | Timestamp-based unique ports | Config-based base port + project ID offset |
| **Lifecycle** | ✅ Create, Deploy, Pause, Delete — all working | ⚠️ Create only (pause/resume/delete commented out) |
| **Pre-flight checks** | ✅ Docker, Compose, internet connectivity | Basic healthcheck only |
| **Error handling** | ✅ Detailed, user-friendly messages | Basic error returns |

### Security

| Aspect | SupaConsole | supa-manager |
|--------|-------------|--------------|
| **Password hashing** | bcryptjs | Argon2 (stronger) |
| **JWT handling** | Mock signatures (⚠️ insecure) | golang-jwt with proper validation |
| **Secret management** | `.env` only | `.env` + SECURITY.md + Docker secrets guide |
| **Security docs** | None | Full SECURITY.md (300+ lines) with rotation guide, checklist |
| **Rate limiting** | None | None (planned Phase 5) |

### Documentation & Planning

| Aspect | SupaConsole | supa-manager |
|--------|-------------|--------------|
| **README** | 257 lines, good | 519 lines, excellent |
| **Architecture docs** | None | SUPABASE_ARCHITECTURE.md (12K) |
| **Roadmap** | None | PROJECT_ROADMAP.md (48K, 8 phases) |
| **Security guide** | None | SECURITY.md (7.7K) |
| **Lessons learned** | None | LESSONS_LEARNED.md (8.7K) |
| **Total doc files** | 1 | **17 documentation files** |

---

## 🏆 Verdict

### Who should use **SupaConsole**?

> **Best for:** Individuals or small teams who need a **working** self-hosted Supabase manager **today**.

- ✅ If you want something that actually **deploys Supabase projects right now**
- ✅ If you're a JavaScript/TypeScript developer
- ✅ If you need a lightweight, easy-to-set-up solution
- ✅ If you want MIT-licensed code for commercial use
- ✅ If you don't need the official Supabase Studio UI

### Who should use **supa-manager**?

> **Best for:** Teams building a **production-grade**, scalable Supabase management platform who are willing to invest time finishing the implementation.

- ✅ If you want the **official Supabase Studio** experience
- ✅ If you need a Go-based, high-performance backend
- ✅ If you plan to deploy to **Kubernetes** at scale
- ✅ If you value comprehensive documentation and a clear roadmap
- ✅ If GPL v3 licensing works for your use case
- ⚠️ Be prepared: **provisioning is not fully functional yet**

### Overall Winner

| Criteria | Winner |
|----------|--------|
| **Works today (functionality)** | 🏆 **SupaConsole** |
| **Architecture & scalability** | 🏆 **supa-manager** |
| **UI/UX** | 🏆 **supa-manager** (official Studio) |
| **Ease of setup** | 🏆 **SupaConsole** |
| **Security** | 🏆 **supa-manager** |
| **Documentation** | 🏆 **supa-manager** |
| **Production readiness** | ⚖️ **Neither** (both lack tests) |
| **Future potential** | 🏆 **supa-manager** |
| **Licensing flexibility** | 🏆 **SupaConsole** (MIT) |

> **Bottom line:** **SupaConsole wins for immediate usability** — it works end-to-end today. **supa-manager wins for long-term potential** — better architecture, security, docs, and the official Studio UI. If provisioning were complete, supa-manager would be the clear winner overall.
