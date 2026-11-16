package backup

// Off-site copies: after each backup, rclone copies it to storage away from
// this server (an S3-compatible bucket, or a folder on another disk), so a
// lost server does not take its backups with it.
//
// There, the uploaded files are one copy as of the newest backup ("files"),
// with those since removed set aside by backup ("files-removed"), and each
// backup has a folder of its own with its database and manifest (the manifest
// goes last: a folder with one is whole; it names the server that made it).
// Each server keeps there as many of its own backups as it keeps here, and
// never deletes another's. Fetch brings one back, ready for Restore.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Offsite is where backups are also copied; the zero value is off.
type Offsite struct {
	Remote string   // for rclone: ":s3:bucket/path", or a folder
	Env    []string // rclone's settings (the storage keys), given to rclone alone
	Rclone string   // the program ("" finds rclone on PATH)
}

// On says whether off-site copies are set up.
func (o Offsite) On() bool { return o.Remote != "" }

// S3Offsite is an S3-compatible bucket (AWS, Wasabi, Backblaze B2, Cloudflare
// R2, DigitalOcean Spaces…): bucket may carry a path ("my-bucket/hiringplug");
// endpoint is empty for AWS itself.
func S3Offsite(bucket, endpoint, region, accessKey, secretKey string) Offsite {
	provider := "Other"
	if endpoint == "" {
		provider = "AWS"
	}
	env := []string{
		"RCLONE_S3_PROVIDER=" + provider, "RCLONE_S3_ACCESS_KEY_ID=" + accessKey, "RCLONE_S3_SECRET_ACCESS_KEY=" + secretKey,
		"RCLONE_S3_NO_CHECK_BUCKET=true", // keys limited to one bucket may not list or make buckets
	}
	if endpoint != "" {
		env = append(env, "RCLONE_S3_ENDPOINT="+endpoint)
	}
	if region != "" {
		env = append(env, "RCLONE_S3_REGION="+region)
	}
	return Offsite{Remote: ":s3:" + strings.Trim(bucket, "/"), Env: env}
}

func (o Offsite) path(parts ...string) string {
	return strings.TrimRight(o.Remote, "/") + "/" + strings.Join(parts, "/")
}

// rclone runs one rclone command; its output is returned on failure.
func (o Offsite) rclone(ctx context.Context, args ...string) (string, error) {
	prog := o.Rclone
	if prog == "" {
		p, err := exec.LookPath("rclone")
		if err != nil {
			return "", errors.New("rclone is not installed (the backup service's image has it)")
		}
		prog = p
	}
	cmd := exec.CommandContext(ctx, prog, append([]string{"--config", os.DevNull}, args...)...) //nolint:gosec // our own settings
	cmd.Env = append(os.Environ(), o.Env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("rclone %s: %w: %s", args[0], err, clip(strings.TrimSpace(string(out)), 1500))
	}
	return string(out), nil
}

// copyOffsite copies backup name (in dir) off the server, then removes there
// this server's oldest backups beyond keep. It never deletes what another
// server put there (a test server, or a new one after a loss, may share the
// storage), and uploaded files that are gone here are set aside there
// ("files-removed/<name>"), never deleted, until no backup kept there is
// older than them.
func copyOffsite(ctx context.Context, o Offsite, dir, name, server string, keep int) error {
	src := filepath.Join(dir, name)
	if info, err := os.Stat(filepath.Join(src, filesFolder)); err == nil && info.IsDir() {
		if _, err := o.rclone(ctx, "sync", filepath.Join(src, filesFolder), o.path(filesFolder),
			"--backup-dir", o.path(removedFolder, name)); err != nil {
			return err
		}
	}
	if _, err := o.rclone(ctx, "copy", src, o.path(name), "--exclude", filesFolder+"/**", "--exclude", "manifest.json"); err != nil {
		return err
	}
	if _, err := o.rclone(ctx, "copyto", filepath.Join(src, "manifest.json"), o.path(name, "manifest.json")); err != nil {
		return err
	}
	there, err := o.backups(ctx)
	if err != nil {
		return err
	}
	var ours []string
	for _, n := range there {
		if m, ok := o.manifest(ctx, n); ok && server != "" && m.Server == server {
			ours = append(ours, n)
		}
	}
	for len(ours) > keep { // oldest first
		if _, err := o.rclone(ctx, "purge", o.path(ours[0])); err != nil {
			return err
		}
		there = slices.DeleteFunc(there, func(n string) bool { return n == ours[0] })
		ours = ours[1:]
	}
	return o.pruneRemoved(ctx, there)
}

// The uploaded files set aside at a backup (gone from the server by then).
const removedFolder = "files-removed"

