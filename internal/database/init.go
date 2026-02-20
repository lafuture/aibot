package database

import (
	"context"
	"log"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct {
	Pool *pgxpool.Pool
}

// dbURLForMigrate нормализует URL для migrate (ожидает схему postgres://).
func dbURLForMigrate(dbURL string) string {
	return strings.Replace(dbURL, "postgresql://", "postgres://", 1)
}

// runMigrations применяет миграции по migraURL к БД dbURL. Не закрывает пул.
func runMigrations(migraURL, dbURL string) error {
	m, err := migrate.New(migraURL, dbURLForMigrate(dbURL))
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	log.Println("Миграции применены или изменений не было")
	return nil
}

// NewDatabase создаёт пул соединений и при необходимости применяет миграции.
// migraURL — путь или URL к миграциям (например "file://migrations"); если пусто — миграции не выполняются.
func NewDatabase(dbURL, migraURL string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 32
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 0
	cfg.MaxConnIdleTime = 0

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, err
	}
	if migraURL != "" {
		if err := runMigrations(migraURL, dbURL); err != nil {
			pool.Close()
			return nil, err
		}
	}
	return &Postgres{Pool: pool}, nil
}

// Close закрывает пул соединений.
func (p *Postgres) Close() {
	if p != nil && p.Pool != nil {
		p.Pool.Close()
	}
}
