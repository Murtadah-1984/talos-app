package postgres

import (
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrations is populated by cmd/platform-api (and platform-worker) via
// embed.FS pointing at the top-level migrations/ directory, since Go embed
// paths cannot escape the embedding package's directory tree.
type Migrations = embed.FS

// Migrate applies every pending up migration found in fsys (rooted at dir)
// against dsn. It is idempotent: already-applied migrations are skipped.
func Migrate(dsn string, fsys embed.FS, dir string) error {
	src, err := iofs.New(fsys, dir)
	if err != nil {
		return fmt.Errorf("loading embedded migrations: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		return fmt.Errorf("initializing migrator: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("applying migrations: %w", err)
	}
	return nil
}
