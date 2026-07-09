-- Migration: Project branches (DB-only branching)
-- A branch is a separate database inside the parent project's Postgres
-- container, cloned from the main database. Managed by the DBBrancher.

CREATE TABLE IF NOT EXISTS public.project_branches (
    id                 serial      primary key,
    parent_project_ref text        not null,
    branch_name        text        not null,
    db_name            text        not null,
    status             text        not null default 'CREATING', -- CREATING | RUNNING | MERGING | RESETTING | REBASING | FAILED | DELETING
    git_branch         text,
    created_at         timestamptz not null default now(),
    updated_at         timestamptz not null default now(),
    UNIQUE (parent_project_ref, branch_name)
);

CREATE INDEX IF NOT EXISTS idx_project_branches_parent
    ON public.project_branches (parent_project_ref);
