package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"webide-backend/internal/maintenance"
	"webide-backend/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: perseus-maintain inspect|backup|verify-restore|archive|vacuum")
	}
	dbPath := os.Getenv("BACKEND_DB_PATH")
	if dbPath == "" {
		dbPath = "/data/app.db"
	}
	ctx := context.Background()

	switch os.Args[1] {
	case "inspect":
		flags := flag.NewFlagSet("inspect", flag.ExitOnError)
		quick := flags.Bool("quick", false, "check connectivity, required schema and page metadata (the default)")
		fullIntegrity := flags.Bool("full-integrity", false, "run the full PRAGMA integrity_check")
		_ = flags.Parse(os.Args[2:])
		if *quick && *fullIntegrity {
			fatalf("inspect accepts either --quick or --full-integrity, not both")
		}
		db := openDatabase(ctx, dbPath, true, false)
		defer db.Close()
		report, err := maintenance.InspectWithOptions(ctx, db, dbPath, maintenance.InspectOptions{FullIntegrity: *fullIntegrity})
		writeResult(report, err)
	case "backup":
		flags := flag.NewFlagSet("backup", flag.ExitOnError)
		output := flags.String("output", "", "consistent SQLite backup path")
		_ = flags.Parse(os.Args[2:])
		if *output == "" {
			*output = maintenance.DefaultBackupPath(dbPath, time.Now())
		}
		validatedOutput, err := maintenance.ValidateMaintenanceOutputPath(dbPath, *output)
		if err != nil {
			fatalf("backup output: %v", err)
		}
		db := openDatabase(ctx, dbPath, false, false)
		defer db.Close()
		report, err := maintenance.Backup(ctx, db, dbPath, validatedOutput)
		writeResult(report, err)
	case "verify-restore":
		flags := flag.NewFlagSet("verify-restore", flag.ExitOnError)
		backup := flags.String("backup", "", "verified SQLite backup path")
		userDataRoot := flags.String("user-data-root", os.Getenv("USER_DATA_ROOT"), "user file root included in the verification manifest")
		_ = flags.Parse(os.Args[2:])
		if strings.TrimSpace(*backup) == "" {
			fatalf("verify-restore requires --backup")
		}
		validatedBackup, err := maintenance.ValidateMaintenanceOutputPath(dbPath, *backup)
		if err != nil {
			fatalf("verify-restore backup: %v", err)
		}
		db := openDatabase(ctx, dbPath, true, false)
		defer db.Close()
		report, err := maintenance.VerifyRestore(ctx, db, dbPath, validatedBackup, *userDataRoot)
		writeResult(report, err)
	case "archive":
		flags := flag.NewFlagSet("archive", flag.ExitOnError)
		before := flags.String("before", "30d", "archive data older than this duration")
		output := flags.String("output", "", "gzip JSONL archive path")
		apply := flags.Bool("apply", false, "write, verify and compact archived data")
		_ = flags.Parse(os.Args[2:])
		age, err := maintenance.ParseAge(*before)
		if err != nil {
			fatalf("before: %v", err)
		}
		if *output == "" {
			*output = maintenance.DefaultArchivePath(dbPath, time.Now())
		}
		validatedOutput, err := maintenance.ValidateMaintenanceOutputPath(dbPath, *output)
		if err != nil {
			fatalf("archive output: %v", err)
		}
		db := openDatabase(ctx, dbPath, !*apply, *apply)
		defer db.Close()
		report, err := maintenance.Archive(ctx, db, maintenance.ArchiveOptions{Before: time.Now().UTC().Add(-age), Output: validatedOutput, Apply: *apply})
		writeResult(report, err)
	case "vacuum":
		flags := flag.NewFlagSet("vacuum", flag.ExitOnError)
		apply := flags.Bool("apply", false, "confirm offline database rewrite")
		_ = flags.Parse(os.Args[2:])
		if !*apply {
			fatalf("vacuum is offline-only and requires --apply")
		}
		db := openDatabase(ctx, dbPath, false, true)
		defer db.Close()
		writeResult(map[string]any{"status": "ok", "database": dbPath}, maintenance.Vacuum(ctx, db))
	default:
		fatalf("unknown command %q", os.Args[1])
	}
}

func openDatabase(ctx context.Context, path string, readOnly, migrate bool) *sql.DB {
	var (
		db  *sql.DB
		err error
	)
	if readOnly {
		db, err = store.OpenReadOnly(path)
	} else {
		db, err = store.Open(path)
	}
	if err != nil {
		fatalf("open database: %v", err)
	}
	if !migrate {
		return db
	}
	if err := store.NewAgentRunStore(db).Migrate(ctx); err != nil {
		db.Close()
		fatalf("migrate agent storage: %v", err)
	}
	if err := store.NewDiagnosticStore(db).Migrate(ctx); err != nil {
		db.Close()
		fatalf("migrate diagnostics: %v", err)
	}
	return db
}

func writeResult(value any, err error) {
	if err != nil {
		fatalf("%v", err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fatalf("encode result: %v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
