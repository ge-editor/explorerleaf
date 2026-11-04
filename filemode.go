package explorerleaf

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxFlatEntries bounds the whole-workspace file list used while filtering.
const maxFlatEntries = 50000

// fileEntry is the Item.Data of the file mode.
type fileEntry struct {
	Path  string
	IsDir bool
}

// FileMode lists the files under the workspace directories.
//
// Without a filter it is a tree: the roots are shown expanded and every
// directory opens or closes with Enter. With a filter it lists every file
// under the roots (relative paths), so a file in a closed directory can be
// found by typing part of its path.
type FileMode struct {
	roots  func() []string
	ignore func(name string, isDir bool) bool

	list     List
	filter   string
	expanded map[string]bool
	seeded   map[string]bool // roots whose initial expansion was applied
	flat     []Item          // cache of the whole-workspace list, nil = not built
}

// FileModeFactory returns a ModeFactory. roots supplies the directories to list
// (the current workspace's Directories); when it is nil or returns nothing,
// the current directory is listed.
func FileModeFactory(roots func() []string) ModeFactory {
	return func() Mode { return NewFileMode(roots) }
}

func NewFileMode(roots func() []string) *FileMode {
	return &FileMode{
		roots:    roots,
		ignore:   defaultIgnore,
		expanded: map[string]bool{},
		seeded:   map[string]bool{},
	}
}

// defaultIgnore hides version-control and dependency directories.
// (To be replaced by the workspace PathFilter.)
func defaultIgnore(name string, isDir bool) bool {
	if !isDir {
		return false
	}
	switch name {
	case ".git", ".hg", ".svn", "node_modules":
		return true
	}
	return false
}

func (m *FileMode) Name() string { return "Files" }
func (m *FileMode) List() *List  { return &m.list }

// Reload rereads the directories.
func (m *FileMode) Reload() error {
	m.flat = nil
	if len(m.rootDirs()) == 0 {
		m.list.SetItems(nil)
		return errors.New("explorer: no directory to list")
	}
	m.refill()
	return nil
}

func (m *FileMode) SetFilter(s string) {
	wasFlat := m.filter != ""
	m.filter = s
	if wasFlat != (s != "") {
		m.refill()
	}
	m.list.SetFilter(s)
}

func (m *FileMode) Activate(it Item) Activation {
	fe, ok := it.Data.(fileEntry)
	if !ok {
		return Activation{}
	}
	if fe.IsDir {
		m.expanded[fe.Path] = !m.expanded[fe.Path]
		m.refill()
		return Activation{}
	}
	return Activation{Open: &Target{Path: fe.Path}}
}

// refill gives the list the item set that matches the current filter state.
func (m *FileMode) refill() {
	if m.filter != "" {
		if m.flat == nil {
			m.flat = m.flatItems()
		}
		m.list.SetItems(m.flat)
		return
	}
	m.list.SetItems(m.treeItems())
}

// rootDirs returns the absolute, cleaned, de-duplicated roots.
func (m *FileMode) rootDirs() []string {
	var dirs []string
	if m.roots != nil {
		dirs = m.roots()
	}
	if len(dirs) == 0 {
		if wd, err := os.Getwd(); err == nil {
			dirs = []string{wd}
		}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if abs, err := filepath.Abs(d); err == nil {
			d = abs
		}
		d = filepath.Clean(d)
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

func (m *FileMode) treeItems() []Item {
	var items []Item
	for _, root := range m.rootDirs() {
		if !m.seeded[root] {
			m.seeded[root] = true
			m.expanded[root] = true
		}
		items = append(items, m.dirItem(root, root, 0))
		if m.expanded[root] {
			items = m.appendDir(items, root, 1)
		}
	}
	return items
}

func (m *FileMode) dirItem(path, label string, depth int) Item {
	marker := "+ "
	if m.expanded[path] {
		marker = "- "
	}
	return Item{
		Key:     path,
		Text:    label,
		Display: strings.Repeat("  ", depth) + marker + label + "/",
		Data:    fileEntry{Path: path, IsDir: true},
	}
}

func (m *FileMode) appendDir(items []Item, dir string, depth int) []Item {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return items // unreadable: shown as an empty directory
	}

	type child struct {
		name  string
		path  string
		isDir bool
	}
	children := make([]child, 0, len(entries))
	for _, e := range entries {
		full := filepath.Join(dir, e.Name())
		isDir := entryIsDir(full, e)
		if m.ignore != nil && m.ignore(e.Name(), isDir) {
			continue
		}
		children = append(children, child{e.Name(), full, isDir})
	}
	// Directories first, then by name.
	sort.SliceStable(children, func(i, j int) bool {
		if children[i].isDir != children[j].isDir {
			return children[i].isDir
		}
		return children[i].name < children[j].name
	})

	for _, c := range children {
		if c.isDir {
			items = append(items, m.dirItem(c.path, c.name, depth))
			if m.expanded[c.path] {
				items = m.appendDir(items, c.path, depth+1)
			}
			continue
		}
		items = append(items, Item{
			Key:     c.path,
			Text:    c.name,
			Display: strings.Repeat("  ", depth) + "  " + c.name,
			Data:    fileEntry{Path: c.path},
		})
	}
	return items
}

func (m *FileMode) flatItems() []Item {
	var items []Item
	roots := m.rootDirs()
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip what cannot be read
			}
			if p == root {
				return nil
			}
			isDir := entryIsDir(p, d)
			if m.ignore != nil && m.ignore(d.Name(), isDir) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if isDir {
				return nil
			}
			if len(items) >= maxFlatEntries {
				return fs.SkipAll
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return nil
			}
			text := filepath.ToSlash(rel)
			if len(roots) > 1 {
				text = filepath.Base(root) + "/" + text
			}
			items = append(items, Item{
				Key:  p,
				Text: text,
				Data: fileEntry{Path: p},
			})
			return nil
		})
	}
	return items
}

// entryIsDir reports whether e is a directory, following symbolic links.
func entryIsDir(full string, e fs.DirEntry) bool {
	if e.IsDir() {
		return true
	}
	if e.Type()&fs.ModeSymlink != 0 {
		if fi, err := os.Stat(full); err == nil {
			return fi.IsDir()
		}
	}
	return false
}
