package explorerleaf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ge-editor/gecore/pathscope"
	"github.com/ge-editor/gecore/tree"
)

const (
	searchMaxHits     = 5000            // matching lines kept
	searchMaxFileSize = 2 << 20         // larger files are not searched
	searchMaxTime     = 5 * time.Second // the search stops here
	searchLineRunes   = 200             // longest line text shown
)

// SearchOptions are the conditions of a search, those of the search form.
type SearchOptions struct {
	// Text is the text searched for. Only its first line is used: the search
	// is made line by line.
	Text      string
	MatchCase bool
	WholeWord bool
	Regex     bool
}

func (o SearchOptions) text() string {
	t, _, _ := strings.Cut(o.Text, "\n")
	return strings.TrimSuffix(t, "\r")
}

// compile builds the regexp: literal text unless Regex, case-insensitive
// unless MatchCase, and bounded by word boundaries when WholeWord.
func (o SearchOptions) compile() (*regexp.Regexp, error) {
	pat := o.text()
	if pat == "" {
		return nil, errors.New("empty search text")
	}
	if !o.Regex {
		pat = regexp.QuoteMeta(pat)
	}
	if o.WholeWord {
		pat = `\b(?:` + pat + `)\b`
	}
	if !o.MatchCase {
		pat = "(?i)" + pat
	}
	return regexp.Compile(pat)
}

// searchHit is one matching line.
type searchHit struct {
	Path string
	Line int // 1-based
	Col  int // 1-based byte column of the first match

	dir     string  // the directory node it is listed under
	name    string  // the file name
	text    string  // the whole line, without the line end
	matches [][]int // submatch indexes (bytes of text) of every match
}

// SearchResult is the result of one search. It is made by the job that
// SearchMode.Prepare returns (on any goroutine) and shown by SetResult.
type SearchResult struct {
	opts SearchOptions
	re   *regexp.Regexp
	hits []searchHit
	cut  bool // the result was cut by a limit
}

// Lines is the number of matching lines.
func (r *SearchResult) Lines() int { return len(r.hits) }

// Files is the number of files with a match.
func (r *SearchResult) Files() int {
	n, last := 0, ""
	for _, h := range r.hits {
		if h.Path != last {
			n++
			last = h.Path
		}
	}
	return n
}

// Cut reports that the search stopped at a limit (lines, time).
func (r *SearchResult) Cut() bool { return r.cut }

// Summary is a short text for the echo area.
func (r *SearchResult) Summary() string {
	if r == nil {
		return ""
	}
	s := fmt.Sprintf("%d lines in %d files", r.Lines(), r.Files())
	if r.cut {
		s += " (cut: too many or too slow)"
	}
	return s
}

type nodeKind int

const (
	nodeDir nodeKind = iota
	nodeFile
	nodeLine
)

// searchNode is the Item.Data of a row of the result tree.
type searchNode struct {
	Kind nodeKind
	Dir  string // the directory node of the row
	Path string // nodeFile, nodeLine
	Line int    // nodeLine; for nodeFile the line of the first match
	Col  int
}

func dirKey(dir string) string   { return "d:" + dir }
func fileKey(path string) string { return "f:" + path }
func lineKey(h searchHit) string { return fmt.Sprintf("l:%s:%d", h.Path, h.Line) }

// SearchMode searches the text of the files under the workspace directories
// and lists the result as a tree:
//
//	- dir
//	  - file name
//	      line number: text
//
// Enter on a directory or file row folds or unfolds it, as in the file mode;
// + and - (Expand and Collapse of the leaf) do it too. Enter on a line opens
// the file at that place.
//
// The search is run by the host (the search form of ge): Prepare makes a job
// to run on a goroutine, and SetResult shows its result on the UI goroutine.
// Only valid UTF-8 text files are searched; binary files, files over 2 MB and
// the directories the file mode hides are skipped. The result is cut at 5000
// lines or 5 seconds.
//
// Replacing is Replacer: the rows are turned into a tree.ReplaceRequest, and
// the editor checks every place again before it changes anything.
type SearchMode struct {
	// roots gives the directories of a PathScope kind; defaultScope is the
	// kind a new pane starts with (the setting of the workspace).
	roots        func(pathscope.Kind) []string
	defaultScope func() pathscope.Kind
	scope        pathscope.Kind
	ignore       func(name string, isDir bool) bool

	list      List
	res       *SearchResult
	collapsed map[string]bool // rows folded by the user, by Key
}

