package main

// hp repair-files [-apply]   check (and with -apply, fix) who owns the uploaded files

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
)

// sampleFiles is how many restored files are opened as their owner to check.
const sampleFiles = 20

// verifyRestoredFiles checks, after a restore, that the user the API runs as
// owns the files put back and can open them (a sample). A failure exits
// non-zero, so a script restoring a server stops there.
func verifyRestoredFiles(ctx context.Context, o backup.Options) error {
	if o.Owner == nil {
		fmt.Println("files: HP_FILES_OWNER is not set, so the restored files are this user's own")
		return nil
	}
	check, err := backup.CheckFiles(ctx, o.FilesDir, *o.Owner, sampleFiles)
	if err != nil {
		return fmt.Errorf("checking the restored files: %w", err)
	}
	fmt.Println("files:", check.Summary())
	if !check.OK() {
		return fmt.Errorf("the database and files are restored, but %s cannot use every file (%s): run `hp repair-files -apply`%s",
			o.Owner, check.Summary(), unreadableList(check))
	}
	return nil
}

// runRepairFiles checks who owns the uploaded files, and with -apply gives
// them to HP_FILES_OWNER, private to that user: for a server restored before
// restores did this (CV previews failing with "permission denied").
func runRepairFiles(ctx context.Context, args []string) error {
	c := command{name: "repair-files", synopsis: "[-apply]"}
	fs := c.flags()
	apply := fs.Bool("apply", false, "change the owners and permissions (without it, only report)")
	if _, err := c.parse(fs, args); err != nil {
		return err
	}
	dir := os.Getenv("HP_FILES_DIR")
	if dir == "" {
		return errors.New("HP_FILES_DIR is required (the uploaded files)")
	}
	owner, err := filesOwner()
	if err != nil {
		return err
	}
	if owner == nil {
		return errors.New("set HP_FILES_OWNER (or HP_BACKUP_OWNER) to the user and group the API runs as, like 65532:65532")
	}
	before, err := backup.CheckFiles(ctx, dir, *owner, sampleFiles)
	if err != nil {
		return err
	}
	fmt.Println("now:", before.Summary())
	if !*apply {
		if before.OK() {
			fmt.Println("nothing to repair")
			return nil
		}
		fmt.Println("nothing changed: run `hp repair-files -apply` to give the files to", owner.String())
		return nil
	}
	skipped, err := backup.RepairFiles(dir, *owner)
	if err != nil {
		return fmt.Errorf("repairing the files: %w", err)
	}
	after, err := backup.CheckFiles(ctx, dir, *owner, sampleFiles)
	if err != nil {
		return err
	}
	fmt.Println("after:", after.Summary())
	if skipped > 0 {
		fmt.Printf("%d files have another link (a kept backup's copy?) and were left as they are\n", skipped)
	}
	if !after.OK() {
		return fmt.Errorf("some files still cannot be used by %s%s", owner, unreadableList(after))
	}
	fmt.Println(remakePreviews)
	return nil
}

// remakePreviews says what repairing the files does not do: PDF copies that
// failed while the files could not be opened stay failed until asked for again.
const remakePreviews = "PDF copies of Word CVs that failed while the files could not be opened stay failed: " +
	"on each such CV (\"No PDF copy\"), choose ⋯ > Make the PDF again. A CV whose text could not be read: Read the CV again."


func unreadableList(c backup.FilesCheck) string {
	if len(c.Unreadable) == 0 {
		return ""
	}
	return "; could not open: " + strings.Join(c.Unreadable, ", ")
}
