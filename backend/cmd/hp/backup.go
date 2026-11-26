package main

// hp backup [-every 24h]   copy the database and files into HP_BACKUP_DIR (keeps HP_BACKUP_KEEP)
// hp backup verify NAME    check a backup reads back (database copy and files), without restoring it
// hp restore -yes NAME     put a backup back (stop the API and workers first)
// hp fetch [NAME]          bring a backup back from off the server (HP_OFFSITE_*) into HP_BACKUP_DIR

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

func backupOptions() (backup.Options, error) {
	o := backup.Options{AdminURL: adminURL(), FilesDir: os.Getenv("HP_FILES_DIR"), Dir: os.Getenv("HP_BACKUP_DIR"), Keep: 14}
	if o.Dir == "" {
		return o, errors.New("HP_BACKUP_DIR is required (where backups go)")
	}
	if o.FilesDir == "" {
		return o, errors.New("HP_FILES_DIR is required (the uploaded files to back up)")
	}
	off, err := offsiteOptions()
	if err != nil {
		return o, err
	}
	o.Offsite = off
	if v := os.Getenv("HP_BACKUP_KEEP"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return o, fmt.Errorf("HP_BACKUP_KEEP must be a whole number of at least 1, got %q", v)
		}
		o.Keep = n
	}
	return o, nil
}

// offsiteOptions reads where backups are also copied, off this server: an
// S3-compatible bucket (HP_OFFSITE_S3_*), or a folder on another disk
// (HP_OFFSITE_DIR). Neither: not set up.
func offsiteOptions() (backup.Offsite, error) {
	bucket, dir := os.Getenv("HP_OFFSITE_S3_BUCKET"), os.Getenv("HP_OFFSITE_DIR")
	switch {
	case bucket != "" && dir != "":
		return backup.Offsite{}, errors.New("set HP_OFFSITE_S3_BUCKET or HP_OFFSITE_DIR, not both")
	case bucket != "":
		key, secret := os.Getenv("HP_OFFSITE_S3_ACCESS_KEY"), os.Getenv("HP_OFFSITE_S3_SECRET_KEY")
		if key == "" || secret == "" {
			return backup.Offsite{}, errors.New("HP_OFFSITE_S3_ACCESS_KEY and HP_OFFSITE_S3_SECRET_KEY are required with HP_OFFSITE_S3_BUCKET")
		}
		return backup.S3Offsite(bucket, os.Getenv("HP_OFFSITE_S3_ENDPOINT"), os.Getenv("HP_OFFSITE_S3_REGION"), key, secret), nil
	case dir != "":
		return backup.Offsite{Remote: dir}, nil
	}
	return backup.Offsite{}, nil
}

// backupOwner reads HP_BACKUP_OWNER ("uid:gid"): who the backups are handed
// to, so the API (another user than the backup service, in Docker) can list,
// send and delete them. Unset: they stay the backup service's own.
func backupOwner() (uid, gid int, set bool, err error) {
	o, err := ownerSetting("HP_BACKUP_OWNER")
	if o == nil || err != nil {
		return 0, 0, false, err
	}
	return o.UID, o.GID, true, nil
}

// filesOwner reads who the uploaded files belong to: the user the API and
// workers run as (HP_FILES_OWNER, else HP_BACKUP_OWNER: in Docker both are
// 65532:65532). nil: neither is set, and restored files stay this
// process's own (running HiringPlug as yourself, without Docker).
func filesOwner() (*backup.Owner, error) {
	if os.Getenv("HP_FILES_OWNER") != "" {
		return ownerSetting("HP_FILES_OWNER")
	}
	return ownerSetting("HP_BACKUP_OWNER")
}

func ownerSetting(name string) (*backup.Owner, error) {
	v := os.Getenv(name)
	if v == "" {
		return nil, nil //nolint:nilnil // not set: nobody to give the files to
	}
	o, err := backup.ParseOwner(v)
	if err != nil {
		return nil, fmt.Errorf("%s %w", name, err)
	}
	return &o, nil
}