func SearchModeFactory(roots func(pathscope.Kind) []string, defaultScope func() pathscope.Kind) ModeFactory {
	return func() Mode { return NewSearchMode(roots, defaultScope) }
}

// NewSearchMode makes a search mode. defaultScope may be nil (Current).
func NewSearchMode(roots func(pathscope.Kind) []string, defaultScope func() pathscope.Kind) *SearchMode {
	m := &SearchMode{
		roots: roots, defaultScope: defaultScope, ignore: defaultIgnore,
		scope: pathscope.Current, collapsed: map[string]bool{},
	}
	if defaultScope != nil {
		if k := defaultScope(); k.Valid() {
			m.scope = k
		}
	}
	return m
}

var (
	_ Folder   = (*SearchMode)(nil)
	_ Replacer = (*SearchMode)(nil)
)

func (m *SearchMode) Name() string { return "Search" }
func (m *SearchMode) List() *List  { return &m.list }

// Label is the mode name with its scope and result, for the mode line.
func (m *SearchMode) Label() string {
	l := "Search:" + string(m.scope)
	if m.res != nil {
		l += fmt.Sprintf(" %d/%d", m.res.Lines(), m.res.Files())
	}
	return l
}

// SetFilter does nothing: what is typed is the search form's business, and
// filtering would break the tree.
func (m *SearchMode) SetFilter(string) {}

// Reload does nothing: a search can take seconds, so the result is kept when
// the pane is shown again. Search again from the form.
func (m *SearchMode) Reload() error { return nil }

// Scope is the PathScope of this pane's search. It is temporary: a new pane
// starts from the setting of the workspace.
func (m *SearchMode) Scope() pathscope.Kind { return m.scope }

// SetScope changes which directories the next search covers.
func (m *SearchMode) SetScope(k pathscope.Kind) {
	if k.Valid() {
		m.scope = k
	}
}

// Result is the result shown, or nil.
func (m *SearchMode) Result() *SearchResult { return m.res }

// Clear removes the result.
func (m *SearchMode) Clear() {
	m.res = nil
	m.rebuild()
}

// Prepare makes the job that searches for o in the directories of the
// current scope. Call it on the UI goroutine; the job needs no more state of
// the mode, so it can run on another goroutine, and stops when ctx is done
// (or after five seconds). An error is an invalid regular expression or an
// empty text.
func (m *SearchMode) Prepare(o SearchOptions) (job func(ctx context.Context) *SearchResult, err error) {
	re, err := o.compile()
	if err != nil {
		return nil, err
	}
	fm := FileMode{roots: func() []string { return m.roots(m.scope) }}
	roots := fm.rootDirs()
	ignore := m.ignore

	return func(ctx context.Context) *SearchResult {
		ctx, cancel := context.WithTimeout(ctx, searchMaxTime)
		defer cancel()
		hits, cut := searchFiles(ctx, roots, ignore, re)
		for i := range hits {
			hits[i].dir, hits[i].name = splitDisplay(roots, hits[i].Path)
		}
		sort.SliceStable(hits, func(i, j int) bool {
			a, b := hits[i], hits[j]
			if a.dir != b.dir {
				return dirLess(a.dir, b.dir)
			}
			if a.Path != b.Path {
				return a.Path < b.Path
			}
			return a.Line < b.Line
		})
		return &SearchResult{opts: o, re: re, hits: hits, cut: cut}
	}, nil
}

// dirLess puts the top directory first.
func dirLess(a, b string) bool {
	if (a == ".") != (b == ".") {
		return a == "."
	}
	return a < b
}

// splitDisplay returns the directory node and the file name of path. The
// directory is relative to the search directory it is in (with the name of
// that directory in front when there are several).
func splitDisplay(roots []string, path string) (dir, name string) {
	name = filepath.Base(path)
	for _, root := range roots {
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		dir = filepath.ToSlash(rel)
		if len(roots) > 1 {
			if dir == "." {
				dir = filepath.Base(root)
			} else {
				dir = filepath.Base(root) + "/" + dir
			}
		}
		return dir, name
	}
	return filepath.ToSlash(filepath.Dir(path)), name
}

// SetResult shows r (UI goroutine). The folded rows stay folded.
func (m *SearchMode) SetResult(r *SearchResult) {
	key := ""
	if it, ok := m.list.Selected(); ok {
		key = it.Key
	}
	m.res = r
	m.rebuild()
	if key == "" || !m.list.SelectKey(key) {
		m.list.Home()
	}
}

