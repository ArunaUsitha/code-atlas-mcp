package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func InitDB(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	if err := RunMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

func RunMigrations(db *sql.DB) error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			project TEXT NOT NULL,
			name TEXT NOT NULL,
			qualified_name TEXT NOT NULL,
			kind TEXT NOT NULL,
			file_path TEXT NOT NULL,
			start_line INTEGER NOT NULL,
			end_line INTEGER NOT NULL,
			signature TEXT,
			content_hash TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_kind ON nodes(kind);`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_project ON nodes(project);`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_file ON nodes(file_path);`,
		`CREATE TABLE IF NOT EXISTS edges (
			source_id TEXT NOT NULL,
			target_id TEXT NOT NULL,
			type TEXT NOT NULL,
			project TEXT NOT NULL,
			PRIMARY KEY (source_id, target_id, type),
			FOREIGN KEY (source_id) REFERENCES nodes(id) ON DELETE CASCADE,
			FOREIGN KEY (target_id) REFERENCES nodes(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_id);`,
		`CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_id);`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS nodes_fts USING fts5(
			node_id UNINDEXED,
			name,
			qualified_name,
			signature,
			content,
			tokenize="unicode61"
		);`,
		`CREATE TABLE IF NOT EXISTS node_vectors (
			node_id TEXT PRIMARY KEY,
			vector BLOB NOT NULL,
			FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS adrs (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			status TEXT NOT NULL,
			date TEXT NOT NULL,
			decisions TEXT NOT NULL,
			context TEXT NOT NULL
		);`,
	}

	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migration statement failed: %w\nStatement: %s", err, stmt)
		}
	}

	return nil
}
