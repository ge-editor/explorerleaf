package explorerleaf

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ge-editor/gecore/pathscope"
)

func writeText(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newSearchRoot(t *testing.T) (string, *SearchMode) {
	t.Helper()
	root := t.TempDir()
	writeText(t, filepath.Join(root, "a.go"), "package a\nfunc Foo() {}\n// foo and FOO\n")
	writeText(t, filepath.Join(root, "pkg", "b.txt"), "no match\r\nfoo bar foo\r\n")
	writeText(t, filepath.Join(root, ".git", "config"), "foo\n") // ignored directory
	writeText(t, filepath.Join(root, "bin.dat"), "foo\x00foo")   // binary
	writeText(t, filepath.Join(root, "sjis.txt"), "foo \x82\xa0") // not UTF-8
	m := NewSearchMode(func(pathscope.Kind) []string { return []string{root} }, nil)
	return root, m
}

// search runs a search and shows its result, as the search form does.
func search(t *testing.T, m *SearchMode, o SearchOptions) {
	t.Helper()
	job, err := m.Prepare(o)
	if err != nil {
		t.Fatal(err)
	}
	m.SetResult(job(context.Background()))
}

func TestSearchOptions(t *testing.T) {
	_, m := newSearchRoot(t)

	search(t, m, SearchOptions{Text: "foo"}) // not Match Case
	if got := m.Result().Lines(); got != 3 {
		t.Fatalf("lines = %d, want 3", got)
	}
	search(t, m, SearchOptions{Text: "Foo", MatchCase: true})
	if got := m.Result().Lines(); got != 1 {
		t.Fatalf("Match Case: lines = %d, want 1", got)
	}
	search(t, m, SearchOptions{Text: "fo", WholeWord: true})
	if got := m.Result().Lines(); got != 0 {
		t.Fatalf("Whole word fo: lines = %d, want 0", got)
	}
	search(t, m, SearchOptions{Text: "foo", WholeWord: true})
	if got := m.Result().Lines(); got != 3 {
		t.Fatalf("Whole word foo: lines = %d, want 3", got)
	}
	search(t, m, SearchOptions{Text: `fo+ (bar|baz)`, Regex: true})
	if got := m.Result().Lines(); got != 1 {
		t.Fatalf("Regex: lines = %d, want 1", got)
	}
	// The text is literal without Regex.
	search(t, m, SearchOptions{Text: `fo+`})
	if got := m.Result().Lines(); got != 0 {
		t.Fatalf("literal: lines = %d, want 0", got)
	}

	if _, err := m.Prepare(SearchOptions{Text: "(", Regex: true}); err == nil {
		t.Fatal("invalid regexp must be an error")
	}
	if _, err := m.Prepare(SearchOptions{}); err == nil {
		t.Fatal("empty text must be an error")
	}
}

func TestSearchTree(t *testing.T) {
	_, m := newSearchRoot(t)
	search(t, m, SearchOptions{Text: "foo"})

	want := []string{
		"- . (2)",
		"  - a.go (2)",
		"      2: func Foo() {}",
		"      3: // foo and FOO",
		"- pkg (1)",
		"  - b.txt (1)",
		"      2: foo bar foo",
	}
	if got := displays(m.List()); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %q\nwant   %q", got, want)
	}
}

func displays(l *List) []string {
	rows, _ := l.Visible(l.Len())
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Display
	}
	return out
}

