-- Bootstrap for docker-compose Postgres. Schema creation itself is handled
-- by the services' idempotent migration on startup; this only ensures the
-- role and database exist.
-- (Kept separate so a native/local Postgres can be prepared the same way.)
DO $$
BEGIN
   IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'agenttrace') THEN
      CREATE ROLE agenttrace LOGIN PASSWORD 'agenttrace';
   END IF;
END
$$;
