package explorerleaf

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v3"
)

// Git status marks of the file list, as VS Code shows them:
//
//	M modified   A added (staged)   D deleted   R renamed
//	U untracked  ! conflict
//
// A directory shows "*" in the color of the strongest status below it.

const gitCommandLimit = 3 * time.Second

var (
	gitColorModified  = tcell.NewRGBColor(224, 168, 56)
	gitColorAdded     = tcell.NewRGBColor(88, 166, 88)
	gitColorUntracked = tcell.NewRGBColor(115, 201, 145)
	gitColorDeleted   = tcell.NewRGBColor(214, 80, 80)
	gitColorRenamed   = tcell.NewRGBColor(80, 170, 190)
	gitColorConflict  = tcell.NewRGBColor(230, 60, 60)
)

// gitPriority orders the statuses for the mark of a directory. Bigger wins.
func gitPriority(c byte) int {
	switch c {
	case '!':
		return 6
	case 'M':
		return 5
	case 'D':
		return 4
	case 'A':
		return 3
	case 'R':
		return 2
	case 'U':
		return 1
	}
	return 0
}

func gitColor(c byte) tcell.Color {
	switch c {
	case 'M':
		return gitColorModified
	case 'A':
		return gitColorAdded
	case 'U':
		return gitColorUntracked
	case 'D':
		return gitColorDeleted
	case 'R':
		return gitColorRenamed
	case '!':
		return gitColorConflict
	}
	return tcell.ColorDefault
}

// gitRootStatus is the status of the files under one listed directory.
// Keys are slash separated paths relative to that directory.
type gitRootStatus struct {
	root  string
	files map[string]byte
	dirs  map[string]byte
}

// gitStatuses is the status of all listed directories. A nil value means no
// marks.
type gitStatuses []*gitRootStatus

// of returns the status mark of path, or 0.
func (g gitStatuses) of(path string, isDir bool) byte {
	for _, r := range g {
		rel, err := filepath.Rel(r.root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			rel = ""
		}
		if isDir {
			return r.dirs[rel]
		}
		return r.files[rel]
	}
	return 0
}

// mark fills the badge of an item.
func (g gitStatuses) mark(it *Item, path string, isDir bool) {
	c := g.of(path, isDir)
	if c == 0 {
		return
	}
	it.Badge = string(c)
	if isDir {
		it.Badge = "*"
	}
	it.BadgeColor = gitColor(c)
}

// gitRun runs git in dir and returns its standard output.
func gitRun(dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitCommandLimit)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0") // do not touch the index
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// loadGitStatuses reads the status of every root. A root that is not in a
// repository, or an environment without git, gives no marks for it.
func loadGitStatuses(roots []string) gitStatuses {
	var all gitStatuses
	for _, root := range roots {
		if r := loadGitRootStatus(root); r != nil {
			all = append(all, r)
		}
	}
	return all
}

func loadGitRootStatus(root string) *gitRootStatus {
	top, err := gitRun(root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil
	}
	topDir := strings.TrimRight(string(top), "\r\n")
	out, err := gitRun(topDir, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil
	}

	// Paths of git are relative to the top of the repository, and the real
	// path of it; the listed directory may be reached through a symbolic link.
	realRoot := root
	if p, err := filepath.EvalSymlinks(root); err == nil {
		realRoot = p
	}
	return parseGitStatus(out, topDir, realRoot, root)
}

// parseGitStatus reads the output of `git status --porcelain=v1 -z`.
// topDir is the top of the repository, realRoot the listed directory without
// symbolic links, and root the listed directory as it is shown.
func parseGitStatus(out []byte, topDir, realRoot, root string) *gitRootStatus {
	st := &gitRootStatus{root: root, files: map[string]byte{}, dirs: map[string]byte{}}

	fields := strings.Split(string(out), "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 4 {
			continue
		}
		x, y, path := f[0], f[1], f[3:]
		if x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			i++ // the next field is the old name
		}
		c := gitStatusLetter(x, y)
		if c == 0 {
			continue
		}

		rel, err := filepath.Rel(realRoot, filepath.Join(topDir, filepath.FromSlash(path)))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue // outside the listed directory
		}
		rel = filepath.ToSlash(rel)
		st.files[rel] = c
		for d := parentSlash(rel); ; d = parentSlash(d) {
			if gitPriority(c) > gitPriority(st.dirs[d]) {
				st.dirs[d] = c
			}
			if d == "" {
				break
			}
		}
	}
	return st
}

// parentSlash returns the directory part of a slash separated path ("" for
// the top).
func parentSlash(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return ""
}

// gitStatusLetter turns the two columns of the porcelain format into the one
// letter shown (0: nothing to show).
func gitStatusLetter(x, y byte) byte {
	switch {
	case x == '?' && y == '?':
		return 'U'
	case x == '!' && y == '!':
		return 0 // ignored
	case x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D'):
		return '!'
	case x == 'A' || x == 'C':
		return 'A'
	case x == 'R' || y == 'R':
		return 'R'
	case x == 'D' || y == 'D':
		return 'D'
	case x == 'M' || y == 'M' || x == 'T' || y == 'T':
		return 'M'
	}
	return 0
}
