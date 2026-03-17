-- Migration: Resource management tables
-- project_resources: per-project resource plan and limits
-- resource_snapshots: raw 30-second container stats
-- resource_snapshots_hourly: aggregated hourly stats

CREATE TABLE IF NOT EXISTS public.project_resources (
    id              serial          not null primary key,
    project_ref     text            not null unique,
    plan            varchar(32)     not null default 'FREE',
    -- CPU limits
    cpu_limit       decimal(4,2)    not null default 0.5,       -- CPU cores
    cpu_reservation decimal(4,2)    not null default 0.25,      -- Reserved CPU
    -- Memory limits (bytes)
    memory_limit    bigint          not null default 536870912,  -- 512 MB
    memory_reservation bigint       not null default 268435456, -- 256 MB (reservation for burst)
    -- Burst pool
    burst_eligible  boolean         not null default true,
    burst_priority  int             not null default 0,          -- Higher = more priority
    -- Tracking
    created_at      timestamptz     not null default now(),
    updated_at      timestamptz     not null default now()
);

CREATE TABLE IF NOT EXISTS public.resource_snapshots (
    id                  bigserial       primary key,
    project_ref         text            not null,
    service_name        varchar(64)     not null,
    -- Memory
    memory_usage_bytes  bigint,
    memory_limit_bytes  bigint,
    -- CPU
    cpu_usage_percent   decimal(5,2),
    cpu_limit_cores     decimal(4,2),
    -- Disk I/O
    disk_read_bytes     bigint,
    disk_write_bytes    bigint,
    -- Network I/O
    network_rx_bytes    bigint,
    network_tx_bytes    bigint,
    -- Container state
    container_status    varchar(32),
    restart_count       int             default 0,
    oom_killed          boolean         default false,
    -- Timestamp
    recorded_at         timestamptz     not null default now()
);

CREATE INDEX IF NOT EXISTS idx_snapshots_project_time
    ON public.resource_snapshots(project_ref, recorded_at DESC);

CREATE TABLE IF NOT EXISTS public.resource_snapshots_hourly (
    id                      bigserial   primary key,
    project_ref             text        not null,
    service_name            varchar(64) not null,
    hour                    timestamptz not null,
    -- Aggregated values
    avg_memory_usage_bytes  bigint,
    max_memory_usage_bytes  bigint,
    avg_cpu_percent         decimal(5,2),
    max_cpu_percent         decimal(5,2),
    total_disk_read_bytes   bigint,
    total_disk_write_bytes  bigint,
    total_network_rx_bytes  bigint,
    total_network_tx_bytes  bigint,
    -- Burst pool
    burst_pool_usage_bytes  bigint      default 0,
    burst_pool_duration_sec int         default 0,
    -- Anomalies
    oom_kill_count          int         default 0,
    restart_count           int         default 0,
    UNIQUE(project_ref, service_name, hour)
);

CREATE TABLE IF NOT EXISTS public.resource_recommendations (
    id                  serial      primary key,
    project_ref         text        not null,
    type                varchar(32) not null,
    severity            varchar(16) not null,
    title               text        not null,
    description         text        not null,
    potential_savings_mb int         default 0,
    is_dismissed        boolean     default false,
    created_at          timestamptz not null default now(),
    dismissed_at        timestamptz
);

CREATE INDEX IF NOT EXISTS idx_recommendations_project
    ON public.resource_recommendations(project_ref, is_dismissed);
