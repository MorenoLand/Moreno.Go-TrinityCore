package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: dbtool schema|schema-audit|import-sql|repair-aura-guids|verify|statement-audit|statement-exercise")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "schema":
		os.Exit(schema(os.Args[2:]))
	case "schema-audit":
		os.Exit(schemaAudit(os.Args[2:]))
	case "import-sql":
		os.Exit(importSQL(os.Args[2:]))
	case "repair-aura-guids":
		os.Exit(repairAuraGUIDs(os.Args[2:]))
	case "verify":
		os.Exit(verify(os.Args[2:]))
	case "statement-audit":
		os.Exit(statementAudit(os.Args[2:]))
	case "statement-exercise":
		os.Exit(statementExercise(os.Args[2:]))
	default:
		fmt.Fprintf(os.Stderr, "unknown dbtool command %q\n", os.Args[1])
		os.Exit(2)
	}
}

func repairAuraGUIDs(args []string) int {
	fs := flag.NewFlagSet("repair-aura-guids", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	databasePath := fs.String("database", "", "SQLite characters database to repair")
	sourceSQL := fs.String("source-sql", "", "original MySQL characters SQL dump")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *databasePath == "" || *sourceSQL == "" {
		fmt.Fprintln(os.Stderr, "repair-aura-guids requires --database and --source-sql")
		return 2
	}
	if _, err := os.Stat(*databasePath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	db, err := sql.Open("sqlite", *databasePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := db.PingContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	report, err := database.RepairSQLiteAuraGUIDs(context.Background(), db, *sourceSQL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("character_aura source_rows=%d recovered=%d untouched=%d pet_aura source_rows=%d recovered=%d untouched=%d\n", report.CharacterSourceRows, report.CharacterRecovered, report.CharacterSkipped, report.PetSourceRows, report.PetRecovered, report.PetSkipped)
	return 0
}

func schemaAudit(args []string) int {
	fs := flag.NewFlagSet("schema-audit", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	mysqlDir := fs.String("mysql-dir", "sql/mysql", "MySQL schema directory")
	sqliteDir := fs.String("sqlite-dir", "sql/sqlite", "SQLite schema directory")
	output := fs.String("output", "docs/SCHEMA_INVENTORY.md", "audit report path")
	strict := fs.Bool("strict", false, "return failure when dialect definitions differ")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	report, status, err := schemaAuditReport(*mysqlDir, *sqliteDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.WriteFile(*output, []byte(report), 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("wrote %s\n", *output)
	if *strict {
		return status
	}
	return 0
}

type statementAuditFailure struct {
	id  database.StatementID
	err error
}

type statementAuditDatabase struct {
	name               string
	total              int
	prepared           int
	validated          int
	failures           []statementAuditFailure
	validationFailures []statementAuditFailure
}

func statementDatabase(id database.StatementID) (string, error) {
	name := string(id)
	switch {
	case strings.HasPrefix(name, "LOGIN_"):
		return "auth", nil
	case strings.HasPrefix(name, "CHAR_"):
		return "characters", nil
	case strings.HasPrefix(name, "WORLD_"):
		return "world", nil
	default:
		return "", fmt.Errorf("statement %s has no logical database mapping", id)
	}
}

func auditStatementDefinitions(ctx context.Context, db *sql.DB, backend database.Backend, definitions []database.StatementDefinition) (int, []statementAuditFailure) {
	prepared := 0
	failures := make([]statementAuditFailure, 0)
	for _, definition := range definitions {
		query, err := database.StatementSQL(definition.ID, backend)
		if err != nil {
			failures = append(failures, statementAuditFailure{id: definition.ID, err: err})
			continue
		}
		stmt, err := db.PrepareContext(ctx, query)
		if err != nil {
			failures = append(failures, statementAuditFailure{id: definition.ID, err: err})
			continue
		}
		if err := stmt.Close(); err != nil {
			failures = append(failures, statementAuditFailure{id: definition.ID, err: err})
			continue
		}
		prepared++
	}
	return prepared, failures
}

func statementBindCount(query string) int {
	count := 0
	quote := byte(0)
	for index := 0; index < len(query); index++ {
		value := query[index]
		if quote != 0 {
			if value == quote {
				if index+1 < len(query) && query[index+1] == quote {
					index++
					continue
				}
				quote = 0
			}
			continue
		}
		if value == '\'' || value == '"' || value == '`' {
			quote = value
		} else if value == '?' {
			count++
		}
	}
	return count
}

func validateStatementDefinitions(ctx context.Context, db *sql.DB, backend database.Backend, definitions []database.StatementDefinition) (int, []statementAuditFailure) {
	validated := 0
	failures := make([]statementAuditFailure, 0)
	for _, definition := range definitions {
		query, err := database.StatementSQL(definition.ID, backend)
		if err != nil {
			failures = append(failures, statementAuditFailure{id: definition.ID, err: err})
			continue
		}
		args := make([]any, statementBindCount(query))
		rows, err := db.QueryContext(ctx, "EXPLAIN "+query, args...)
		if err != nil {
			failures = append(failures, statementAuditFailure{id: definition.ID, err: err})
			continue
		}
		if err := rows.Close(); err != nil {
			failures = append(failures, statementAuditFailure{id: definition.ID, err: err})
			continue
		}
		validated++
	}
	return validated, failures
}

func auditStatements(ctx context.Context, inputDir string, backend database.Backend) ([]statementAuditDatabase, error) {
	definitions := map[string][]database.StatementDefinition{"auth": {}, "characters": {}, "world": {}}
	for _, definition := range database.AllStatements() {
		name, err := statementDatabase(definition.ID)
		if err != nil {
			return nil, err
		}
		definitions[name] = append(definitions[name], definition)
	}
	names := []string{"auth", "characters", "world"}
	results := make([]statementAuditDatabase, 0, len(names))
	for _, name := range names {
		sort.Slice(definitions[name], func(i, j int) bool { return definitions[name][i].ID < definitions[name][j].ID })
		path := filepath.Join(inputDir, name+".db")
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("%s database %s: %w", name, path, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("%s database path %s is a directory", name, path)
		}
		db, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, fmt.Errorf("%s database %s: %w", name, path, err)
		}
		db.SetMaxOpenConns(1)
		if err := db.PingContext(ctx); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s database %s: %w", name, path, err)
		}
		prepared, failures := auditStatementDefinitions(ctx, db, backend, definitions[name])
		validated, validationFailures := validateStatementDefinitions(ctx, db, backend, definitions[name])
		dbErr := db.Close()
		if dbErr != nil {
			return nil, fmt.Errorf("%s database %s close: %w", name, path, dbErr)
		}
		results = append(results, statementAuditDatabase{name: name, total: len(definitions[name]), prepared: prepared, validated: validated, failures: failures, validationFailures: validationFailures})
	}
	return results, nil
}

func statementAudit(args []string) int {
	fs := flag.NewFlagSet("statement-audit", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("input-dir", ".", "SQLite database directory")
	backendName := fs.String("backend", "sqlite", "statement dialect; only sqlite can be audited against local database files")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	backend := database.Backend(strings.ToLower(*backendName))
	if backend != database.BackendSQLite {
		fmt.Fprintf(os.Stderr, "statement-audit requires --backend=sqlite for local database files\n")
		return 2
	}
	results, err := auditStatements(context.Background(), *dir, backend)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	total, prepared, validated, failures := 0, 0, 0, 0
	for _, result := range results {
		fmt.Printf("%s statements=%d prepared=%d validated=%d failures=%d\n", result.name, result.total, result.prepared, result.validated, len(result.failures)+len(result.validationFailures))
		total += result.total
		prepared += result.prepared
		validated += result.validated
		failures += len(result.failures) + len(result.validationFailures)
		for _, failure := range result.failures {
			fmt.Printf("  %s: %v\n", failure.id, failure.err)
		}
		for _, failure := range result.validationFailures {
			fmt.Printf("  EXPLAIN %s: %v\n", failure.id, failure.err)
		}
	}
	fmt.Printf("total statements=%d prepared=%d validated=%d failures=%d backend=%s\n", total, prepared, validated, failures, backend)
	if failures != 0 {
		return 1
	}
	return 0
}

func schema(args []string) int {
	fs := flag.NewFlagSet("schema", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	auth := fs.String("auth", "", "auth SQL dump")
	characters := fs.String("characters", "", "characters SQL dump")
	world := fs.String("world", "", "world SQL dump")
	out := fs.String("output", "sql", "output schema directory")
	backend := fs.String("backend", "both", "mysql, sqlite, or both")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	inputs := map[string]string{"auth": *auth, "characters": *characters, "world": *world}
	for name, path := range inputs {
		if path == "" {
			fmt.Fprintf(os.Stderr, "--%s is required\n", name)
			return 2
		}
	}
	for dialect, enabled := range map[string]bool{"mysql": *backend == "mysql" || *backend == "both", "sqlite": *backend == "sqlite" || *backend == "both"} {
		if !enabled {
			continue
		}
		for name, path := range inputs {
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			converted, err := database.NormalizeSchemaScript(string(data), dialect)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s %s: %v\n", dialect, name, err)
				return 1
			}
			path := filepath.Join(*out, dialect, name+".sql")
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			if err := os.WriteFile(path, []byte(converted), 0644); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			fmt.Printf("wrote %s\n", path)
		}
	}
	return 0
}

func importSQL(args []string) int {
	fs := flag.NewFlagSet("import-sql", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	out := fs.String("output-dir", ".", "SQLite output directory")
	force := fs.Bool("force", false, "replace existing output databases")
	auth := fs.String("auth", "", "auth SQL dump")
	characters := fs.String("characters", "", "characters SQL dump")
	world := fs.String("world", "", "world SQL dump")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	inputs := map[string]string{"auth": *auth, "characters": *characters, "world": *world}
	for name, path := range inputs {
		if path == "" {
			fmt.Fprintf(os.Stderr, "--%s is required\n", name)
			return 2
		}
		output := filepath.Join(*out, name+".db")
		if err := database.ImportSQLiteDump(context.Background(), path, output, *force); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			return 1
		}
		fmt.Printf("wrote %s\n", output)
	}
	return 0
}

func verify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("input-dir", ".", "SQLite database directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	expected := map[string]int{"auth": 19, "characters": 94, "world": 190}
	for name, tables := range expected {
		path := filepath.Join(*dir, name+".db")
		db, err := sql.Open("sqlite", path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			return 1
		}
		actualTables, views, rows, err := databaseStats(db)
		_ = db.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			return 1
		}
		if actualTables != tables {
			fmt.Fprintf(os.Stderr, "%s: found %d tables, expected %d\n", name, actualTables, tables)
			return 1
		}
		fmt.Printf("%s tables=%d views=%d rows=%d\n", name, actualTables, views, rows)
	}
	return 0
}

func databaseStats(db *sql.DB) (int, int, int64, error) {
	rows, err := db.Query("SELECT type, name FROM sqlite_master WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%' ORDER BY type, name")
	if err != nil {
		return 0, 0, 0, err
	}
	type item struct {
		kind string
		name string
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.kind, &it.name); err != nil {
			rows.Close()
			return 0, 0, 0, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, 0, 0, err
	}
	rows.Close()

	tables, views := 0, 0
	var total int64
	for _, it := range items {
		if it.kind == "view" {
			views++
			continue
		}
		if it.name == "trinitygo_schema" {
			continue
		}
		tables++
		query := `SELECT COUNT(*) FROM "` + strings.ReplaceAll(it.name, `"`, `""`) + `"`
		var count int64
		if err := db.QueryRow(query).Scan(&count); err != nil {
			return 0, 0, 0, err
		}
		total += count
	}
	return tables, views, total, nil
}
