//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/store"
)

// Creates a new migration for all store drivers as separate files.
// Delete unused file if you want to migrate only a specific db driver
func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run scripts/migratenew.go [name]")
		os.Exit(1)
	}

	name := os.Args[1]

	dbTypes := []string{
		config.DBTypeSQLite,
		config.DBTypePostgres,
	}

	if err := crateMigrations(name, dbTypes); err != nil {
		fmt.Printf("Error creating migrations: %v\n", err)
		os.Exit(1)
	}
}

func crateMigrations(name string, engines []string) error {
	for _, engine := range engines {
		migrationsDir, err := absMigrationsDir(engine)
		if err != nil {
			return fmt.Errorf("unable to get absolute path %s: %w", engine, err)
		}

		filename, err := store.CreateMigration(name, migrationsDir)
		if err != nil {
			return fmt.Errorf("unable to create migration for %s: %w", engine, err)
		}

		fmt.Printf("✔︎ Created %s\n", filename)
	}

	return nil
}

func absMigrationsDir(engine string) (string, error) {
	const pkg = "internal/store/migrations"

	return filepath.Abs(fmt.Sprintf("%s/%s", pkg, engine))
}