func TestSearchFold(t *testing.T) {
	_, m := newSearchRoot(t)
	search(t, m, SearchOptions{Text: "foo"})

	// - on a line folds its file and goes there.
	m.List().SelectKey("l:" + m.res.hits[0].Path + ":2")
	it, _ := m.List().Selected()
	key := m.Fold(it, true)
	if key != fileKey(m.res.hits[0].Path) {
		t.Fatalf("key = %q", key)
	}
	if got := len(displays(m.List())); got != 5 {
		t.Fatalf("rows = %d, want 5", got)
	}

	// - on a folded file goes to its directory; + unfolds.
	m.List().SelectKey(key)
	it, _ = m.List().Selected()
	if got := m.Fold(it, true); got != dirKey(".") {
		t.Fatalf("parent = %q", got)
	}
	m.Fold(it, false)
	if got := len(displays(m.List())); got != 7 {
		t.Fatalf("rows = %d, want 7", got)
	}

	// Fold a directory: its files are gone from the list.
	m.List().SelectKey(dirKey("pkg"))
	it, _ = m.List().Selected()
	m.Fold(it, true)
	if got := displays(m.List()); len(got) != 5 || got[4] != "+ pkg (1)" {
		t.Fatalf("rows = %q", got)
	}

	// Enter on a directory toggles it.
	it, _ = m.List().Selected()
	if act := m.Activate(it); act.Open != nil {
		t.Fatal("a directory must not open")
	}
	if got := len(displays(m.List())); got != 7 {
		t.Fatalf("rows = %d, want 7", got)
	}
}

func TestSearchActivate(t *testing.T) {
	root, m := newSearchRoot(t)
	search(t, m, SearchOptions{Text: "bar"})
	path := filepath.Join(root, "pkg", "b.txt")

	// A line opens the file at the match.
	m.List().SelectKey("l:" + path + ":2")
	it, _ := m.List().Selected()
	act := m.Activate(it)
	if act.Open == nil || act.Open.Path != path || act.Open.Line != 2 || act.Open.Col != 5 {
		t.Fatalf("Open = %+v, want %s:2:5", act.Open, path)
	}

	// A file row folds and unfolds (the line row is gone while it is folded).
	m.List().SelectKey(fileKey(path))
	it, _ = m.List().Selected()
	if act := m.Activate(it); act.Open != nil {
		t.Fatal("a file row must not open")
	}
	if got := len(displays(m.List())); got != 2 {
		t.Fatalf("rows = %d, want 2", got)
	}
	m.Activate(it)
	if got := len(displays(m.List())); got != 3 {
		t.Fatalf("rows = %d, want 3", got)
	}
}

func TestSearchMoveResult(t *testing.T) {
	_, m := newSearchRoot(t)
	search(t, m, SearchOptions{Text: "foo"})

	var got []string
	for i := 0; i < 4; i++ {
		got = append(got, m.MoveResult(1))
	}
	if !reflect.DeepEqual(got, []string{"1/3", "2/3", "3/3", "1/3"}) {
		t.Fatalf("forward = %v", got)
	}

	// Backward from a directory row goes round to the last line.
	m.List().Home()
	if got := m.MoveResult(-1); got != "3/3" {
		t.Fatalf("backward = %q", got)
	}
	if it, _ := m.List().Selected(); it.Display != "      2: foo bar foo" {
		t.Fatalf("selected = %q", it.Display)
	}

	// A folded file is unfolded to show the line.
	m.List().SelectKey(fileKey(m.res.hits[0].Path))
	it, _ := m.List().Selected()
	m.Fold(it, true)
	m.List().Home()
	m.MoveResult(1)
	if it, _ := m.List().Selected(); it.Display != "      2: func Foo() {}" {
		t.Fatalf("selected = %q", it.Display)
	}
}

