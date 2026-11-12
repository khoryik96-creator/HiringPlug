package main

// hp backup [-every 24h]   copy the database and files into HP_BACKUP_DIR (keeps HP_BACKUP_KEEP)
// hp restore NAME -yes     put a backup back (stop the API and workers first)

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
)

func backupOptions() (backup.Options, error) {
	o := backup.Options{AdminURL: adminURL(), FilesDir: os.Getenv("HP_FILES_DIR"), Dir: os.Getenv("HP_BACKUP_DIR"), Keep: 14}
	if o.Dir == "" {
		return o, errors.New("HP_BACKUP_DIR is required (where backups go)")
	}
	if o.FilesDir == "" {
		return o, errors.New("HP_FILES_DIR is required (the uploaded files to back up)")
	}
	if v := os.Getenv("HP_BACKUP_KEEP"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return o, fmt.Errorf("HP_BACKUP_KEEP must be a whole number of at least 1, got %q", v)
		}
		o.Keep = n
	}
	return o, nil
}

func runBackup(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	every := fs.Duration("every", 0, "keep running: a backup every this long while the nightly backup is on (Settings), and whenever one is asked for; 0 = one now")
	if err := fs.Parse(args); err != nil {
		return err
	}
	o, err := backupOptions()
	if err != nil {
		return err
	}
	pool, err := adminPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	if *every == 0 {
		dir, err := backup.Run(ctx, pool, o)
		if err == nil {
			fmt.Println(dir)
		}
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "backup")
	for {
		if err := backupTurn(ctx, pool, o, *every, log); err != nil {
			log.Error("backup failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backup.PollEvery):
		}
	}
}

// backupTurn checks in, and makes a backup when one was asked for or the
// nightly one is on and due.
func backupTurn(ctx context.Context, pool *pgxpool.Pool, o backup.Options, every time.Duration, log *slog.Logger) error {
	c, err := backup.CheckIn(ctx, pool)
	if err != nil {
		return err
	}
	last, err := backup.LastStarted(ctx, pool)
	if err != nil || !backup.Due(time.Now(), c, last, every) {
		return err
	}
	dir, runErr := backup.Run(ctx, pool, o)
	if err := backup.Answered(ctx, pool); err != nil {
		return errors.Join(runErr, err)
	}
	if runErr == nil {
		log.Info("backup made", "dir", dir, "keep", o.Keep, "asked", c.RequestedAt != nil)
	}
	return runErr
}

func runRestore(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "replace the database and files with the backup's")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: hp restore hiringplug-YYYYMMDD-HHMMSS -yes")
	}
	if !*yes {
		return errors.New("restoring replaces every company's data and files with the backup's: add -yes (and stop the API and workers first)")
	}
	o, err := backupOptions()
	if err != nil {
		return err
	}
	if o.AdminURL == "" {
		return errors.New("HP_DATABASE_ADMIN_URL is required")
	}
	if err := backup.Restore(ctx, o, fs.Arg(0)); err != nil {
		return err
	}
	fmt.Println("restored", fs.Arg(0), "- run `hp bootstrap` if this backup is from an older version, then start the API and workers")
	return nil
}