func runBackup(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "verify" {
		return runVerify(ctx, args[1:])
	}
	c := command{name: "backup", synopsis: "[-every 24h] | verify NAME"}
	fs := c.flags()
	every := fs.Duration("every", 0, "keep running: a backup every this long while the nightly backup is on (Settings), and whenever one is asked for; 0 = one now")
	if _, err := c.parse(fs, args); err != nil {
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
	uid, gid, owned, err := backupOwner()
	if err != nil {
		return err
	}
	if err := backup.FailInterrupted(ctx, pool); err != nil {
		return err
	}
	own := func() { // also backups made before this setting, or by hand as root
		if !owned {
			return
		}
		if err := backup.Own(o.Dir, uid, gid); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Warn("could not hand the backups to HP_BACKUP_OWNER", "error", err)
		}
	}
	own()
	for {
		if err := backupTurn(ctx, pool, o, *every, log); err != nil {
			log.Error("backup failed", "error", err)
		}
		own() // straight after a backup is made, so the API can read it at once
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
	// A long backup keeps checking in, so Settings does not take the service for stopped.
	beat, stop := context.WithCancel(ctx)
	go func() {
		for {
			select {
			case <-beat.Done():
				return
			case <-time.After(backup.PollEvery):
				_, _ = backup.CheckIn(beat, pool)
			}
		}
	}()
	dir, runErr := backup.Run(ctx, pool, o)
	stop()
	if err := backup.Answered(ctx, pool); err != nil {
		return errors.Join(runErr, err)
	}
	if runErr == nil {
		log.Info("backup made", "dir", dir, "keep", o.Keep, "asked", c.RequestedAt != nil)
	}
	return runErr
}

func runRestore(ctx context.Context, args []string) error {
	c := command{name: "restore", synopsis: "-yes [-resume-outbound] [-resume-paid-ai] hiringplug-YYYYMMDD-HHMMSS", min: 1, max: 1}
	fs := c.flags()
	yes := fs.Bool("yes", false, "replace the database and files with the backup's")
	resume := fs.Bool("resume-outbound", false, "let the emails, calendar updates, webhooks and automations waiting in the backup run (they may have happened already); without it they are held until released in Settings > System or with hp jobs. Paid AI is not affected: see -resume-paid-ai")
	resumeAI := fs.Bool("resume-paid-ai", false, "let the paid AI work waiting in the backup run (it may have been billed already), and leave paid AI on after a restore from another server; without it that work is held like the emails, and paid AI is paused until hp paid-ai on (or Settings > System)")
	names, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	name := names[0]
	if !backup.ValidName(name) {
		return fmt.Errorf("%q is not a backup name: they look like hiringplug-YYYYMMDD-HHMMSS (the folders in HP_BACKUP_DIR; hp fetch brings one from off the server)", name)
	}
	if !*yes {
		return fmt.Errorf("restoring replaces every company's data and files with the backup's: stop the API and workers, then run hp restore -yes %s", name)
	}
	o, err := backupOptions()
	if err != nil {
		return err
	}
	if o.AdminURL == "" {
		return errors.New("HP_DATABASE_ADMIN_URL is required")
	}
	if o.Owner, err = filesOwner(); err != nil {
		return err
	}
	if err := backup.Restore(ctx, o, name); err != nil {
		return err
	}
	// A restore recreates pg_trgm, whose functions then lose their mark.
	if err := database.AllowSearchIndexes(ctx, o.AdminURL, os.Stderr); err != nil {
		return err
	}
	if err := settleJobs(ctx, o, name, *resume, *resumeAI); err != nil {
		return err
	}
	if err := verifyRestoredFiles(ctx, o); err != nil {
		return err
	}
	fmt.Println("restored", name, "- start the API and workers")
	return nil
}

func runFetch(ctx context.Context, args []string) error {
	c := command{name: "fetch", synopsis: "[hiringplug-YYYYMMDD-HHMMSS]", max: 1}
	names, err := c.parse(c.flags(), args)
	if err != nil {
		return err
	}
	wanted := ""
	if len(names) == 1 {
		wanted = names[0]
	}
	o, err := backupOptions()
	if err != nil {
		return err
	}
	// Not this server's own backups (on a new server: only its empty database).
	server, err := backup.ServerID(ctx, o.AdminURL)
	if err != nil {
		return err
	}
	name, err := backup.Fetch(ctx, o.Offsite, o.Dir, wanted, server)
	if err != nil {
		return err
	}
	fmt.Printf("fetched %s into %s - to put it back, stop the API and workers, then: hp restore -yes %s\n", name, o.Dir, name)
	return nil
}

// runVerify checks a backup reads back without restoring it (the API and
// workers keep running): every backup is checked as it is made, and this
// checks one again, after copying it or bringing it back (hp fetch).
func runVerify(ctx context.Context, args []string) error {
	c := command{name: "backup verify", synopsis: "NAME", min: 1, max: 1}
	fs := c.flags()
	names, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	o, err := backupOptions()
	if err != nil {
		return err
	}
	got, err := backup.Verify(ctx, o, names[0])
	if err != nil {
		return err
	}
	fmt.Printf("%s reads back: its database copy (%d tables with data) and %d files\n", names[0], got.Tables, got.Files)
	return nil
}
