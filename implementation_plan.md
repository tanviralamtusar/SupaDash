# SupaDash Dashboard — Implementation Plan

> Fork of official Supabase Studio, patched to connect to SupaDash Go API, with new features for project management, resource monitoring, and team management.

---

## Strategy: Fork + Patch Official Studio

Instead of building a dashboard from scratch, we fork the **official `supabase/studio`** and patch it to:
1. Point all API calls at SupaDash Go backend instead of Supabase Cloud
2. Add new pages for SupaDash-specific features (resource manager, server overview)
3. Replace branding (logo, colors, strings)
4. Remove cloud-only features (billing, Stripe, cloud regions)

This is how **supa-manager** already works — it has a `studio/` directory with `build.sh`, `patch.sh`, and `patches/` that modify Studio source and build a custom Docker image.

> [!IMPORTANT]
> This is a **large, multi-week project**. This plan covers the full scope but we'll execute in phases.

---

## User Requirements Summary

| Requirement | Decision |
|-------------|----------|
| Base | Fork official Studio (like supa-manager) |
| Users | Single admin + team members with roles |
| Auth | Email/password + 2-step verification |
| Theme | Official Supabase dark theme (matching) |
| Branding | SupaDash logo (blue dual-triangle) |
| Responsive | Desktop only |
| Deployment | Bundled in docker-compose (single stack) |
| Real-time | Live updates via WebSocket |
| Priority pages | Login, Project List, Create Project, Resource Manager |

---

## Architecture

```
┌───────────────────────────────────────────────────────────┐
│                    Browser (Desktop)                       │
│                                                           │
│  ┌─────────────────────────────────────────────────────┐  │
│  │          SupaDash Studio (Patched Next.js)           │  │
│  │                                                     │  │
│  │  Login → Projects → Project Detail → Resources      │  │
│  │          ↑ Table Editor, SQL Editor, Auth, Logs      │  │
│  └──────────┬──────────────────────────────────────────┘  │
│             │ HTTP + WebSocket                             │
└─────────────┼─────────────────────────────────────────────┘
              │
    ┌─────────▼──────────┐
    │  SupaDash Go API    │ ← Existing (:8080)
    │  63 endpoints       │
    └─────────┬──────────┘
              │
    ┌─────────▼──────────┐     ┌──────────────────┐
    │  Management DB     │     │  Docker Engine     │
    │  (PostgreSQL)      │     │  (project stacks)  │
    └────────────────────┘     └──────────────────┘
```

---

## Phase 1: Studio Fork & Patch System (Week 1)

Set up the build pipeline to fork, patch, and build a custom Studio Docker image.

### Tasks

#### [NEW] `studio/build.sh`
- Script to download official Studio source at a specific tag
- Apply patches from `studio/patches/`
- Copy replacement files from `studio/files/`
- Build Docker image: `supadash/studio:latest`

#### [NEW] `studio/patch.sh`
- Apply `.patch` files to Studio source
- Core patches needed:
  - **API URL**: Change `https://api.supabase.io` → `http://api:8080` (SupaDash backend)
  - **Auth endpoint**: `/auth/token` → SupaDash Go auth
  - **Remove cloud features**: Stripe billing, cloud regions, support tickets
  - **Branding**: Replace Supabase logo with SupaDash logo

#### [NEW] `studio/patches/` directory
- `01-api-urls.patch` — Redirect all API calls
- `02-auth-integration.patch` — Use SupaDash JWT auth
- `03-remove-cloud-features.patch` — Strip cloud-only UI
- `04-branding.patch` — Logo, title, favicon
- `05-resource-manager.patch` — Add new Resource Manager page

#### [MODIFY] [docker-compose.yaml](file:///d:/Coding/supadash/SupaDash/docker-compose.yaml) (Coolify)
- Add `supadash-studio` service pointing to our custom image
- Expose on port 3000
- Connect to `supadash-api` via internal network

---

## Phase 2: Auth Integration (Week 1–2)

