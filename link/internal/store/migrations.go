package store

import (
	"bytes"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/pkg/errors"
	migrate "github.com/rubenv/sql-migrate"

	"github.com/cosmos/ibc/link/internal/config"
)

const migrationTemplate = `-- +migrate Up
-- todo write your migration up here
-- +migrate Down
-- todo write your migration down here
`

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

func migrateDB(db *sql.DB, dbType string, direction migrate.MigrationDirection, num int) (int, error) {
	src, err := MigrationsSource(dbType)
	if err != nil {
		return 0, errors.Wrapf(err, "migrations source")
	}

	return migrate.ExecMax(db, migrationDialect(dbType), src, direction, num)
}

func migrationStatus(db *sql.DB, dbType string) ([]MigrationStatus, error) {
	src, err := MigrationsSource(dbType)
	if err != nil {
		return nil, errors.Wrapf(err, "migrations source")
	}

	// fetch from migrations dir
	migrations, err := src.FindMigrations()
	if err != nil {
		return nil, errors.Wrap(err, "find migrations")
	}

	dialect := migrationDialect(dbType)

	// fetch from DB
	records, err := migrate.GetMigrationRecords(db, dialect)
	if err != nil {
		return nil, errors.Wrap(err, "get migration records")
	}

	applied := make(map[string]migrate.MigrationRecord, len(records))
	for _, record := range records {
		applied[record.Id] = *record
	}

	// compare: applied or not?
	results := make([]MigrationStatus, 0, len(migrations))
	for _, m := range migrations {
		record, applied := applied[m.Id]
		if !applied {
			results = append(results, MigrationStatus{ID: m.Id})
			continue
		}

		appliedAt := record.AppliedAt.UTC()
		results = append(results, MigrationStatus{
			ID:        m.Id,
			Applied:   applied,
			AppliedAt: &appliedAt,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})

	return results, nil
}

func migrationDialect(dbType string) string {
	if dbType == config.DBTypeSQLite {
		return "sqlite3"
	}

	return dbType
}

func init() {
	migrate.SetTable("migrations")
}