// rebuild makes the rows of the tree from the result and the folded rows.
func (m *SearchMode) rebuild() {
	if m.res == nil {
		m.list.SetItems(nil)
		return
	}
	hits := m.res.hits
	marker := func(key string) string {
		if m.collapsed[key] {
			return "+"
		}
		return "-"
	}

	var items []Item
	for i := 0; i < len(hits); {
		d := hits[i].dir
		j := i
		for j < len(hits) && hits[j].dir == d {
			j++
		}

		dk := dirKey(d)
		head := fmt.Sprintf("%s %s (%d)", marker(dk), d, j-i)
		items = append(items, Item{Key: dk, Text: head, Display: head,
			Data: searchNode{Kind: nodeDir, Dir: d}})

		if !m.collapsed[dk] {
			for k := i; k < j; {
				p := hits[k].Path
				l := k
				for l < j && hits[l].Path == p {
					l++
				}

				fk := fileKey(p)
				fhead := fmt.Sprintf("  %s %s (%d)", marker(fk), hits[k].name, l-k)
				items = append(items, Item{Key: fk, Text: fhead, Display: fhead,
					Data: searchNode{Kind: nodeFile, Dir: d, Path: p, Line: hits[k].Line, Col: hits[k].Col}})

				if !m.collapsed[fk] {
					for _, h := range hits[k:l] {
						disp := fmt.Sprintf("      %d: %s", h.Line, lineText(h.text))
						items = append(items, Item{Key: lineKey(h), Text: disp, Display: disp,
							Data: searchNode{Kind: nodeLine, Dir: d, Path: p, Line: h.Line, Col: h.Col}})
					}
				}
				k = l
			}
		}
		i = j
	}
	m.list.SetItems(items)
}

func lineText(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > searchLineRunes {
		s = string(r[:searchLineRunes]) + "..."
	}
	return s
}

// Activate is Enter on a row: a directory or a file row folds or unfolds
// (as the directories of the file mode do), a line opens the file at the
// match.
func (m *SearchMode) Activate(it Item) Activation {
	n, ok := it.Data.(searchNode)
	if !ok {
		return Activation{}
	}
	if n.Kind == nodeLine {
		return Activation{Open: &Target{Path: n.Path, Line: n.Line, Col: n.Col}}
	}
	m.collapsed[it.Key] = !m.collapsed[it.Key]
	m.rebuild()
	return Activation{}
}

// Fold folds or unfolds a row (see Folder).
func (m *SearchMode) Fold(it Item, fold bool) string {
	n, ok := it.Data.(searchNode)
	if !ok {
		return ""
	}
	switch {
	case n.Kind == nodeLine:
		if !fold {
			return ""
		}
		// A line cannot be folded: fold its file, and go there.
		fk := fileKey(n.Path)
		m.collapsed[fk] = true
		m.rebuild()
		return fk
	case fold && m.collapsed[it.Key]:
		// Folded already: go to the parent.
		if n.Kind == nodeFile {
			return dirKey(n.Dir)
		}
		return ""
	case fold:
		m.collapsed[it.Key] = true
	default:
		delete(m.collapsed, it.Key)
	}
	m.rebuild()
	return it.Key
}

// MoveResult selects the next (dir > 0) or previous (dir < 0) matching line,
// unfolding the rows above it, and goes round at the ends. It returns the
// place as "n/total", or "" when there is no result.
func (m *SearchMode) MoveResult(dir int) string {
	if m.res == nil || len(m.res.hits) == 0 {
		return ""
	}
	hits := m.res.hits

	// cur is the index of the line the cursor is on, or just before or after
	// the first line of the row it is on.
	cur := -1
	if dir < 0 {
		cur = len(hits)
	}
	if it, ok := m.list.Selected(); ok {
		if n, ok := it.Data.(searchNode); ok {
			for i, h := range hits {
				var match bool
				switch n.Kind {
				case nodeLine:
					match = h.Path == n.Path && h.Line == n.Line
				case nodeFile:
					match = h.Path == n.Path
				default:
					match = h.dir == n.Dir
				}
				if match {
					cur = i
					if n.Kind != nodeLine { // before or after the first line
						cur = i - 1
						if dir < 0 {
							cur = i
						}
					}
					break
				}
			}
		}
	}

	next := ((cur+dir)%len(hits) + len(hits)) % len(hits)
	h := hits[next]
	delete(m.collapsed, dirKey(h.dir))
	delete(m.collapsed, fileKey(h.Path))
	m.rebuild()
	m.list.SelectKey(lineKey(h))
	return fmt.Sprintf("%d/%d", next+1, len(hits))
}