Make Studio login work with SupaDash's Go backend.

### Tasks

#### Patch: Auth Flow
Studio normally calls Supabase GoTrue for auth. We patch it to call SupaDash's:
- `POST /auth/token` — Login (email + password → JWT)
- `POST /auth/logout` — Logout
- `GET /profile` — Get user profile
- `GET /profile/permissions` — Get RBAC permissions

#### [NEW] 2-Step Verification (Go API)
- Add `POST /v1/auth/2fa/setup` — Generate TOTP secret + QR code
- Add `POST /v1/auth/2fa/verify` — Verify TOTP code
- Add `POST /v1/auth/2fa/disable` — Disable 2FA
- Store TOTP secrets encrypted in management DB
- Update `POST /auth/token` to require 2FA code when enabled

#### [MODIFY] [api/auth.go](file:///d:/Coding/supadash/SupaDash/api/auth.go)
- Add 2FA middleware check after password verification
- Generate TOTP using `github.com/pquerna/otp/totp`

---

## Phase 3: Core Dashboard Pages (Week 2–3)

These pages exist in official Studio but need patching to work with SupaDash API.

### Pages (Patch existing Studio pages)

| Page | Studio Route | SupaDash API |
|------|-------------|--------------|
| **Login** | `/sign-in` | `POST /auth/token` |
| **Project List** | `/projects` | `GET /platform/projects` |
| **Create Project** | `/new/project` | `POST /platform/projects` |
| **Project Dashboard** | `/project/[ref]` | `GET /platform/projects/:ref` |
| **Table Editor** | `/project/[ref]/editor` | `POST /platform/pg-meta/:ref/query` |
| **SQL Editor** | `/project/[ref]/sql` | `POST /platform/pg-meta/:ref/query` |
| **Auth Users** | `/project/[ref]/auth/users` | Proxied to project's GoTrue |
| **Storage** | `/project/[ref]/storage` | Proxied to project's Storage API |
| **Edge Functions** | `/project/[ref]/functions` | Proxied to project's Edge Runtime |
| **Logs** | `/project/[ref]/logs` | Proxied to project's Analytics |
| **Settings** | `/project/[ref]/settings` | `GET /platform/projects/:ref/settings` |

### Key patch: Project Creation Flow
Official Studio assumes cloud infrastructure. We patch the "New Project" page to:
1. Show plan selector (Free/Basic/Pro/Enterprise) with resource limits
2. Call `POST /platform/projects` with selected plan
3. Show real-time provisioning progress (WebSocket)
4. Display connection strings when ready

---

## Phase 4: Resource Manager (Week 3–4) — NEW PAGE

This is the **core SupaDash differentiator** — doesn't exist in official Studio.

### [NEW] Resource Manager Page (`/project/[ref]/resources`)

**Sections:**

1. **Overview Cards**
   - Total RAM usage vs limit (gauge)
   - Total CPU usage vs limit (gauge)
   - Total Disk usage vs limit (gauge)
   - Plan badge (Free/Basic/Pro/Enterprise)

