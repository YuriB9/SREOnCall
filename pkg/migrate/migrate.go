package migrate

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Run applies all pending UP migrations from the given source path against dsn.
// source should be a file URI, e.g. "file://./migrations".
// Returns nil if already up-to-date (migrate.ErrNoChange is swallowed).
func Run(dsn, source string) error {
	m, err := migrate.New(source, dsn)
	if err != nil {
		return fmt.Errorf("migrate.New: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	v, _, _ := m.Version()
	slog.Info("migrations applied", "version", v, "source", source)
	return nil
}