// searchFiles looks for re in the files under roots.
func searchFiles(ctx context.Context, roots []string, ignore func(string, bool) bool, re *regexp.Regexp) (hits []searchHit, cut bool) {
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip what cannot be read
			}
			if ctx.Err() != nil {
				cut = true
				return fs.SkipAll
			}
			if p == root {
				return nil
			}
			if ignore != nil && ignore(d.Name(), d.IsDir()) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil // directories (their files come next), links, devices
			}
			found, over := searchFile(p, re, searchMaxHits-len(hits))
			hits = append(hits, found...)
			if over || len(hits) >= searchMaxHits {
				cut = true
				return fs.SkipAll
			}
			return nil
		})
		if cut {
			break
		}
	}
	return hits, cut
}

// searchFile returns the matching lines of one file; over is true when there
// were more than limit.
func searchFile(path string, re *regexp.Regexp, limit int) (hits []searchHit, over bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > searchMaxFileSize {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	head := data
	if len(head) > 8000 {
		head = head[:8000]
	}
	if bytes.IndexByte(head, 0) >= 0 || !utf8.Valid(data) {
		return nil, false // binary, or an encoding other than UTF-8
	}

	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r") // the editor drops it too
		// An empty match (a regexp like "x*") is not a place to replace.
		ms := nonEmpty(re.FindAllStringSubmatchIndex(line, -1))
		if len(ms) == 0 {
			continue
		}
		if len(hits) >= limit {
			return hits, true
		}
		hits = append(hits, searchHit{
			Path: path, Line: i + 1, Col: ms[0][0] + 1,
			text: line, matches: ms,
		})
	}
	return hits, false
}

func nonEmpty(ms [][]int) [][]int {
	out := ms[:0]
	for _, m := range ms {
		if m[1] > m[0] {
			out = append(out, m)
		}
	}
	return out
}

// BuildReplace makes the request that replaces the matches of the rows in
// scope with the given text (see Replacer). With a regular expression the
// text may use $1 and ${name}.
func (m *SearchMode) BuildReplace(with, scope string) (tree.ReplaceRequest, error) {
	if m.res == nil || len(m.res.hits) == 0 {
		return tree.ReplaceRequest{}, errors.New("no search result")
	}

	var keep func(h searchHit) bool
	if scope == "all" {
		keep = func(searchHit) bool { return true }
	} else {
		it, ok := m.list.Selected()
		if !ok {
			return tree.ReplaceRequest{}, errors.New("no row is selected")
		}
		n, ok := it.Data.(searchNode)
		if !ok {
			return tree.ReplaceRequest{}, errors.New("no row is selected")
		}
		switch {
		case scope == "node" && n.Kind == nodeDir:
			keep = func(h searchHit) bool { return h.dir == n.Dir }
		case (scope == "node" || scope == "file") && n.Kind != nodeDir:
			keep = func(h searchHit) bool { return h.Path == n.Path }
			if scope == "node" && n.Kind == nodeLine {
				keep = func(h searchHit) bool { return h.Path == n.Path && h.Line == n.Line }
			}
		case scope == "line" && n.Kind == nodeLine:
			keep = func(h searchHit) bool { return h.Path == n.Path && h.Line == n.Line }
		case scope == "file":
			return tree.ReplaceRequest{}, errors.New("select a file or a line")
		case scope == "line":
			return tree.ReplaceRequest{}, errors.New("select a line")
		default:
			return tree.ReplaceRequest{}, fmt.Errorf("unknown scope %q", scope)
		}
	}

	req := tree.ReplaceRequest{
		Title: fmt.Sprintf("Replace %q with %q: ", m.res.opts.text(), with),
	}
	byPath := map[string]int{}
	for _, h := range m.res.hits {
		if !keep(h) {
			continue
		}
		i, ok := byPath[h.Path]
		if !ok {
			i = len(req.Files)
			byPath[h.Path] = i
			req.Files = append(req.Files, tree.ReplaceFile{Path: h.Path})
		}
		for _, mt := range h.matches {
			repl := with
			if m.res.opts.Regex {
				repl = string(m.res.re.ExpandString(nil, with, h.text, mt))
			}
			req.Files[i].Edits = append(req.Files[i].Edits, tree.ReplaceEdit{
				Line: h.Line, Col: mt[0] + 1,
				Old: h.text[mt[0]:mt[1]], New: repl,
			})
		}
	}
	if len(req.Files) == 0 {
		return tree.ReplaceRequest{}, errors.New("nothing to replace")
	}
	return req, nil
}
