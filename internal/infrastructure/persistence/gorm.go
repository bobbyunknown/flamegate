package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open creates a GORM database connection.
// For SQLite: dsn is the file path (e.g. "file:./data/flamegate.db").
// For Postgres: dsn is the connection string (e.g. "postgres://user:pass@host:5432/dbname?sslmode=disable").
func Open(driver, dsn string) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch driver {
	case "sqlite", "sqlite3":
		dialector = sqlite.Open(dsn)
	case "postgres", "postgresql", "pg":
		dialector = postgres.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 &filterNotFoundLogger{l: logger.Default.LogMode(logger.Error)},
	})
	if err != nil {
		return nil, fmt.Errorf("gorm open %s: %w", driver, err)
	}

	return db, nil
}

// filterNotFoundLogger wraps GORM's logger to suppress ErrRecordNotFound logs.
// GORM internally logs "record not found" even at Error level for First() queries.
type filterNotFoundLogger struct {
	l logger.Interface
}

func (l *filterNotFoundLogger) LogMode(level logger.LogLevel) logger.Interface {
	return &filterNotFoundLogger{l: l.l.LogMode(level)}
}

func (l *filterNotFoundLogger) Info(ctx context.Context, msg string, data ...any) {
	l.l.Info(ctx, msg, data...)
}

func (l *filterNotFoundLogger) Warn(ctx context.Context, msg string, data ...any) {
	l.l.Warn(ctx, msg, data...)
}

func (l *filterNotFoundLogger) Error(ctx context.Context, msg string, data ...any) {
	l.l.Error(ctx, msg, data...)
}

func (l *filterNotFoundLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		l.l.Trace(ctx, begin, fc, err)
	}
}
