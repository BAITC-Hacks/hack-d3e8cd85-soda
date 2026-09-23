package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

const localDatabaseURL = "postgres://eventmatch:eventmatch_local@127.0.0.1:5432/eventmatch?sslmode=disable"

func openDatabase(ctx context.Context, address string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(address)
	if err != nil {
		return nil, errors.New("Некорректная настройка DATABASE_URL.")
	}
	config.MaxConns = 5
	config.ConnConfig.ConnectTimeout = 5 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err == nil {
		err = pool.Ping(ctx)
	}
	if err != nil {
		if pool != nil {
			pool.Close()
		}
		// Driver errors can include connection details; never echo credentials.
		return nil, errors.New("Не удалось подключиться к PostgreSQL. Проверьте DATABASE_URL и запустите docker compose up -d db.")
	}
	return pool, nil
}

func initDatabase(ctx context.Context, pool *pgxpool.Pool, csvPath, evidencePath string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	// Serialize schema creation and the first import across app processes.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(793501)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, schemaSQL); err != nil {
		return err
	}
	var imported bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM eventmatch.catalog_imports WHERE source='organizer')").Scan(&imported); err != nil {
		return err
	}
	if imported {
		return tx.Commit(ctx)
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM eventmatch.contractors").Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return errors.New("БД уже содержит профили без отметки импорта; автоматическое заполнение остановлено, существующие данные сохранены.")
	}
	a, err := loadCatalog(csvPath, evidencePath)
	if err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for _, p := range a.Profiles {
		batch.Queue(`INSERT INTO eventmatch.contractors
			(id, anon_name, categories, city, price_from_kzt, event_formats, languages, max_hours,
			description, evidence, synthetic, city_imputed, price_imputed, data_origin)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
			p.ID, p.Name, p.Categories, p.City, p.Price, p.Formats, p.Languages, p.Hours, p.Description, p.Evidence, p.Synthetic, p.CityImputed, p.PriceImputed, p.Origin)
		batch.Queue(`INSERT INTO eventmatch.busy_dates(contractor_id,busy_date)
			SELECT $1, day::date FROM unnest($2::text[]) AS day`, p.ID, p.Busy)
	}
	if err = tx.SendBatch(ctx, batch).Close(); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO eventmatch.catalog_imports(source,source_version,profile_count) VALUES ('organizer',$1,$2)`, a.Version, len(a.Profiles)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (a *App) snapshot(ctx context.Context) (*App, error) {
	if a.DB == nil {
		return a, nil
	} // Pure in-memory inputs are used by unit tests only.
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	// One statement reads the entire catalog consistently.
	// For a large catalog, move the shared filtering rules into indexed SQL queries.
	rows, err := a.DB.Query(ctx, `SELECT c.id,c.anon_name,c.categories,c.city,c.price_from_kzt,
		c.event_formats,c.languages,c.max_hours,c.description,c.evidence,c.synthetic,
		c.city_imputed,c.price_imputed,c.data_origin,
		ARRAY(SELECT to_char(b.busy_date,'YYYY-MM-DD') FROM eventmatch.busy_dates b
			WHERE b.contractor_id=c.id ORDER BY b.busy_date)
		FROM eventmatch.contractors c ORDER BY c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	snapshot := &App{AI: a.AI, DB: a.DB, Search: a.Search}
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.Name, &p.Categories, &p.City, &p.Price, &p.Formats, &p.Languages,
			&p.Hours, &p.Description, &p.Evidence, &p.Synthetic, &p.CityImputed, &p.PriceImputed, &p.Origin, &p.Busy); err != nil {
			return nil, err
		}
		snapshot.Profiles = append(snapshot.Profiles, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(snapshot.Profiles) == 0 {
		return nil, errors.New("Каталог в PostgreSQL пуст.")
	}
	if err := snapshot.finalizeCatalog(); err != nil {
		return nil, fmt.Errorf("некорректный каталог в БД: %w", err)
	}
	if a.Index != nil && a.Index.DataVersion == snapshot.Version {
		snapshot.Index = a.Index
	}
	return snapshot, nil
}

func (a *App) requestCatalog(w http.ResponseWriter, r *http.Request) *App {
	current, err := a.snapshot(r.Context())
	if err != nil {
		apiError(w, 503, "DATABASE_UNAVAILABLE", "Каталог временно недоступен. Условия сохранены — попробуйте ещё раз.")
		return nil
	}
	return current
}
