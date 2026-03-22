# SupaDash — Must-Have Services Reference

Every Supabase project provisioned by SupaDash runs these **15 services** as Docker containers.

---

## Core Database

| # | Service | Image | Port | Purpose |
|---|---------|-------|------|---------|
| 1 | **PostgreSQL** | `supabase/postgres:15.8.1.048` | 5432 | The main database. Stores all user data, auth users, storage metadata, and analytics. Everything in Supabase revolves around Postgres. |

## API Layer

| # | Service | Image | Port | Purpose |
|---|---------|-------|------|---------|
| 2 | **Kong** | `kong:2.8.1` | 8000 | API Gateway. Routes all incoming requests to the correct service (Auth, REST, Realtime, Storage, Functions). Handles authentication via JWT, rate limiting, and CORS. The single entry point for all client requests. |
| 3 | **PostgREST** | `postgrest/postgrest:v12.2.12` | 3000 | Generates a full REST API from the Postgres schema automatically. When users do `supabase.from('table').select()`, this is what handles it. |
| 4 | **Postgres Meta** | `supabase/postgres-meta:v0.89.3` | 8080 | Provides database metadata (tables, columns, types, extensions) for Studio's Table Editor. Powers the visual database management UI. |

## Authentication

| # | Service | Image | Port | Purpose |
|---|---------|-------|------|---------|
| 5 | **GoTrue (Auth)** | `supabase/gotrue:v2.174.0` | 9999 | Full authentication system: email/password, magic links, OAuth (Google, GitHub, etc.), phone/SMS, anonymous users. Manages JWTs, sessions, and user profiles. |

## Realtime

| # | Service | Image | Port | Purpose |
|---|---------|-------|------|---------|
| 6 | **Realtime** | `supabase/realtime:v2.34.47` | 4000 | WebSocket server for real-time subscriptions. When users do `supabase.channel('room').on('postgres_changes', ...)`, this listens to Postgres WAL changes and pushes them to connected clients. |

## Storage

| # | Service | Image | Port | Purpose |
|---|---------|-------|------|---------|
| 7 | **Storage API** | `supabase/storage-api:v1.14.6` | 5000 | File upload/download API. Manages buckets, access policies, and presigned URLs. When users upload images or files, this service handles it. |
| 8 | **MinIO** | `ghcr.io/coollabsio/minio` | 9000 | S3-compatible object storage backend. Storage API stores files here instead of on disk. Enables bucket management, presigned URLs, and S3 API compatibility. |
| 9 | **Imgproxy** | `darthsim/imgproxy:v3.8.0` | 8080 | On-the-fly image transformation. Resizes, crops, and converts images when requested (e.g., `?width=200&height=200`). Used by Storage API for image optimization. |

## Serverless Functions

| # | Service | Image | Port | Purpose |
|---|---------|-------|------|---------|
| 10 | **Edge Functions** | `supabase/edge-runtime:v1.67.4` | — | Deno-based serverless runtime. Runs user-written TypeScript functions on the server. Like AWS Lambda but built into Supabase. Essential for custom backend logic, webhooks, and cron jobs. |

## Connection Management

| # | Service | Image | Port | Purpose |
|---|---------|-------|------|---------|
| 11 | **Supavisor** | `supabase/supavisor:2.5.1` | 4000 | Connection pooler (replaces PgBouncer). Manages a pool of database connections so the DB doesn't get overwhelmed by concurrent users. Critical for production with many connections. |

## Observability

| # | Service | Image | Port | Purpose |
|---|---------|-------|------|---------|
| 12 | **Analytics (Logflare)** | `supabase/logflare:1.4.0` | 4000 | Log aggregation and analytics engine. Collects logs from all services and stores them in Postgres for querying from Studio. |
| 13 | **Vector** | `timberio/vector:0.28.1-alpine` | — | Log collector agent. Reads Docker container logs and forwards them to Logflare for processing. |

## Dashboard

| # | Service | Image | Port | Purpose |
|---|---------|-------|------|---------|
| 14 | **Studio** | `supabase/studio:2026.03.04` | 3000 | The web dashboard UI. Visual interface for managing tables, running SQL, viewing auth users, browsing storage, reading logs, and managing Edge Functions. The "control panel" of each project. |

---

## Service Dependency Chain

```
                    PostgreSQL (db)
                    ┌─────┴─────┐
                    │           │
              Analytics     All Services
                    │
              ┌─────┴─────┐
              │           │
           Vector      Studio
                          │
                      Kong (gateway)
                    ┌───┬───┬───┬───┐
                    │   │   │   │   │
                  REST Auth RT  Stor Edge
                              │
                            MinIO
```

## Per-Project Resource Usage

| Scale | CPU | RAM | Disk |
|-------|-----|-----|------|
| Minimal (idle) | ~0.5 cores | ~1.5 GB | ~500 MB |
| Light (few users) | ~1 core | ~2.5 GB | ~1 GB |
| Medium (100+ users) | ~2 cores | ~4 GB | ~5 GB |
| Heavy (1000+ users) | ~4 cores | ~8 GB | ~20 GB+ |

> **Note:** Each provisioned project runs all 15 services. A server with 16 GB RAM can comfortably run ~3-4 projects.
