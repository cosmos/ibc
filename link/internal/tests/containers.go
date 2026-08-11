// SPDX-License-Identifier: Apache-2.0

package tests

import (
	"context"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// PostgresContainer represents a test docker container with Postgres database.
type PostgresContainer struct {
	t *testing.T

	container testcontainers.Container

	host            string
	port            string
	username        string
	password        string
	defaultDatabase string
}

const (
	postgresImage           = "postgres:17-alpine"
	postgresPort            = "5432/tcp"
	postgresUsername        = "postgres"
	postgresPassword        = "postgres"
	postgresDefaultDatabase = "postgres"
)

// NewPostgresContainer start a new PG container
// Read more: https://golang.testcontainers.org/modules/postgres/
// Automatically cleans up the container after the test.
func NewPostgresContainer(t *testing.T) *PostgresContainer {
	t.Helper()

	t.Logf("Starting postgres container: %s", postgresImage)
	start := time.Now()

	pgContainer, err := postgres.Run(
		context.Background(),
		postgresImage,
		postgres.WithUsername(postgresUsername),
		postgres.WithPassword(postgresPassword),
		postgres.WithDatabase(postgresDefaultDatabase),
		postgres.BasicWaitStrategies(),
		testcontainers.WithExposedPorts(postgresPort),
		testcontainers.WithLogger(noopLogger{}),
	)
	require.NoError(t, err, "unable to start postgres container")

	t.Logf("Postgres container started in %s", time.Since(start))

	t.Cleanup(func() {
		if err = testcontainers.TerminateContainer(pgContainer); err != nil {
			t.Logf("Failed to terminate postgres container: %s", err.Error())
		}
	})

	ctx := context.Background()

	host, err := pgContainer.Host(ctx)
	require.NoError(t, err, "get postgres container host")

	// external port that maps to pg's 5432/tcp in docker
	dockerPort, err := pgContainer.MappedPort(ctx, postgresPort)
	require.NoError(t, err, "get postgres container port")

	return &PostgresContainer{
		t: t,

		container: pgContainer,

		host: host,
		port: dockerPort.Port(),

		username:        postgresUsername,
		password:        postgresPassword,
		defaultDatabase: postgresDefaultDatabase,
	}
}

func (p *PostgresContainer) CreateDB(database string) {
	p.t.Helper()

	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, p.URL(p.defaultDatabase))
	require.NoError(p.t, err, "connect to postgres container")

	//nolint:errcheck
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, "create database "+pgx.Identifier{database}.Sanitize())
	require.NoError(p.t, err)

	p.t.Logf("Created database %s in %s", database, time.Since(start))
}

func (p *PostgresContainer) URL(database string) string {
	p.t.Helper()

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(p.username, p.password),
		Host:   net.JoinHostPort(p.host, p.port),
		Path:   "/" + database,
	}

	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()

	return u.String()
}

type noopLogger struct{}

func (n noopLogger) Printf(_ string, _ ...any) {}
