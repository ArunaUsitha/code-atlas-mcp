package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

type RAMCompiler struct {
	memDB *sql.DB
}

func NewRAMCompiler() (*RAMCompiler, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}

	pragmas := []string{
		"PRAGMA journal_mode = OFF;",
		"PRAGMA synchronous = OFF;",
		"PRAGMA locking_mode = EXCLUSIVE;",
		"PRAGMA cache_size = -2000000;", // ~2GB memory cache
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
	}

	if err := RunMigrations(db); err != nil {
		db.Close()
		return nil, err
	}

	return &RAMCompiler{memDB: db}, nil
}

func (rc *RAMCompiler) DB() *sql.DB {
	return rc.memDB
}

func (rc *RAMCompiler) FlushToDisk(diskPath string) error {
	if _, err := os.Stat(diskPath); err == nil {
		if err := os.Remove(diskPath); err != nil {
			return fmt.Errorf("failed to remove existing target db: %w", err)
		}
	}
	_, err := rc.memDB.Exec(fmt.Sprintf("VACUUM INTO '%s';", diskPath))
	return err
}

func (rc *RAMCompiler) Close() {
	if rc.memDB != nil {
		rc.memDB.Close()
	}
}
