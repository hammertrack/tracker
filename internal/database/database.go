package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	gomigrate "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"

	cfg "pedro.to/hammertrace/tracker/internal/config"
	"pedro.to/hammertrace/tracker/internal/errors"
)

var (
	ErrDBBadArguments = errors.New("connection arguments could not be validated")
	ErrDBConnTimeout  = errors.New("test connection with database timed out")
	ErrDBMigration    = errors.New("database migration failed")
)

func src() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName,
	)
}

// pingUntil tries to connect to the database. If the database is not ready it will
// try again until the given context is canceled
func pingUntil(ctx context.Context, db *sql.DB) (err error) {
	timer := time.NewTicker(time.Second)
	for {
		select {
		case <-timer.C:
			if err = db.Ping(); err == nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func migrate(db *sql.DB) (err error) {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return
	}

	mg, err := gomigrate.NewWithDatabaseInstance(
		"file://internal/database/migrations",
		"postgres", driver,
	)
	if err != nil {
		return
	}

	if err = mg.Steps(cfg.DBVersion); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = nil
			log.Print("  → no new migrations found, no changes were applied")
		}
	}
	return
}

func New(doMigrate bool) *sql.DB {
	log.Print("validating database connection...")
	db, err := sql.Open("postgres", src())
	if err != nil {
		errors.WrapFatalWithContext(ErrDBBadArguments, struct {
			Cause string
		}{err.Error()})
	}
	log.Print("  ✓ database parameters")

	log.Print("testing database connection...")
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.DBConnTimeoutSeconds)*time.Second)
	defer cancel()
	if err = pingUntil(ctx, db); err != nil {
		errors.WrapFatalWithContext(ErrDBConnTimeout, struct {
			Cause string
		}{err.Error()})
	}
	log.Print("  ✓ database connection")

	if doMigrate {
		log.Print("applying migrations...")
		if err := migrate(db); err != nil {
			errors.WrapFatalWithContext(ErrDBMigration, struct {
				Cause string
			}{err.Error()})
		}
		log.Printf("  ✓ database is up to date - v%d", cfg.DBVersion)
	}

	return db
}