// pruneRemoved deletes the files set aside before the oldest backup kept
// there: no backup still there can need them.
func (o Offsite) pruneRemoved(ctx context.Context, there []string) error {
	removed, err := o.list(ctx, o.path(removedFolder))
	if err != nil || len(there) == 0 {
		return err
	}
	for _, n := range removed {
		if n < there[0] {
			if _, err := o.rclone(ctx, "purge", o.path(removedFolder, n)); err != nil {
				return err
			}
		}
	}
	return nil
}

// manifest is what a backup there says of itself; ok is false for one not
// whole (its copy was cut off: the manifest goes last).
type manifest struct {
	Server string `json:"server"` // the database server that made it
}

func (o Offsite) manifest(ctx context.Context, name string) (manifest, bool) {
	out, err := o.rclone(ctx, "cat", o.path(name, "manifest.json"))
	var m manifest
	if err != nil || json.Unmarshal([]byte(out), &m) != nil {
		return manifest{}, false
	}
	return m, true
}

// backups lists the backups off the server, oldest first.
func (o Offsite) backups(ctx context.Context) ([]string, error) {
	return o.list(ctx, o.Remote)
}

// list lists the backup-named folders in remote, oldest first.
func (o Offsite) list(ctx context.Context, remote string) ([]string, error) {
	out, err := o.rclone(ctx, "lsf", "--dirs-only", remote)
	if err != nil {
		// A bucket path or folder not made yet holds none.
		if strings.Contains(err.Error(), "directory not found") {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if n := strings.TrimSuffix(strings.TrimSpace(line), "/"); nameRe.MatchString(n) {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	return names, nil
}

// Fetch brings a backup back from off the server into dir, whole, files
// included; then Restore can put it back. It returns the backup's name. ""
// or "latest" is the newest whole one made by another database server than
// server (the one fetching, which on a new server has backed up only its own
// empty database), or else the newest whole one.
func Fetch(ctx context.Context, o Offsite, dir, name, server string) (string, error) {
	if !o.On() {
		return "", errors.New("no off-site storage is set up (HP_OFFSITE_*)")
	}
	there, err := o.backups(ctx)
	if err != nil {
		return "", err
	}
	if name == "" || name == "latest" {
		if name, err = o.latest(ctx, there, server); err != nil {
			return "", err
		}
	}
	if !slices.Contains(there, name) {
		return "", fmt.Errorf("no backup %q off the server (there: %s)", name, strings.Join(there, ", "))
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
		return "", fmt.Errorf("%s is already here: restore it as it is", name)
	}
	// Not ".hiringplug-…partial": a backup made meanwhile clears those away.
	tmp := filepath.Join(dir, ".fetching-"+name)
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(tmp)
		}
	}()
	if _, err := o.rclone(ctx, "copy", o.path(name), tmp); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(tmp, "manifest.json")); err != nil {
		return "", fmt.Errorf("%s off the server is not whole (no manifest): fetch another", name)
	}
	// The files there are as of the newest backup, with those removed since
	// this one set aside by the later backups: together, its files (perhaps
	// with some added since, unused).
	if _, err := o.rclone(ctx, "copy", o.path(filesFolder), filepath.Join(tmp, filesFolder)); err != nil && !strings.Contains(err.Error(), "directory not found") {
		return "", err
	}
	removed, err := o.list(ctx, o.path(removedFolder))
	if err != nil {
		return "", err
	}
	for _, n := range removed {
		if n > name {
			if _, err := o.rclone(ctx, "copy", "--ignore-existing", o.path(removedFolder, n), filepath.Join(tmp, filesFolder)); err != nil {
				return "", err
			}
		}
	}
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		return "", err
	}
	ok = true
	return name, nil
}

// latest is the newest whole backup there made by another server than
// server, or else the newest whole one.
func (o Offsite) latest(ctx context.Context, there []string, server string) (string, error) {
	newestWhole := ""
	for i := len(there) - 1; i >= 0; i-- {
		m, ok := o.manifest(ctx, there[i])
		if !ok {
			continue
		}
		if server == "" || m.Server != server {
			return there[i], nil
		}
		if newestWhole == "" {
			newestWhole = there[i]
		}
	}
	if newestWhole == "" {
		return "", errors.New("there is no whole backup off the server yet")
	}
	return newestWhole, nil
}

// ServerID names the database server at url (PostgreSQL's system
// identifier): a restore does not change it, so a test or new server's
// backups are told apart from the live one's.
func ServerID(ctx context.Context, url string) (string, error) {
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return "", fmt.Errorf("connecting to the database: %s", hidePassword(err.Error(), url))
	}
	defer func() { _ = conn.Close(ctx) }()
	var id string
	err = conn.QueryRow(ctx, `SELECT system_identifier::text FROM pg_control_system()`).Scan(&id)
	return id, err
}