2. **Per-Service Table**
   - Service name, RAM, CPU, Disk, Status (healthy/warning/critical)
   - Color-coded bars showing usage vs limits
   - Matches the mockup from [thinking.md](file:///d:/Coding/supadash/thinking.md) lines 524–545

3. **Resource Scaling**
   - Plan selector or custom sliders
   - "Scale Up" / "Scale Down" buttons
   - Confirmation dialog with impact warning
   - Calls `PUT /projects/:ref/resources`

4. **Usage Charts** (historical)
   - RAM over time (stacked area by service)
   - CPU over time (line chart)
   - Disk growth trend with projected full date
   - Burst pool usage timeline
   - Calls `GET /projects/:ref/analysis/history`

5. **Recommendations Panel**
   - Auto-generated optimization suggestions
   - Dismiss/acknowledge actions
   - Calls `GET /projects/:ref/analysis/recommendations`

6. **Anomaly Alerts**
   - Memory leak warnings
   - CPU spike alerts
   - Disk filling warnings
   - Idle project suggestions

### [NEW] Server Overview Page (`/server/resources`)

Admin-only page showing server-wide resource utilization:
- Total server CPU/RAM/Disk gauges
- Per-project resource allocation bars
- Unified burst pool status
- Capacity remaining for new projects
- Calls `GET /server/resources` and `GET /server/resources/capacity`

---

## Phase 5: Team Management (Week 4)

### Patch: Team Page (`/org/[slug]/team`)
- List team members with roles
- Invite new members (email field)
- Change roles (owner/admin/member/viewer)
- Remove members
- Uses existing SupaDash API:
  - `GET /organizations/:slug/team`
  - `POST /organizations/:slug/team/invite`
  - `PUT /organizations/:slug/team/:id`
  - `DELETE /organizations/:slug/team/:id`

---

## Phase 6: Real-time Updates (Week 4–5)

### [MODIFY] Go API — Add WebSocket endpoints
- `WS /v1/projects/:ref/status/stream` — Live project status
- `WS /v1/projects/:ref/resources/stream` — Live resource usage (Docker stats)
- `WS /v1/server/resources/stream` — Live server overview

### Patch: Studio WebSocket integration
- Project list page: live status badges (creating → running → paused)
- Resource manager: live updating charts and gauges
- Use `gorilla/websocket` on backend

---

## Phase 7: Branding & Polish (Week 5)

### Logo replacement
- Sidebar logo → [supadash-full-white.png](file:///d:/Coding/supadash/SupaDash/supadash-full-white.png)
- Login page logo → `supadash-full-black.png`
- Favicon → [supadash-icon.png](file:///d:/Coding/supadash/SupaDash/supadash-icon.png)
- Browser tab title → "SupaDash"

### Remove cloud-only UI
- Remove Stripe billing pages
- Remove "Upgrade to Pro" CTAs
- Remove cloud region selector (we use local Docker)
- Remove support ticket system
- Remove "Supabase" text references → "SupaDash"

---

## Docker Compose Integration

```yaml
# Added to existing docker-compose.yaml
supadash-studio:
  image: supadash/studio:latest
  build:
    context: ./studio
    dockerfile: Dockerfile
  restart: unless-stopped
  ports:
    - "3000:3000"
  environment:
    SUPADASH_API_URL: http://supadash-api:8080
    STUDIO_PG_META_URL: http://supadash-api:8080/platform/pg-meta
    NEXT_PUBLIC_ENABLE_LOGS: "true"
  depends_on:
    supadash-api:
      condition: service_healthy
```

---

## Execution Order (Priority)

| Phase | What | Effort | Dependency |
|-------|------|--------|------------|
| 1 | Fork + Patch system | 3 days | None |
| 2 | Auth integration + 2FA | 3 days | Phase 1 |
| 3 | Core pages working | 5 days | Phase 2 |
| 4 | Resource Manager (NEW) | 5 days | Phase 3 |
| 5 | Team management | 2 days | Phase 3 |
| 6 | WebSocket real-time | 3 days | Phase 4 |
| 7 | Branding + polish | 2 days | Phase 3 |
| **Total** | | **~23 days** | |

---

## Verification Plan

### Manual Verification
1. Build custom Studio image locally: `./studio/build.sh`
2. Start full stack via `docker-compose up`
3. Login at `http://localhost:3000` with SupaDash credentials
4. Create a new project → verify Docker containers spin up
5. Open project → verify Table Editor, SQL Editor, Auth pages work
6. Open Resource Manager → verify live gauges and charts
7. Invite team member → verify email sent and role assigned
8. Verify real-time status updates on project list page

### Automated Tests
- Studio E2E with Playwright: login → create project → verify resource page
- API integration tests for new 2FA and WebSocket endpoints
- Docker build test in CI: verify `build.sh` produces a working image
