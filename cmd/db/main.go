// Command db applies SQL migrations and seeds against each service database.
//
// seed returns the demo to its cast. It is run with every service stopped: it
// first empties the broker queues, so no message from before the seed reaches
// a consumer afterwards, and then loads each service's seed, which removes
// what the service recorded since the last one.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leohteixeira/advisor-radar/internal/envfile"
)

var services = []string{"account_sim", "advisory", "triage", "cases"}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := envfile.Load(filepath.Join(root, ".env")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var runErr error
	switch os.Args[1] {
	case "migrate":
		runErr = migrateAll(ctx, root)
	case "seed":
		flags := flag.NewFlagSet("seed", flag.ExitOnError)
		skipQueues := flags.Bool("skip-queues", false,
			"load the seed without emptying the broker queues; messages published before the seed are still delivered after it")
		flags.Usage = func() {
			fmt.Fprintln(flags.Output(), usage)
			flags.PrintDefaults()
		}
		// ExitOnError: Parse exits with status 2 on a bad flag.
		_ = flags.Parse(os.Args[2:])
		if flags.NArg() > 0 {
			flags.Usage()
			os.Exit(2)
		}
		runErr = seed(ctx, root, *skipQueues, os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, runErr)
		os.Exit(1)
	}
}

const usage = "usage: db migrate | db seed [-skip-queues]"

// seed empties the broker queues and then loads every service's seed. The
// queues go first: when they cannot be emptied, because no broker is
// configured, it does not answer, or a service is still consuming, nothing
// has been changed yet and the services can go on with the state they had.
func seed(ctx context.Context, root string, skipQueues bool, out io.Writer) error {
	if skipQueues {
		fmt.Fprintln(out, "queues: not emptied (-skip-queues); messages published before this seed are still delivered")
	} else if err := purgeBroker(ctx, out); err != nil {
		return err
	}
	return seedAll(ctx, root, out)
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("db: go.mod not found from working directory")
		}
		dir = parent
	}
}

func dsnFor(service string) (string, error) {
	envKeys := map[string]string{
		"account_sim": "ACCOUNT_SIM_DATABASE_URL",
		"advisory":    "ADVISORY_DATABASE_URL",
		"triage":      "TRIAGE_DATABASE_URL",
		"cases":       "CASES_DATABASE_URL",
	}
	key := envKeys[service]
	dsn := os.Getenv(key)
	if dsn == "" {
		return "", fmt.Errorf("db: %s is required", key)
	}
	return dsn, nil
}

func migrateAll(ctx context.Context, root string) error {
	for _, service := range services {
		if err := migrateService(ctx, root, service); err != nil {
			return err
		}
	}
	return nil
}

func migrateService(ctx context.Context, root, service string) error {
	dsn, err := dsnFor(service)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("db: connect %s: %w", service, err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    filename TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`); err != nil {
		return fmt.Errorf("db: ensure migrations table %s: %w", service, err)
	}

	dir := filepath.Join(root, "migrations", service)
	files, err := listSQL(dir)
	if err != nil {
		return err
	}
	for _, name := range files {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE filename = $1)`, name,
		).Scan(&exists); err != nil {
			return fmt.Errorf("db: check migration %s/%s: %w", service, name, err)
		}
		if exists {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("db: read migration %s/%s: %w", service, name, err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("db: begin %s/%s: %w", service, name, err)
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("db: apply %s/%s: %w", service, name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (filename) VALUES ($1)`, name,
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("db: record %s/%s: %w", service, name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("db: commit %s/%s: %w", service, name, err)
		}
		fmt.Printf("migrated %s/%s\n", service, name)
	}
	return nil
}

func seedAll(ctx context.Context, root string, out io.Writer) error {
	for _, service := range services {
		if err := seedService(ctx, root, service, out); err != nil {
			return err
		}
	}
	return nil
}

func seedService(ctx context.Context, root, service string, out io.Writer) error {
	dsn, err := dsnFor(service)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("db: connect %s: %w", service, err)
	}
	defer pool.Close()

	dir := filepath.Join(root, "seeds", service)
	files, err := listSQL(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	// One transaction per service: a failing file rolls back the files before
	// it, so a reseed never leaves cash reset and positions stale, or a
	// service's history removed and its cast missing.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin seed %s: %w", service, err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	for _, name := range files {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("db: read seed %s/%s: %w", service, name, err)
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("db: apply seed %s/%s: %w", service, name, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit seed %s: %w", service, err)
	}
	for _, name := range files {
		fmt.Fprintf(out, "seeded %s/%s\n", service, name)
	}
	return nil
}

func listSQL(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".sql") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}
