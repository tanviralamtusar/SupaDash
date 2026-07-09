-- Migration: Personal access tokens (PATs)
-- Used to authenticate MCP clients and other API automation.
-- Only a SHA-256 hash of the token is stored.

CREATE TABLE IF NOT EXISTS public.personal_access_tokens (
    id           serial      primary key,
    account_id   int         not null,
    name         text        not null,
    token_hash   text        not null unique,
    created_at   timestamptz not null default now(),
    last_used_at timestamptz
);

ALTER TABLE public.personal_access_tokens
    ADD CONSTRAINT fk_pat_account FOREIGN KEY (account_id) REFERENCES accounts (id);

CREATE INDEX IF NOT EXISTS idx_pat_account
    ON public.personal_access_tokens (account_id);
