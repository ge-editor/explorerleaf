package explorerleaf

import "github.com/ge-editor/gecore/tree"

// Target is a place a mode asks the host to open. It is the shared
// tree.OpenRequest (Path, Line, Col), so a mode that knows a position (Mark,
// Find) just fills Line and Col in.
type Target = tree.OpenRequest

// Activation is what a mode returns for Enter on a row.
type Activation struct {
	// Open, when set, is opened through tree.OpenFile.
	Open *Target
	// Message, when set, is shown in the echo area.
	Message string
}

// Mode is one view of the explorer (files, functions, marks, ...).
//
// Every ExplorerLeaf owns its own Mode values, and a Mode keeps its own
// List, so cursor, scroll position and filter are restored when the user
// comes back to a mode. What a mode lists, how its items are shaped and what
// Enter does are the mode's business; the leaf only provides the shared list
// operations and drawing.
type Mode interface {
	Name() string
	List() *List

	// Reload rebuilds the items from the data source.
	Reload() error

	// SetFilter changes the filter text. A mode may switch its item set
	// here (the file mode lists the whole workspace while filtering).
	SetFilter(string)

	// Activate handles Enter on the selected row.
	Activate(Item) Activation
}

// ModeFactory creates a fresh Mode. Each ExplorerLeaf calls the factories
// of its LeafType once, so panes never share mode state.
type ModeFactory func() Mode

// Folder is a Mode whose rows form a tree that can be folded (the + and -
// keys).
type Folder interface {
	// Fold folds (true) or unfolds (false) the row it. It returns the Key of
	// the row the cursor should be on afterwards ("" keeps it). Folding a
	// row that is folded already, or a leaf row, goes to the parent row.
	Fold(it Item, fold bool) (key string)
}

// Replacer is a Mode that can replace what its rows show in the files.
type Replacer interface {
	// BuildReplace makes the request that replaces the matches with text.
	// scope is "all" (every row), "node" (the selected row and the rows
	// below it), "file" (the file of the selected row) or "line" (the
	// selected line only).
	BuildReplace(with, scope string) (tree.ReplaceRequest, error)
}
