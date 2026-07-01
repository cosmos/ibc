package store

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"

	stderrors "errors"
	migrate "github.com/rubenv/sql-migrate"
)

const migrationTemplate = `-- +migrate Up
-- todo write your migration up here
-- +migrate Down
-- todo write your migration down here
`

const migrationTable = "migrations"

// MigrationStatus represents migration entry in the database.
type MigrationStatus struct {
	ID        string     `json:"id"`
	Applied   bool       `json:"applied"`
	AppliedAt *time.Time `json:"appliedAt,omitempty"`
}

//go:embed migrations/*
var migrationsFS embed.FS

func MigrationsSource(dbType string) (migrate.MigrationSource, error) {
	if dbType != config.DBTypeSQLite && dbType != config.DBTypePostgres {
		return nil, fmt.Errorf("invalid database type: %s", dbType)
	}

	return migrate.EmbedFileSystemMigrationSource{
		FileSystem: migrationsFS,
		Root:       fmt.Sprintf("migrations/%s", dbType),
	}, nil
}

func CreateMigration(name, migrationsDir string) (string, error) {
	info, err := os.Stat(migrationsDir)
	if err != nil {
		return "", errors.Wrapf(err, "stat migrations directory %q", migrationsDir)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("migrations path is not a directory: %s", migrationsDir)
	}

	// will be used to generate migration name
	// Deleting a migration file with existing number (eg. 011-...) in the database should be avoided!
	count, err := countFilesInDir(migrationsDir)
	if err != nil {
		return "", errors.Wrap(err, "count files")
	}

	var (
		fileName = fmt.Sprintf("%.3d-%s.sql", count+1, name)
		fqn      = filepath.Join(migrationsDir, fileName)
		content  = []byte(migrationTemplate)
	)

	if _, err := migrate.ParseMigration(fileName, bytes.NewReader(content)); err != nil {
		return "", errors.Wrap(err, "invalid template")
	}

	if err := os.WriteFile(fqn, content, 0o644); err != nil {
		return "", errors.Wrap(err, "unable to write migration file")
	}

	return fqn, nil
}

// non recursive, counts only .sql files
func countFilesInDir(dir string) (int, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return 0, errors.Wrapf(err, "read directory %s", dir)
	}

	var count int
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".sql" {
			count++
		}
	}

	return count, nil
}

func migrateDB(
	ctx context.Context,
	db *sql.DB,
	dbType string,
	direction migrate.MigrationDirection,
	num int,
) (int, error) {
	src, err := MigrationsSource(dbType)
	if err != nil {
		return 0, errors.Wrapf(err, "migrations source")
	}

	return migrate.ExecMaxContext(ctx, db, migrationDialect(dbType), src, direction, num)
}

func migrationStatus(ctx context.Context, db *sql.DB, dbType string) ([]MigrationStatus, error) {
	src, err := MigrationsSource(dbType)
	if err != nil {
		return nil, errors.Wrapf(err, "migrations source")
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}

	// fetch from migrations dir
	migrations, err := src.FindMigrations()
	if err != nil {
		return nil, errors.Wrap(err, "find migrations")
	}

	applied, err := migrationRecords(ctx, db, dbType)
	if err != nil {
		return nil, errors.Wrap(err, "get migration records")
	}

	// compare: applied or not?
	results := make([]MigrationStatus, 0, len(migrations))
	for _, m := range migrations {
		appliedAt, ok := applied[m.Id]
		if !ok {
			results = append(results, MigrationStatus{ID: m.Id})
			continue
		}

		appliedAt = appliedAt.UTC()
		results = append(results, MigrationStatus{
			ID:        m.Id,
			Applied:   true,
			AppliedAt: &appliedAt,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})

	return results, nil
}

func migrationRecords(ctx context.Context, db *sql.DB, dbType string) (map[string]time.Time, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, applied_at FROM "+migrationTable+" ORDER BY id ASC")
	if err != nil {
		if isMissingMigrationTable(err, dbType) {
			return nil, nil
		}

		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	records := make(map[string]time.Time)
	for rows.Next() {
		var (
			id        string
			appliedAt time.Time
		)
		if err := rows.Scan(&id, &appliedAt); err != nil {
			return nil, err
		}

		records[id] = appliedAt
	}

	return records, rows.Err()
}

func isMissingMigrationTable(err error, dbType string) bool {
	switch dbType {
	case config.DBTypePostgres:
		var pgErr *pgconn.PgError
		return stderrors.As(err, &pgErr) && pgErr.Code == "42P01"
	case config.DBTypeSQLite:
		return strings.Contains(err.Error(), "no such table")
	default:
		return false
	}
}

func migrationDialect(dbType string) string {
	if dbType == config.DBTypeSQLite {
		return "sqlite3"
	}

	return dbType
}

func init() {
	migrate.SetTable(migrationTable)
}