func TestBuildReplaceScopes(t *testing.T) {
	root, m := newSearchRoot(t)
	search(t, m, SearchOptions{Text: "foo"})
	count := func(scope string) (files, edits int) {
		req, err := m.BuildReplace("X", scope)
		if err != nil {
			t.Fatalf("%s: %v", scope, err)
		}
		for _, f := range req.Files {
			files++
			edits += len(f.Edits)
		}
		return
	}

	if f, e := count("all"); f != 2 || e != 5 { // a.go: 1+2, b.txt: 2
		t.Fatalf("all: %d files %d edits", f, e)
	}

	m.List().SelectKey(dirKey("pkg"))
	if f, e := count("node"); f != 1 || e != 2 {
		t.Fatalf("dir: %d files %d edits", f, e)
	}
	m.List().SelectKey(fileKey(filepath.Join(root, "a.go")))
	if f, e := count("node"); f != 1 || e != 3 {
		t.Fatalf("file: %d files %d edits", f, e)
	}
	m.List().SelectKey("l:" + filepath.Join(root, "a.go") + ":3")
	if f, e := count("node"); f != 1 || e != 2 {
		t.Fatalf("line: %d files %d edits", f, e)
	}
	if f, e := count("line"); f != 1 || e != 2 {
		t.Fatalf("line scope: %d files %d edits", f, e)
	}

	// "line" needs a line; "file" needs a file or a line.
	m.List().SelectKey(dirKey("."))
	if _, err := m.BuildReplace("X", "line"); err == nil {
		t.Fatal("line scope on a directory must fail")
	}
	if _, err := m.BuildReplace("X", "file"); err == nil {
		t.Fatal("file scope on a directory must fail")
	}
}

func TestBuildReplaceEdit(t *testing.T) {
	root, m := newSearchRoot(t)
	search(t, m, SearchOptions{Text: "foo bar foo"})

	req, err := m.BuildReplace("X", "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Files) != 1 || req.Files[0].Path != filepath.Join(root, "pkg", "b.txt") {
		t.Fatalf("files = %+v", req.Files)
	}
	ed := req.Files[0].Edits
	if len(ed) != 1 || ed[0].Line != 2 || ed[0].Col != 1 || ed[0].Old != "foo bar foo" || ed[0].New != "X" {
		t.Fatalf("edits = %+v", ed)
	}
}

func TestBuildReplaceTwoMatchesInALine(t *testing.T) {
	root, m := newSearchRoot(t)
	search(t, m, SearchOptions{Text: "foo"})
	m.List().SelectKey("l:" + filepath.Join(root, "pkg", "b.txt") + ":2")

	req, err := m.BuildReplace("Z", "line")
	if err != nil {
		t.Fatal(err)
	}
	ed := req.Files[0].Edits
	if len(ed) != 2 || ed[0].Col != 1 || ed[1].Col != 9 {
		t.Fatalf("edits = %+v", ed)
	}
}

func TestBuildReplaceRegexpGroups(t *testing.T) {
	_, m := newSearchRoot(t)
	search(t, m, SearchOptions{Text: `(foo) (bar)`, Regex: true})

	req, err := m.BuildReplace("$2 $1", "all")
	if err != nil {
		t.Fatal(err)
	}
	if got := req.Files[0].Edits[0].New; got != "bar foo" {
		t.Fatalf("New = %q, want %q", got, "bar foo")
	}
}

func TestBuildReplaceNeedsResult(t *testing.T) {
	_, m := newSearchRoot(t)
	if _, err := m.BuildReplace("x", "all"); err == nil {
		t.Fatal("replace without a result must fail")
	}
}

func TestSearchScopeRoots(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	writeText(t, filepath.Join(a, "x.txt"), "needle\n")
	writeText(t, filepath.Join(b, "sub", "y.txt"), "needle\n")
	roots := func(k pathscope.Kind) []string {
		if k == pathscope.Current {
			return []string{a}
		}
		return []string{a, b}
	}
	m := NewSearchMode(roots, func() pathscope.Kind { return pathscope.Current })
	search(t, m, SearchOptions{Text: "needle"})
	if got := m.Result().Lines(); got != 1 {
		t.Fatalf("current: lines = %d", got)
	}

	m.SetScope(pathscope.Workspaces)
	search(t, m, SearchOptions{Text: "needle"})
	if got := m.Result().Lines(); got != 2 {
		t.Fatalf("workspaces: lines = %d", got)
	}
	// With several directories the nodes carry the directory name.
	rows := displays(m.List())
	if rows[0] != "- "+filepath.Base(a)+" (1)" || rows[3] != "- "+filepath.Base(b)+"/sub (1)" {
		t.Fatalf("rows = %q", rows)
	}
}
