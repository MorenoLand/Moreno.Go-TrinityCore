package database

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	MigrationStateReleased = "RELEASED"
	MigrationStateArchived = "ARCHIVED"
)

type Migration struct {
	Version    int
	Name       string
	Statements []string
	Hash       string
	State      string
}

type MigrationOptions struct {
	RedundancyChecks   bool
	AllowRehash        bool
	ArchivedRedundancy bool
	CleanOrphans       bool
	CleanOrphansMax    int
}

type MigrationResult struct {
	Applied  int
	Rehashed int
	Archived int
	Orphans  int
}

func LoadMigrations(dir string) ([]Migration, error) {
	result := make([]Migration, 0)
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			return nil
		}
		base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		parts := strings.SplitN(base, "_", 2)
		if len(parts) != 2 {
			return fmt.Errorf("migration %s must begin with a numeric version", entry.Name())
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		state := MigrationStateReleased
		if strings.EqualFold(filepath.Base(filepath.Dir(path)), "archived") {
			state = MigrationStateArchived
		}
		result = append(result, Migration{Version: version, Name: entry.Name(), Statements: SplitSQL(string(data)), Hash: migrationHash(data), State: state})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version < result[j].Version })
	for i := 1; i < len(result); i++ {
		if result[i-1].Version == result[i].Version {
			return nil, fmt.Errorf("duplicate migration version %d", result[i].Version)
		}
	}
	return result, nil
}

func ApplyMigrations(ctx context.Context, store *Store, migrations []Migration) error {
	_, err := ApplyMigrationsWithOptions(ctx, store, migrations, MigrationOptions{})
	return err
}

func ApplyMigrationsWithOptions(ctx context.Context, store *Store, migrations []Migration, options MigrationOptions) (MigrationResult, error) {
	var result MigrationResult
	if store == nil || store.DB == nil {
		return result, fmt.Errorf("migration store is unavailable")
	}
	if err := ensureMigrationMetadata(ctx, store); err != nil {
		return result, err
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	seen := make(map[int]struct{}, len(migrations))
	seenNames := make(map[string]struct{}, len(migrations))
	for index := range migrations {
		migration := &migrations[index]
		if _, exists := seen[migration.Version]; exists {
			return result, fmt.Errorf("duplicate migration version %d", migration.Version)
		}
		seen[migration.Version] = struct{}{}
		if _, exists := seenNames[migration.Name]; exists {
			return result, fmt.Errorf("duplicate migration name %s", migration.Name)
		}
		seenNames[migration.Name] = struct{}{}
		if migration.State == "" {
			migration.State = MigrationStateReleased
		}
		if migration.Hash == "" {
			migration.Hash = migrationHash([]byte(strings.Join(migration.Statements, "\n")))
		}
		var applied migrationRecord
		err := store.DB.QueryRowContext(ctx, "SELECT name, hash, state FROM updates WHERE name = ?", migration.Name).Scan(&applied.Name, &applied.Hash, &applied.State)
		if err == nil {
			if !options.RedundancyChecks || (!options.ArchivedRedundancy && applied.State == MigrationStateArchived && migration.State == MigrationStateArchived) {
				continue
			}
			if applied.Hash == migration.Hash {
				if applied.State != migration.State {
					if _, err := store.DB.ExecContext(ctx, "UPDATE updates SET hash = ?, state = ? WHERE name = ?", migration.Hash, migration.State, migration.Name); err != nil {
						return result, err
					}
				}
				continue
			}
			if options.AllowRehash && applied.Hash == "" {
				if _, err := store.DB.ExecContext(ctx, "UPDATE updates SET hash = ?, state = ? WHERE name = ?", migration.Hash, migration.State, migration.Name); err != nil {
					return result, err
				}
				result.Rehashed++
				continue
			}
			if err := applyMigration(ctx, store, *migration, true); err != nil {
				return result, err
			}
			result.Applied++
			continue
		}
		if err != sql.ErrNoRows {
			return result, err
		}
		renamed, renameErr := renameMigratedUpdate(ctx, store, migrations, *migration)
		if renameErr != nil {
			return result, renameErr
		}
		if renamed {
			continue
		}
		if err := applyMigration(ctx, store, *migration, false); err != nil {
			return result, err
		}
		if migration.State == MigrationStateArchived {
			result.Archived++
		}
		result.Applied++
	}
	if options.CleanOrphans || options.CleanOrphansMax != 0 {
		rows, err := store.DB.QueryContext(ctx, "SELECT name FROM updates")
		if err != nil {
			return result, err
		}
		var orphaned []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				rows.Close()
				return result, err
			}
			if !migrationNamesContain(migrations, name) {
				orphaned = append(orphaned, name)
			}
		}
		if err := rows.Close(); err != nil {
			return result, err
		}
		cleanup := options.CleanOrphans
		if options.CleanOrphansMax != 0 {
			cleanup = options.CleanOrphansMax < 0 || len(orphaned) <= options.CleanOrphansMax
		}
		if !cleanup {
			return result, nil
		}
		for _, name := range orphaned {
			if _, err := store.DB.ExecContext(ctx, "DELETE FROM updates WHERE name = ?", name); err != nil {
				return result, err
			}
			result.Orphans++
		}
	}
	return result, nil
}

