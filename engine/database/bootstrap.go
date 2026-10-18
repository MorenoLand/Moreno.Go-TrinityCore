package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/config"
)

var (
	createTablePattern = regexp.MustCompile(`(?is)^CREATE\s+TABLE\s+(IF\s+NOT\s+EXISTS\s+)?`)
	tableNamePattern   = regexp.MustCompile(`(?is)^CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([^\s(]+)`)
)

func EnsureSchemas(ctx context.Context, c config.Config, set *Set) error {
	stores := []struct {
		store *Store
		mask  uint32
	}{{set.Auth, 1}, {set.Characters, 2}, {set.World, 4}}
	for _, item := range stores {
		store := item.store
		if err := ensureSchema(ctx, c, store); err != nil {
			return err
		}
		if c.UpdatesEnableDatabases&item.mask != 0 {
			if err := applyConfiguredMigrations(ctx, c, store); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyConfiguredMigrations(ctx context.Context, c config.Config, store *Store) error {
	dialect := "mysql"
	if store.Backend == BackendSQLite {
		dialect = "sqlite"
	}
	candidates := []string{
		filepath.Join(c.SchemaDir, dialect, "updates", store.Name),
		filepath.Join(c.SchemaDir, dialect, store.Name, "updates"),
		filepath.Join(c.SchemaDir, "updates", store.Name),
	}
	directories := make([]struct{ path, state string }, 0, len(candidates)+4)
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			directories = append(directories, struct{ path, state string }{path: candidate, state: MigrationStateReleased})
		}
	}
	rows, err := store.DB.QueryContext(ctx, "SELECT path, state FROM updates_include ORDER BY path")
	if err == nil {
		for rows.Next() {
			var path, state string
			if scanErr := rows.Scan(&path, &state); scanErr != nil {
				rows.Close()
				return scanErr
			}
			if strings.HasPrefix(path, "$") {
				path = filepath.Join(c.SchemaDir, strings.TrimLeft(strings.TrimPrefix(path, "$"), `/\\`))
			}
			if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
				directories = append(directories, struct{ path, state string }{path: path, state: state})
			}
		}
		if closeErr := rows.Close(); closeErr != nil {
			return closeErr
		}
	} else if !strings.Contains(strings.ToLower(err.Error()), "no such table") && !strings.Contains(strings.ToLower(err.Error()), "doesn't exist") {
		return err
	}
	if len(directories) == 0 {
		return nil
	}
	migrations := make([]Migration, 0)
	for _, directory := range directories {
		loaded, loadErr := LoadMigrations(directory.path)
		if loadErr != nil {
			return fmt.Errorf("%s migration discovery %s: %w", store.Name, directory.path, loadErr)
		}
		// C++ UpdateFetcher::AppliedFileEntry::StateConvert: only "RELEASED" reads
		// as RELEASED — anything else, including an empty state, is ARCHIVED.
		state := strings.ToUpper(directory.state)
		if state != MigrationStateReleased {
			state = MigrationStateArchived
		}
		for index := range loaded {
			loaded[index].State = state
		}
		migrations = append(migrations, loaded...)
	}
	_, err = ApplyMigrationsWithOptions(ctx, store, migrations, MigrationOptions{RedundancyChecks: c.UpdatesRedundancy, AllowRehash: c.UpdatesAllowRehash, ArchivedRedundancy: c.UpdatesArchivedRedundancy, CleanOrphansMax: c.UpdatesCleanDeadReferencesMaxCount})
	if err != nil {
		return fmt.Errorf("%s migrations: %w", store.Name, err)
	}
	return nil
}

func ensureSchema(ctx context.Context, c config.Config, store *Store) error {
	meta := "CREATE TABLE IF NOT EXISTS trinitygo_schema (id INTEGER NOT NULL PRIMARY KEY, version INTEGER NOT NULL)"
	if store.Backend != BackendSQLite {
		meta = "CREATE TABLE IF NOT EXISTS trinitygo_schema (id INT NOT NULL PRIMARY KEY, version INT NOT NULL)"
	}
	if _, err := store.Exec(ctx, meta); err != nil {
		return fmt.Errorf("%s schema metadata: %w", store.Name, err)
	}
	var version int
	err := store.DB.QueryRowContext(ctx, "SELECT version FROM trinitygo_schema WHERE id = 1").Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := store.Exec(ctx, "INSERT INTO trinitygo_schema (id, version) VALUES (1, 0)"); err != nil {
			return fmt.Errorf("%s schema metadata insert: %w", store.Name, err)
		}
		version = 0
	} else if err != nil {
		return fmt.Errorf("%s schema metadata read: %w", store.Name, err)
	}
	if version > 0 {
		return nil
	}
	// C++ DBUpdater::Populate: a database that already holds tables was created
	// externally (e.g. by the C++ server) — skip the base file and mark it done
	// instead of re-applying CREATE TABLEs onto a live schema.
	populated, err := databaseHasTables(ctx, store)
	if err != nil {
		return fmt.Errorf("%s schema table check: %w", store.Name, err)
	}
	if populated {
		if _, err := store.Exec(ctx, "UPDATE trinitygo_schema SET version = 1 WHERE id = 1"); err != nil {
			return fmt.Errorf("%s schema metadata update: %w", store.Name, err)
		}
		return nil
	}
	if !c.UpdatesAutoSetup {
		return nil
	}
	dialect := "mysql"
	if store.Backend == BackendSQLite {
		dialect = "sqlite"
	}
	path := filepath.Join(c.SchemaDir, dialect, store.Name+".sql")
	script, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s schema file %s: %w", store.Name, path, err)
	}
	tx, err := store.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s schema transaction: %w", store.Name, err)
	}
	for _, statement := range SplitSQL(string(script)) {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("%s schema statement: %w", store.Name, err)
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE trinitygo_schema SET version = 1 WHERE id = 1"); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("%s schema metadata update: %w", store.Name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s schema commit: %w", store.Name, err)
	}
	return nil
}

func databaseHasTables(ctx context.Context, store *Store) (bool, error) {
	if store.Backend == BackendSQLite {
		var count int
		err := store.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name <> 'trinitygo_schema'").Scan(&count)
		return count > 0, err
	}
	rows, err := store.DB.QueryContext(ctx, "SHOW TABLES")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name != "trinitygo_schema" {
			return true, nil
		}
	}
	return false, rows.Err()
}

func normalizeMySQLCreateTable(statement string) string {
	statement = strings.TrimSpace(statement)
	if !createTablePattern.MatchString(statement) {
		return statement
	}
	return createTablePattern.ReplaceAllString(statement, "CREATE TABLE IF NOT EXISTS ")
}
