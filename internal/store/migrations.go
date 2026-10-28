package store

import (
	"context"
	"database/sql"
	"fmt"
)

// migration is one ordered schema step. Every statement of a migration, plus
// the schema_migrations row recording its version, commits in a single
// transaction, so a migration is either fully applied or not recorded at all.
type migration struct {
	version int64
	name    string
	stmts   []string
}

// migrations is the production migration list. Append only; never reorder,
// renumber, or edit an already released migration.
var migrations = []migration{
	{
		version: 1,
		name:    "create review tasks and delivery deduplication",
		stmts: []string{
			`CREATE TABLE review_tasks (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				source_identity TEXT NOT NULL UNIQUE,
				provider TEXT NOT NULL,
				owner TEXT NOT NULL,
				repo TEXT NOT NULL,
				external_project_id TEXT NOT NULL DEFAULT '',
				review_kind TEXT NOT NULL,
				number INTEGER NOT NULL,
				comment_id TEXT NOT NULL,
				requester_login TEXT NOT NULL,
				agent TEXT NOT NULL,
				focus TEXT NOT NULL,
				delivery_id TEXT NOT NULL DEFAULT '',
				state TEXT NOT NULL DEFAULT 'queued'
					CHECK (state IN ('queued', 'running', 'succeeded', 'failed')),
				received_at TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE task_deliveries (
				provider TEXT NOT NULL,
				owner TEXT NOT NULL,
				repo TEXT NOT NULL,
				delivery_id TEXT NOT NULL,
				source_identity TEXT NOT NULL
					REFERENCES review_tasks (source_identity),
				first_seen_at TEXT NOT NULL,
				PRIMARY KEY (provider, owner, repo, delivery_id)
			)`,
			`CREATE INDEX idx_review_tasks_state ON review_tasks (state)`,
		},
	},
}

// migrate applies pending migrations from list in order. It rejects a list
// that is not gapless starting at 1, and a database whose applied versions
// are not a prefix-compatible subset of list, so a downgrade or a forked
// history fails loudly instead of silently diverging.
func migrate(ctx context.Context, db *sql.DB, list []migration) error {
	for i, m := range list {
		if m.version != int64(i+1) {
			return fmt.Errorf("store: migration list: position %d has version %d, want %d", i, m.version, i+1)
		}
	}

	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("store: create schema_migrations: %w", err)
	}

	known := make(map[int64]migration, len(list))
	for _, m := range list {
		known[m.version] = m
	}

	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return fmt.Errorf("store: read schema_migrations: %w", err)
	}
	defer rows.Close()
	var applied []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return fmt.Errorf("store: scan schema_migrations: %w", err)
		}
		applied = append(applied, v)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: read schema_migrations: %w", err)
	}
	for _, v := range applied {
		if _, ok := known[v]; !ok {
			return fmt.Errorf("store: database has applied migration %d which is not in the migration list", v)
		}
	}

	for _, m := range list {
		if containsVersion(applied, m.version) {
			continue
		}
		if err := applyMigration(ctx, db, m); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: migration %d: begin: %w", m.version, err)
	}
	defer tx.Rollback()

	for _, stmt := range m.stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("store: migration %d (%s): %w", m.version, m.name, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.version, m.name, nowUTC().Format(timeFormat),
	); err != nil {
		return fmt.Errorf("store: migration %d (%s): record: %w", m.version, m.name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: migration %d (%s): commit: %w", m.version, m.name, err)
	}
	return nil
}

func containsVersion(versions []int64, v int64) bool {
	for _, x := range versions {
		if x == v {
			return true
		}
	}
	return false
}
