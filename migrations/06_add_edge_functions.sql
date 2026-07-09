-- Migration: Edge Functions metadata and per-project secrets
-- edge_functions: metadata for functions deployed to a project's edge-runtime
--                 (source files live under <projects_dir>/<ref>/functions/<slug>/)
-- project_secrets: encrypted env vars injected into the edge-runtime

CREATE TABLE IF NOT EXISTS public.edge_functions (
    id              serial      primary key,
    project_ref     text        not null,
    slug            text        not null,
    name            text        not null,
    status          text        not null default 'ACTIVE',
    verify_jwt      boolean     not null default true,
    entrypoint_path text        not null default 'index.ts',
    version         int         not null default 1,
    created_at      timestamptz not null default now(),
    updated_at      timestamptz not null default now(),
    UNIQUE (project_ref, slug)
);

CREATE INDEX IF NOT EXISTS idx_edge_functions_project
    ON public.edge_functions (project_ref);

CREATE TABLE IF NOT EXISTS public.project_secrets (
    id              serial      primary key,
    project_ref     text        not null,
    name            text        not null,
    value_encrypted text        not null,
    created_at      timestamptz not null default now(),
    updated_at      timestamptz not null default now(),
    UNIQUE (project_ref, name)
);

CREATE INDEX IF NOT EXISTS idx_project_secrets_project
    ON public.project_secrets (project_ref);
