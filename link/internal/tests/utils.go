package tests

import (
	"os"
	"strconv"
	"testing"
)

// ENV constants for testing
const (
	EnvTestPostgres = "TEST_POSTGRES"
)

func GuardPostgresTests(t *testing.T) {
	t.Helper()

	if EnvBool(EnvTestPostgres) {
		return
	}

	t.Skipf("Postgres tests are disabled. Set env %s=true to enable them.", EnvTestPostgres)
}

func EnvBool(env string) bool {
	b, _ := strconv.ParseBool(os.Getenv(env))

	return b
}
