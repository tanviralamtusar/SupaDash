# SupaDash Dashboard — Task Checklist

## Phase 1: Studio Fork & Patch System ✅
- [x] Create `studio/` directory structure
- [x] Create [studio/build.sh](file:///d:/Coding/supadash/SupaDash/studio/build.sh) — Download + patch + build
- [x] Create [studio/patch.sh](file:///d:/Coding/supadash/SupaDash/studio/patch.sh) — Apply patch files
- [x] Create [studio/Dockerfile](file:///d:/Coding/supadash/SupaDash/studio/Dockerfile) — Build custom Studio image
- [x] Create patch: [01-api-urls.patch](file:///d:/Coding/supadash/SupaDash/studio/patches/01-api-urls.patch) — Redirect API calls
- [x] Create patch: [02-auth-integration.patch](file:///d:/Coding/supadash/SupaDash/studio/patches/02-auth-integration.patch) — Use SupaDash auth
- [x] Create patch: [03-remove-cloud-features.patch](file:///d:/Coding/supadash/SupaDash/studio/patches/03-remove-cloud-features.patch) — Strip cloud UI  
- [x] Create patch: [04-branding.patch](file:///d:/Coding/supadash/SupaDash/studio/patches/04-branding.patch) — SupaDash logos + title
- [x] Add `supadash-studio` to [docker-compose.yaml](file:///d:/Coding/supadash/SupaDash/docker-compose.yaml)
- [x] Copy branding assets to `files/public/img/`

## Phase 2: Auth Integration + 2FA
- [x] Patch Studio login to call SupaDash `/auth/token` (in Phase 1 patches)
- [x] Add TOTP 2FA endpoints to Go API
- [x] Add database migration for 2FA secrets
- [x] Add 2FA setup page in Studio
- [x] Test: login → 2FA → dashboard

## Phase 3: Core Dashboard Pages
- [x] Patch: Project List page
- [x] Patch: Create Project page (with plan selector)
- [x] Patch: Project Dashboard
- [x] Patch: Table Editor / SQL Editor
- [x] Patch: Auth Users, Storage, Edge Functions, Logs

## Phase 4: Resource Manager (NEW page)
- [ ] Create Resource Manager page
- [ ] Overview cards (RAM/CPU/Disk gauges)
- [ ] Per-service breakdown table
- [ ] Scaling controls (plan selector + sliders)
- [ ] Usage charts (historical)
- [ ] Recommendations panel
- [ ] Server Overview page (admin-only)

## Phase 5: Team Management
- [ ] Patch team page for SupaDash API

## Phase 6: Real-time Updates
- [ ] Add WebSocket endpoints to Go API
- [ ] Patch Studio for live status updates

## Phase 7: Branding & Polish
- [ ] Replace logos and favicon
- [ ] Remove cloud-only UI elements
- [ ] Final QA pass
