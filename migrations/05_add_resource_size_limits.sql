-- Migration: Per-project disk size limits and usage tracking
-- Adds database/storage size limits (with platform minimums: 500 MB DB, 1 GB storage)
-- and current-usage columns updated by the analysis collector.

ALTER TABLE public.project_resources
    ADD COLUMN IF NOT EXISTS database_size_limit_bytes bigint      not null default 524288000,  -- 500 MB (minimum)
    ADD COLUMN IF NOT EXISTS storage_size_limit_bytes  bigint      not null default 1073741824, -- 1 GB (minimum)
    ADD COLUMN IF NOT EXISTS database_size_bytes       bigint      not null default 0,          -- current usage
    ADD COLUMN IF NOT EXISTS storage_size_bytes        bigint      not null default 0,          -- current usage
    ADD COLUMN IF NOT EXISTS writes_blocked            boolean     not null default false,      -- soft-block flag at 100% usage
    ADD COLUMN IF NOT EXISTS usage_updated_at          timestamptz;
