CREATE SCHEMA IF NOT EXISTS eventmatch;

CREATE TABLE IF NOT EXISTS eventmatch.schema_migrations (
    version integer PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS eventmatch.contractors (
    id text PRIMARY KEY CHECK (id <> ''),
    anon_name text NOT NULL CHECK (anon_name <> ''),
    categories text[] NOT NULL CHECK (cardinality(categories) > 0),
    city text NOT NULL CHECK (city <> ''),
    price_from_kzt bigint NOT NULL CHECK (price_from_kzt >= 0),
    event_formats text[] NOT NULL CHECK (cardinality(event_formats) > 0),
    languages text[] NOT NULL CHECK (cardinality(languages) > 0),
    max_hours double precision CHECK (max_hours > 0 AND max_hours < 'Infinity'::float8),
    description text NOT NULL,
    evidence text NOT NULL CHECK (evidence <> '' AND strpos(description, evidence) > 0),
    synthetic boolean NOT NULL,
    city_imputed boolean NOT NULL,
    price_imputed boolean NOT NULL,
    data_origin text NOT NULL CHECK (data_origin IN ('organizer', 'team'))
);

CREATE TABLE IF NOT EXISTS eventmatch.busy_dates (
    contractor_id text NOT NULL REFERENCES eventmatch.contractors(id) ON DELETE CASCADE,
    busy_date date NOT NULL CHECK (busy_date BETWEEN DATE '2026-09-23' AND DATE '2026-12-31'),
    PRIMARY KEY (contractor_id, busy_date)
);

CREATE TABLE IF NOT EXISTS eventmatch.catalog_imports (
    source text PRIMARY KEY,
    source_version text NOT NULL,
    profile_count integer NOT NULL CHECK (profile_count > 0),
    imported_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO eventmatch.schema_migrations(version) VALUES (1) ON CONFLICT DO NOTHING;
