package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/config"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocoltrace"
)

type Backend string

const (
	BackendSQLite  Backend = "sqlite"
	BackendMySQL   Backend = "mysql"
	BackendMariaDB Backend = "mariadb"
)

type Store struct {
	Name          string
	Backend       Backend
	DB            *sql.DB
	TraceRecorder *protocoltrace.Recorder
}

type Set struct {
	Auth       *Store
	Characters *Store
	World      *Store
}

type SQLStore interface {
	PingContext(context.Context) error
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	PrepareContext(context.Context, string) (*sql.Stmt, error)
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

func OpenSet(ctx context.Context, c config.Config) (*Set, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if c.Backend == string(BackendSQLite) {
		if err := os.MkdirAll(c.DataDir, 0755); err != nil {
			return nil, fmt.Errorf("runtime data directory: %w", err)
		}
	}
	auth, err := open(ctx, "auth", c.Backend, c.AuthDatabaseFile, c.LoginDatabaseInfo, c)
	if err != nil {
		return nil, err
	}
	characters, err := open(ctx, "characters", c.Backend, c.CharactersDatabaseFile, c.CharacterDatabaseInfo, c)
	if err != nil {
		auth.Close()
		return nil, err
	}
	world, err := open(ctx, "world", c.Backend, c.WorldDatabaseFile, c.WorldDatabaseInfo, c)
	if err != nil {
		auth.Close()
		characters.Close()
		return nil, err
	}
	set := &Set{Auth: auth, Characters: characters, World: world}
	if err := EnsureSchemas(ctx, c, set); err != nil {
		_ = set.Close()
		return nil, err
	}
	return set, nil
}

func (s *Set) Close() error {
	var first error
	for _, store := range []*Store{s.Auth, s.Characters, s.World} {
		if store != nil {
			if err := store.DB.Close(); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

func (s *Store) Ping(ctx context.Context) error { return s.DB.PingContext(ctx) }
func (s *Store) Close() error                   { return s.DB.Close() }
func (s *Store) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	result, err := s.DB.ExecContext(ctx, query, args...)
	s.recordDatabaseResult("exec", query, len(args), result, err)
	return result, err
}
func (s *Store) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	rows, err := s.DB.QueryContext(ctx, query, args...)
	s.recordDatabaseEvent("query", query, len(args), err)
	return rows, err
}
func (s *Store) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return s.DB.QueryRowContext(ctx, query, args...)
}
func (s *Store) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return s.DB.QueryRowContext(ctx, query, args...)
}
func (s *Store) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	stmt, err := s.DB.PrepareContext(ctx, query)
	s.recordDatabaseEvent("prepare", query, 0, err)
	return stmt, err
}
func (s *Store) Begin(ctx context.Context, options *sql.TxOptions) (*sql.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, options)
	s.recordDatabaseEvent("begin", "", 0, err)
	return tx, err
}

func (s *Store) recordDatabaseEvent(operation, statement string, argumentCount int, err error) {
	s.recordDatabaseResult(operation, statement, argumentCount, nil, err)
}

func (s *Store) recordDatabaseResult(operation, statement string, argumentCount int, result sql.Result, err error) {
	if s == nil || s.TraceRecorder == nil {
		return
	}
	event := map[string]any{"store": s.Name, "operation": operation, "statement": statement, "argument_count": argumentCount}
	if result != nil {
		if rows, resultErr := result.RowsAffected(); resultErr == nil {
			event["rows_affected"] = rows
		}
		if insertID, resultErr := result.LastInsertId(); resultErr == nil {
			event["last_insert_id"] = insertID
		}
	}
	if err != nil {
		event["error"] = err.Error()
	}
	payload, marshalErr := json.Marshal(event)
	if marshalErr == nil {
		s.TraceRecorder.Record(protocoltrace.ClientToServer, 0, payload, "database")
	}
}

func open(ctx context.Context, name, backend, file, info string, c config.Config) (*Store, error) {
	driver, dsn, kind, err := connection(backend, file, info, c)
	if err != nil {
		return nil, fmt.Errorf("%s database: %w", name, err)
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("%s database: %w", name, err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("%s database: %w", name, err)
	}
	return &Store{Name: name, Backend: kind, DB: db}, nil
}
