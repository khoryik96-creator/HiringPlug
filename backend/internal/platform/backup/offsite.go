package backup

// Off-site copies: after each backup, rclone copies it to storage away from
// this server (an S3-compatible bucket, or a folder on another disk), so a
// lost server does not take its backups with it.
//
// There, the uploaded files are one copy as of the newest backup ("files"),
// and each backup kept here has a folder of its own with its database and
// manifest (the manifest goes last: a folder with one is whole). Backups no
// longer kept here are removed there too. Fetch brings one back, ready for
// Restore.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
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
// the backups that are not in kept.
func copyOffsite(ctx context.Context, o Offsite, dir, name string, kept []string) error {
	src := filepath.Join(dir, name)
	if info, err := os.Stat(filepath.Join(src, filesFolder)); err == nil && info.IsDir() {
		if _, err := o.rclone(ctx, "sync", filepath.Join(src, filesFolder), o.path(filesFolder)); err != nil {
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
	for _, n := range there {
		if !slices.Contains(kept, n) {
			if _, err := o.rclone(ctx, "purge", o.path(n)); err != nil {
				return err
			}
		}
	}
	return nil
}

// backups lists the backups off the server, oldest first.
func (o Offsite) backups(ctx context.Context) ([]string, error) {
	out, err := o.rclone(ctx, "lsf", "--dirs-only", o.Remote)
	if err != nil {
		// A bucket path or folder not made yet holds no backups.
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

// Fetch brings a backup back from off the server into dir ("" or "latest":
// the newest there), whole, files included; then Restore can put it back. It
// returns the backup's name.
func Fetch(ctx context.Context, o Offsite, dir, name string) (string, error) {
	if !o.On() {
		return "", errors.New("no off-site storage is set up (HP_OFFSITE_*)")
	}
	there, err := o.backups(ctx)
	if err != nil {
		return "", err
	}
	if name == "" || name == "latest" {
		if len(there) == 0 {
			return "", errors.New("there is no backup off the server yet")
		}
		name = there[len(there)-1]
	}
	if !slices.Contains(there, name) {
		return "", fmt.Errorf("no backup %q off the server (there: %s)", name, strings.Join(there, ", "))
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
		return "", fmt.Errorf("%s is already here: restore it as it is", name)
	}
	tmp := filepath.Join(dir, "."+name+".partial")
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
	// The files there are as of the newest backup: with an older database
	// they may include files added since (unused) or lack some deleted since.
	if _, err := o.rclone(ctx, "copy", o.path(filesFolder), filepath.Join(tmp, filesFolder)); err != nil {
		if !strings.Contains(err.Error(), "directory not found") {
			return "", err
		}
	}
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		return "", err
	}
	ok = true
	return name, nil
}
