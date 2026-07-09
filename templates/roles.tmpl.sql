-- Sync internal Supabase role passwords to the project's DB password.
--
-- The supabase/postgres image CREATES these roles (via its bundled migrations)
-- but does not assign them POSTGRES_PASSWORD. Without this, PostgREST/GoTrue/
-- Storage/Realtime connect as authenticator/supabase_auth_admin/etc. and fail
-- with "password authentication failed". The official self-hosted compose
-- mounts an equivalent roles.sql; SupaDash renders this one per project.
--
-- Mounted at /docker-entrypoint-initdb.d/init-scripts/99-roles.sql so it runs
-- AFTER the image's role-creating migrations, and only on first DB init.
--
-- The password is a generated alphanumeric string (see IsInfraSafePassword), so
-- it is safe to embed here; the DO block guards each role with IF EXISTS so a
-- role missing in a given image version doesn't abort initialization.
DO $$
DECLARE
  target_role text;
  roles text[] := ARRAY[
    'authenticator',
    'pgbouncer',
    'supabase_admin',
    'supabase_auth_admin',
    'supabase_storage_admin',
    'supabase_functions_admin',
    'supabase_read_only_user',
    'supabase_replication_admin',
    'dashboard_user'
  ];
BEGIN
  FOREACH target_role IN ARRAY roles LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = target_role) THEN
      EXECUTE format('ALTER ROLE %I WITH PASSWORD %L', target_role, '{{.DBPassword}}');
    END IF;
  END LOOP;
END
$$;