type migrationRecord struct {
	Name  string
	Hash  string
	State string
}

func renameMigratedUpdate(ctx context.Context, store *Store, migrations []Migration, migration Migration) (bool, error) {
	if migration.Hash == "" {
		return false, nil
	}
	names := make(map[string]struct{}, len(migrations))
	for _, candidate := range migrations {
		names[candidate.Name] = struct{}{}
	}
	rows, err := store.DB.QueryContext(ctx, "SELECT name FROM updates WHERE hash = ?", migration.Hash)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var oldName string
		if err := rows.Scan(&oldName); err != nil {
			return false, err
		}
		if _, present := names[oldName]; present {
			continue
		}
		if _, err := store.DB.ExecContext(ctx, "UPDATE updates SET name = ? WHERE name = ?", migration.Name, oldName); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func ensureMigrationMetadata(ctx context.Context, store *Store) error {
	meta := "CREATE TABLE IF NOT EXISTS updates (name TEXT NOT NULL PRIMARY KEY, hash TEXT DEFAULT '', state TEXT NOT NULL DEFAULT 'RELEASED', timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, speed INTEGER NOT NULL DEFAULT 0)"
	if store.Backend != BackendSQLite {
		meta = "CREATE TABLE IF NOT EXISTS updates (name VARCHAR(200) NOT NULL PRIMARY KEY, hash CHAR(40) DEFAULT '', state VARCHAR(16) NOT NULL DEFAULT 'RELEASED', timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, speed INT UNSIGNED NOT NULL DEFAULT 0)"
	}
	if _, err := store.DB.ExecContext(ctx, meta); err != nil {
		return err
	}
	columns := []struct{ name, definition string }{{"hash", "TEXT NOT NULL DEFAULT ''"}, {"state", "TEXT NOT NULL DEFAULT 'RELEASED'"}, {"speed", "INTEGER NOT NULL DEFAULT 0"}}
	for _, column := range columns {
		if _, err := store.DB.ExecContext(ctx, "ALTER TABLE updates ADD COLUMN "+column.name+" "+column.definition); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") && !strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return err
		}
	}
	return nil
}

func applyMigration(ctx context.Context, store *Store, migration Migration, replacing bool) error {
	start := time.Now()
	tx, err := store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, statement := range migration.Statements {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d %s: %w", migration.Version, migration.Name, err)
		}
	}
	speed := time.Since(start).Milliseconds()
	query := "INSERT INTO updates (name, hash, state, speed) VALUES (?, ?, ?, ?)"
	args := []any{migration.Name, migration.Hash, migration.State, speed}
	if replacing {
		query = "UPDATE updates SET hash = ?, state = ?, speed = ?, timestamp = CURRENT_TIMESTAMP WHERE name = ?"
		args = []any{migration.Hash, migration.State, speed, migration.Name}
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func migrationHash(data []byte) string {
	digest := sha1.Sum(data)
	return hex.EncodeToString(digest[:])
}

func migrationNamesContain(migrations []Migration, name string) bool {
	for _, migration := range migrations {
		if migration.Name == name {
			return true
		}
	}
	return false
}
