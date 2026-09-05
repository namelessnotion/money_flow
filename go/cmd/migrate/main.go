// DATABASE_URL=postgres://... go run ./cmd/migrate up
// DATABASE_URL=postgres://... go run ./cmd/migrate down
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

func main() {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("migrate: DATABASE_URL is not set")
	}

	dir := os.Getenv("MIGRATIONS_DIR")
	if dir == "" {
		dir = "db/migrations"
	}

	direction := "up"
	if len(os.Args) > 1 {
		direction = os.Args[1]
	}

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("migrate: connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		log.Fatalf("migrate: create schema_migrations: %v", err)
	}

	versions, err := migrationVersions(dir)
	if err != nil {
		log.Fatalf("migrate: list migrations: %v", err)
	}

	switch direction {
	case "up":
		runUp(ctx, conn, dir, versions)
	case "down":
		runDown(ctx, conn, dir, versions)
	default:
		log.Fatalf("migrate: unknown direction %q: want \"up\" or \"down\"", direction)
	}
}

func migrationVersions(dir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return nil, err
	}
	versions := make([]string, len(matches))
	for i, m := range matches {
		versions[i] = strings.TrimSuffix(filepath.Base(m), ".up.sql")
	}
	sort.Strings(versions)
	return versions, nil
}

func appliedVersions(ctx context.Context, conn *pgx.Conn) (map[string]bool, error) {
	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	done := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		done[v] = true
	}
	return done, rows.Err()
}

func runUp(ctx context.Context, conn *pgx.Conn, dir string, versions []string) {
	done, err := appliedVersions(ctx, conn)
	if err != nil {
		log.Fatalf("migrate: load applied migrations: %v", err)
	}

	for _, v := range versions {
		if done[v] {
			continue
		}
		applyStep(ctx, conn, dir, v, "up", `INSERT INTO schema_migrations (version) VALUES ($1)`)
		fmt.Printf("applied %s\n", v)
	}
}

func runDown(ctx context.Context, conn *pgx.Conn, dir string, versions []string) {
	done, err := appliedVersions(ctx, conn)
	if err != nil {
		log.Fatalf("migrate: load applied migrations: %v", err)
	}

	var last string
	for _, v := range versions {
		if done[v] {
			last = v
		}
	}
	if last == "" {
		fmt.Println("nothing to roll back")
		return
	}

	applyStep(ctx, conn, dir, last, "down", `DELETE FROM schema_migrations WHERE version = $1`)
	fmt.Printf("rolled back %s\n", last)
}

func applyStep(ctx context.Context, conn *pgx.Conn, dir, version, direction, bookkeepingSQL string) {
	sqlBytes, err := os.ReadFile(filepath.Join(dir, version+"."+direction+".sql"))
	if err != nil {
		log.Fatalf("migrate: read %s.%s.sql: %v", version, direction, err)
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		log.Fatalf("migrate: begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
		log.Fatalf("migrate: run %s.%s.sql: %v", version, direction, err)
	}
	if _, err := tx.Exec(ctx, bookkeepingSQL, version); err != nil {
		log.Fatalf("migrate: record %s: %v", version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("migrate: commit %s: %v", version, err)
	}
}
