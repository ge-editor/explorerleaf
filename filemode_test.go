package explorerleaf

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Layout:
//
//	root/a.go
//	root/b.txt
//	root/pkg/c.go
//	root/pkg/sub/d.go
//	root/.git/config   (ignored)
func newTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.go"))
	writeFile(t, filepath.Join(root, "b.txt"))
	writeFile(t, filepath.Join(root, "pkg", "c.go"))
	writeFile(t, filepath.Join(root, "pkg", "sub", "d.go"))
	writeFile(t, filepath.Join(root, ".git", "config"))
	return root
}

func texts(l *List) []string {
	rows, _ := l.Visible(l.Len())
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Text
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func newTestMode(root string) *FileMode {
	return NewFileMode(func() []string { return []string{root} })
}

func TestFileModeTree(t *testing.T) {
	root := newTestRoot(t)
	m := newTestMode(root)
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}

	// Root expanded; directories first; .git hidden; pkg collapsed.
	want := []string{root, "pkg", "a.go", "b.txt"}
	if got := texts(m.List()); !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFileModeToggleDirectory(t *testing.T) {
	root := newTestRoot(t)
	m := newTestMode(root)
	_ = m.Reload()

	l := m.List()
	l.Move(1) // pkg
	it, _ := l.Selected()
	if act := m.Activate(it); act.Open != nil {
		t.Fatal("a directory must not be opened")
	}
	want := []string{root, "pkg", "sub", "c.go", "a.go", "b.txt"}
	if got := texts(l); !equal(got, want) {
		t.Fatalf("expanded: got %v, want %v", got, want)
	}
	if it, _ := l.Selected(); it.Text != "pkg" {
		t.Fatalf("the cursor must stay on pkg, got %s", it.Text)
	}

	it, _ = l.Selected()
	m.Activate(it) // collapse
	want = []string{root, "pkg", "a.go", "b.txt"}
	if got := texts(l); !equal(got, want) {
		t.Fatalf("collapsed: got %v, want %v", got, want)
	}
}

func TestFileModeOpenFile(t *testing.T) {
	root := newTestRoot(t)
	m := newTestMode(root)
	_ = m.Reload()

	l := m.List()
	l.End() // b.txt
	it, _ := l.Selected()
	act := m.Activate(it)
	if act.Open == nil || act.Open.Path != filepath.Join(root, "b.txt") {
		t.Fatalf("Activation = %+v", act)
	}
}

func TestFileModeFilterListsWholeWorkspace(t *testing.T) {
	root := newTestRoot(t)
	m := newTestMode(root)
	_ = m.Reload()
	l := m.List()
	l.Move(2) // a.go
	if it, _ := l.Selected(); it.Text != "a.go" {
		t.Fatalf("setup: %s", it.Text)
	}

	// d.go is inside the closed pkg/sub, but the filter finds it.
	m.SetFilter("d.go")
	if got, want := texts(l), []string{"pkg/sub/d.go"}; !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	it, _ := l.Selected()
	act := m.Activate(it)
	if act.Open == nil || act.Open.Path != filepath.Join(root, "pkg", "sub", "d.go") {
		t.Fatalf("Activation = %+v", act)
	}

	// .git is not searched.
	m.SetFilter("config")
	if l.Len() != 0 {
		t.Fatalf("ignored directory leaked: %v", texts(l))
	}

	// Clearing the filter returns to the tree and keeps the state.
	m.SetFilter("")
	want := []string{root, "pkg", "a.go", "b.txt"}
	if got := texts(l); !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFileModeReloadSeesNewFiles(t *testing.T) {
	root := newTestRoot(t)
	m := newTestMode(root)
	_ = m.Reload()

	writeFile(t, filepath.Join(root, "new.go"))
	_ = m.Reload()
	if got := texts(m.List()); !equal(got, []string{root, "pkg", "a.go", "b.txt", "new.go"}) {
		t.Fatalf("got %v", got)
	}

	// The whole-workspace cache is rebuilt too.
	writeFile(t, filepath.Join(root, "newer.go"))
	_ = m.Reload()
	m.SetFilter("newer")
	if m.List().Len() != 1 {
		t.Fatalf("got %v", texts(m.List()))
	}
}

func TestFileModeMultipleRoots(t *testing.T) {
	r1 := newTestRoot(t)
	r2 := t.TempDir()
	writeFile(t, filepath.Join(r2, "z.go"))

	m := NewFileMode(func() []string { return []string{r1, r2, r1} }) // r1 twice
	_ = m.Reload()
	want := []string{r1, "pkg", "a.go", "b.txt", r2, "z.go"}
	if got := texts(m.List()); !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	// With several roots a filtered path starts with the root's name.
	m.SetFilter("z.go")
	if got, want := texts(m.List()), []string{filepath.Base(r2) + "/z.go"}; !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFileModeNoRootsFallsBackToCwd(t *testing.T) {
	root := newTestRoot(t)
	t.Chdir(root)

	m := NewFileMode(nil)
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	if m.List().Len() == 0 {
		t.Fatal("expected the current directory to be listed")
	}
}

func TestDisplayDir(t *testing.T) {
	sep := string(filepath.Separator)
	home := sep + "home" + sep + "taro"
	for in, want := range map[string]string{
		home:                         "~",
		home + sep + "proj":          "~" + sep + "proj",
		home + sep + "a" + sep + "b": "~" + sep + "a" + sep + "b",
		home + "x":                   home + "x", // another directory with the same prefix
		sep + "usr" + sep + "local":  sep + "usr" + sep + "local",
	} {
		if got := displayDir(in, home); got != want {
			t.Errorf("displayDir(%q) = %q, want %q", in, got, want)
		}
	}
	if got := displayDir(home, ""); got != home {
		t.Errorf("no home: %q", got)
	}
}

// A root under the home directory is shown with "~"; its item keeps the real
// path, so opening and filtering are not affected.
func TestFileModeShowsHomeAsTilde(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "proj")
	writeFile(t, filepath.Join(root, "a.go"))

	old := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = old }()

	m := newTestMode(root)
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	rows, _ := m.List().Visible(m.List().Len())
	if len(rows) == 0 {
		t.Fatal("no rows")
	}
	first := rows[0]
	if want := "- ~" + string(filepath.Separator) + "proj/"; first.display() != want {
		t.Errorf("display = %q, want %q", first.display(), want)
	}
	if first.Text != root || first.Key != root {
		t.Errorf("the item changed: text %q key %q", first.Text, first.Key)
	}
}
