package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func disposableDatabase(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	address := os.Getenv("TEST_DATABASE_URL")
	if address == "" {
		t.Skip("set TEST_DATABASE_URL to run against PostgreSQL; requires CREATE DATABASE")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := openDatabase(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	// Own disposable database: never clear the application's schema or catalog.
	name := fmt.Sprintf("eventmatch_check_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+identifier); err != nil {
			t.Error(err)
		}
	})
	config, err := pgxpool.ParseConfig(address)
	if err != nil {
		t.Fatal("invalid TEST_DATABASE_URL")
	}
	config.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal("cannot open disposable database")
	}
	t.Cleanup(pool.Close)
	databaseURL := address + " dbname=" + name
	if strings.HasPrefix(address, "postgres://") || strings.HasPrefix(address, "postgresql://") {
		u, err := url.Parse(address)
		if err != nil {
			t.Fatal("invalid TEST_DATABASE_URL")
		}
		u.Path = "/" + name
		query := u.Query()
		query.Del("dbname")
		u.RawQuery = query.Encode()
		databaseURL = u.String()
	}
	return pool, databaseURL
}

func TestPostgreSQL(t *testing.T) {
	pool, _ := disposableDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var err error
	if err = initDatabase(ctx, pool, "missing.csv", "missing.json"); err == nil {
		t.Fatal("bad import succeeded")
	}
	var schemaExists bool
	if err = pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='eventmatch')").Scan(&schemaExists); err != nil || schemaExists {
		t.Fatal("failed import did not roll back schema", err)
	}
	// Concurrent application starts must import exactly once.
	completed := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { completed <- initDatabase(ctx, pool, "data/catalog.csv", "data/evidence.json") }()
	}
	for i := 0; i < 2; i++ {
		if err := <-completed; err != nil {
			t.Fatal(err)
		}
	}
	if err = initDatabase(ctx, pool, "missing.csv", "missing.json"); err != nil {
		t.Fatal("restart reread seed files", err)
	}
	a := &App{DB: pool}
	snapshot, err := a.snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seed := catalog(t)
	if !reflect.DeepEqual(seed.Profiles, snapshot.Profiles) || seed.Version != snapshot.Version {
		t.Fatal("database round trip changed data or provenance")
	}
	q := hostQuery()
	before := snapshot.match(q)
	if !reflect.DeepEqual(before, seed.match(q)) {
		t.Fatal("database changed recommendations")
	}
	// A committed calendar update changes the next request without a server restart.
	a.Index = &SemanticIndex{DataVersion: seed.Version}
	if _, err = pool.Exec(ctx, "INSERT INTO eventmatch.busy_dates(contractor_id,busy_date) VALUES ($1,$2::date)", before.Cards[0].ID, q.Date); err != nil {
		t.Fatal(err)
	}
	snapshot, err = a.snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Index != nil || snapshot.Version == seed.Version {
		t.Fatal("stale semantic index reused")
	}
	after := snapshot.match(q)
	if after.Total != before.Total-1 || strings.Contains(strings.Join(ids(after), ","), before.Cards[0].ID) {
		t.Fatal("busy profile still recommended")
	}
	if err = initDatabase(ctx, pool, "data/catalog.csv", "data/evidence.json"); err != nil {
		t.Fatal(err)
	}
	snapshot, err = a.snapshot(ctx)
	if err != nil || !reflect.DeepEqual(after, snapshot.match(q)) {
		t.Fatal("restart overwrote edited calendar", err)
	}
	// Both public search endpoints must read committed edits, not the startup catalog.
	t.Run("fresh search snapshot", func(t *testing.T) {
		python := pythonExecutable()
		if _, err := exec.LookPath(python); err != nil {
			t.Skipf("Python unavailable: %v", err)
		}
		app := seed
		app.DB, app.Search = pool, pythonSearch(python)
		body, _ := json.Marshal(q)
		for _, path := range []string{"/api/matches", "/api/search"} {
			r := httptest.NewRecorder()
			request := httptest.NewRequest("POST", path, strings.NewReader(string(body)))
			request.Header.Set("Content-Type", "application/json")
			app.handler().ServeHTTP(r, request)
			var result Result
			if err := json.Unmarshal(r.Body.Bytes(), &result); err != nil || r.Code != 200 || result.Total != before.Total-1 || strings.Contains(strings.Join(ids(result), ","), before.Cards[0].ID) {
				t.Fatalf("%s returned stale catalog: HTTP %d %s", path, r.Code, r.Body)
			}
			if len(result.DateAlternatives) == 0 || result.DateAlternatives[0].ID != before.Cards[0].ID {
				t.Fatalf("%s lost date alternatives", path)
			}
		}
	})
	for _, statement := range []string{
		"UPDATE eventmatch.contractors SET evidence='invented fact' WHERE id='HK-88430'",
		"UPDATE eventmatch.contractors SET price_from_kzt=-1 WHERE id='HK-88430'",
		"UPDATE eventmatch.contractors SET max_hours='NaN' WHERE id='HK-88430'",
		"INSERT INTO eventmatch.busy_dates VALUES ('HK-88430','2027-01-01')",
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatal("database constraint missing", statement)
		}
	}
	pool.Close()
	r := httptest.NewRecorder()
	a.handler().ServeHTTP(r, httptest.NewRequest("GET", "/api/options", nil))
	if r.Code != 503 || !strings.Contains(r.Body.String(), "DATABASE_UNAVAILABLE") {
		t.Fatal("DB outage not reported", r.Code)
	}
}
