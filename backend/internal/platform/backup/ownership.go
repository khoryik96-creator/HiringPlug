package backup

// Who the uploaded files belong to. In Docker the backup service runs as
// root and the API and workers as another user (65532): files a restore
// writes as root would be root's, and every CV preview would fail with
// "permission denied". So a restore gives the files it puts back to the
// runtime user (HP_FILES_OWNER, else HP_BACKUP_OWNER) before they replace
// the current ones, keeps them private to that user, and checks afterwards
// that the user can open them. RepairFiles does the same for files restored
// before this (hp repair-files).

import (
	"context"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Owner is a user and group id, such as 65532:65532.
type Owner struct{ UID, GID int }

func (o Owner) String() string { return strconv.Itoa(o.UID) + ":" + strconv.Itoa(o.GID) }

// ParseOwner reads "uid:gid" (HP_FILES_OWNER, HP_BACKUP_OWNER).
func ParseOwner(v string) (Owner, error) {
	u, g, ok := strings.Cut(strings.TrimSpace(v), ":")
	uid, uerr := strconv.Atoi(u)
	gid, gerr := strconv.Atoi(g)
	if !ok || uerr != nil || gerr != nil || uid < 0 || gid < 0 {
		return Owner{}, fmt.Errorf("must be a user and group id like 65532:65532, got %q", v)
	}
	return Owner{UID: uid, GID: gid}, nil
}

// stageName is the restore's staging folder inside the files folder.
const stageName = ".hp-restore"

// node is a folder or regular file met by walk, open: changes act on it
// (fchown, fchmod), never on its name, so a name swapped for a link while
// the walk runs (the API still running, writing) is never followed. rel is
// its path inside the walked folder ("" for the folder itself).
type node struct {
	fd       int
	rel      string
	dir      bool
	uid, gid int
	perm     fs.FileMode
	nlink    uint64
}

// chown gives one open folder or file to o; tests make it fail.
var chown = func(n node, o Owner) error { return n.chown(o) }

// giveFiles gives root, its folders and their regular files to o. Links are
// never followed nor changed (walk). With skipLinked, a file with more
// than one link (shared with another folder, such as a kept backup's copy)
// is left alone, so changing it here never changes the other copy; skipped
// counts them.
func giveFiles(root string, o Owner, skipLinked bool) (skipped int, err error) {
	err = walk(root, func(n node) error {
		if skipLinked && !n.dir && n.nlink > 1 {
			skipped++
			return nil
		}
		return chown(n, o)
	})
	return skipped, err
}

// lchown is os.Lchown (it never follows a link); tests make it fail.
var lchown = os.Lchown

// FilesCheck is what CheckFiles found in the files folder (the folder
// itself counts as one of its folders, but its mode is its mount point's).
type FilesCheck struct {
	Owner      Owner
	Folders    int
	Files      int
	WrongOwner int // folders and files that are not the owner's
	TooOpen    int // folders and files other users may read, write or enter
	Linked     int // files with another link (left alone by RepairFiles)
	Sampled    int // files opened as the owner
	// Unreadable are sampled files the owner could not open (paths inside
	// the folder; never their contents).
	Unreadable []string
	// NotSampled says why no file was opened as the owner (empty when some were).
	NotSampled string
}

// OK reports whether every folder and file is the owner's, private, and the
// sampled files open.
func (c FilesCheck) OK() bool {
	return c.WrongOwner == 0 && c.TooOpen == 0 && len(c.Unreadable) == 0
}

// Summary is the check in one line, for the restore's and repair's output.
func (c FilesCheck) Summary() string {
	s := fmt.Sprintf("%d folders and %d files; %d not owned by %s, %d open to other users", c.Folders, c.Files, c.WrongOwner, c.Owner, c.TooOpen)
	switch {
	case c.NotSampled != "":
		s += "; not opened as " + c.Owner.String() + ": " + c.NotSampled
	case c.Sampled > 0:
		s += fmt.Sprintf("; %d of %d sampled files opened as %s", c.Sampled-len(c.Unreadable), c.Sampled, c.Owner)
	}
	if c.Linked > 0 {
		s += fmt.Sprintf("; %d files linked elsewhere (left alone)", c.Linked)
	}
	return s
}

// CheckFiles counts the folders and files under dir that are not o's or that
// other users may open, and opens up to sample files (picked at random) as o:
// only root can act as another user, so when this process is neither root
// nor o it says so instead.
func CheckFiles(ctx context.Context, dir string, o Owner, sample int) (FilesCheck, error) {
	c := FilesCheck{Owner: o}
	var picked []string
	seen := 0
	err := walk(dir, func(n node) error {
		if n.dir {
			c.Folders++
		} else {
			c.Files++
			if n.nlink > 1 {
				c.Linked++
			}
			// Reservoir sampling: every file equally likely, in one pass.
			seen++
			p := filepath.Join(dir, n.rel)
			if len(picked) < sample {
				picked = append(picked, p)
			} else if i := rand.IntN(seen); i < sample { //nolint:gosec // which files to check, not a secret
				picked[i] = p
			}
		}
		if n.uid != o.UID || n.gid != o.GID {
			c.WrongOwner++
		}
		if n.rel != "" && n.perm&0o007 != 0 { // the folder itself may be a mount point, as its image made it
			c.TooOpen++
		}
		return nil
	})
	if err != nil {
		return c, err
	}
	switch euid := os.Geteuid(); {
	case len(picked) == 0:
	case euid != o.UID && euid != 0:
		c.NotSampled = "only root or that user can open files as that user"
	default:
		for _, p := range picked {
			c.Sampled++
			if err := openAs(ctx, p, o); err != nil {
				rel, _ := filepath.Rel(dir, p)
				c.Unreadable = append(c.Unreadable, rel)
			}
		}
	}
	return c, nil
}

// RepairFiles gives every folder and file under dir (dir too) to o and takes
// away other users' access, never making anything more open, and never
// changing a file with another link (it would change that copy too). It
// returns how many files were left alone for that reason. Links are never
// followed (walk), but stop the API and workers first: a file they write
// meanwhile may be left as they made it.
func RepairFiles(dir string, o Owner) (skipped int, err error) {
	err = walk(dir, func(n node) error {
		if !n.dir && n.nlink > 1 {
			skipped++
			return nil
		}
		if err := chown(n, o); err != nil {
			return err
		}
		if n.rel != "" && n.perm&0o007 != 0 {
			return n.chmod(n.perm &^ 0o007) // only ever narrower
		}
		return nil
	})
	return skipped, err
}
