-- Migration to add project_env_vars table
CREATE TABLE IF NOT EXISTS public.project_env_vars (
    id         serial      not null primary key,
    project_ref text       not null,
    key        text        not null,
    value      text        not null,
    is_secret  boolean     not null default false,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    UNIQUE(project_ref, key)
);

CREATE INDEX IF NOT EXISTS idx_project_env_vars_ref ON public.project_env_vars(project_ref);
