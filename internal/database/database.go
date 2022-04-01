package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gocql/gocql"
	gomigrate "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/cassandra"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	// _ "github.com/lib/pq"

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
func pingUntil(ctx context.Context, c *gocql.ClusterConfig) (s *gocql.Session, err error) {
	timer := time.NewTicker(time.Second)
	for {
		select {
		case <-timer.C:
			if s, err = c.CreateSession(); err == nil {
				var t string
				if err = s.Query("SELECT now() FROM system.local").
					WithContext(ctx).
					Consistency(gocql.One).
					Scan(&t); err == nil {
					return
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func migrate(s *gocql.Session) (err error) {
	driver, err := cassandra.WithInstance(s, &cassandra.Config{
		MultiStatementEnabled: true,
		KeyspaceName:          cfg.DBKeyspace,
	})
	if err != nil {
		return
	}

	mg, err := gomigrate.NewWithDatabaseInstance(
		"file://internal/database/migrations/cassandra",
		"cassandra", driver,
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

func New(doMigrate bool) *gocql.Session {
	cluster := gocql.NewCluster(fmt.Sprintf("%s:%s", cfg.DBHost, cfg.DBPort))
	cluster.Keyspace = cfg.DBKeyspace
	cluster.ProtoVersion = 4
	cluster.Consistency = gocql.Quorum

	log.Print("testing database connection...")
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.DBConnTimeoutSeconds)*time.Second)
	defer cancel()

	s, err := pingUntil(ctx, cluster)
	if err != nil {
		errors.WrapFatalWithContext(ErrDBConnTimeout, struct {
			Cause string
		}{err.Error()})
	}
	log.Print("  ✓ database connection")

	if doMigrate {
		log.Print("applying migrations...")
		if err := migrate(s); err != nil {
			errors.WrapFatalWithContext(ErrDBMigration, struct {
				Cause string
			}{err.Error()})
		}
		log.Printf("  ✓ database is up to date - v%d", cfg.DBVersion)
	}

	return s
}
