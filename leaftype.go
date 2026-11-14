package explorerleaf

import (
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/gecore/workspace"
	"github.com/ge-editor/keychord"
)

type ExplorerKeymapBinder func(root *keychord.RootNode, explorer *ExplorerLeaf)

func NewLeafType(bind ExplorerKeymapBinder) *LeafType {
	return &LeafType{
		name:       "explorerleaf",
		bindKeymap: bind,
		modes:      []ModeFactory{FileModeFactory(currentWorkspaceRoots)},
	}
}

// currentWorkspaceRoots returns the directories of the workspace currently
// displayed by the active root tree. FileMode falls back to the working
// directory when no workspace is active or it has no directories.
func currentWorkspaceRoots() []string {
	if w := workspace.Current(); w != nil {
		return w.Directories
	}
	return nil
}

// Implements LeafType interface
type LeafType struct {
	name       string
	bindKeymap ExplorerKeymapBinder
	ctx        tree.LeafContext

	modes []ModeFactory
}

// AddMode adds a mode to the explorers of this type. The first one added is
// the initial mode.
func (v *LeafType) AddMode(f ModeFactory) *LeafType {
	v.modes = append(v.modes, f)
	return v
}

// newExplorer creates an ExplorerLeaf with its modes and keymap.
func (v *LeafType) newExplorer() *ExplorerLeaf {
	explorer := newExplorerLeaf()
	explorer.parentLeafType = v
	explorer.screen = screen.Get()
	for _, f := range v.modes {
		explorer.AddMode(f())
	}

	// Explorer が生成されたタイミングで keymap を作成してバインドする
	km := keychord.NewRootNode()
	if v.bindKeymap != nil {
		v.bindKeymap(km, explorer)
	}
	explorer.SetKeyDispatcher(km)
	return explorer
}

// Return *explorerleaf.Explorer as tree.Leaf *interface
func (v *LeafType) NewLeaf() tree.Leaf {
	var leaf tree.Leaf = v.newExplorer()
	return leaf
}

// Create a new tree.Leaf (Explorer) from leaf *tree.Leaf information
// direction: "right", "bottom"
func (v *LeafType) NewSiblingLeaf(direction string, leaf tree.Leaf) tree.Leaf {
	newExplorer := v.newExplorer()

	// The new pane starts in the mode the source pane shows.
	// (Each pane has its own mode state, so list positions are not copied.)
	if src, ok := leaf.(*ExplorerLeaf); ok && src.current < len(newExplorer.modes) {
		newExplorer.current = src.current
	}

	var tl tree.Leaf = newExplorer
	return tl
}

func (v *LeafType) RealName() string {
	return "github.com/ge-editor/explorerleaf"
}

func (v *LeafType) Name() string {
	return v.name
}

func (v *LeafType) SetRegisteredName(name string) {
	v.name = name
}

func (l *LeafType) SetCtx(ctx *tree.LeafContext) {
	l.ctx = *ctx

	ctx.CancelManager.Rotate("draw").Done()
}

func (l *LeafType) CancelManager() *gecore.EventCancelManager {
	return l.ctx.CancelManager // .Get("draw")
}

// DispatchLeafRequest currently handles no requests. It is present to satisfy tree.LeafType.
func (l *LeafType) DispatchLeafRequest(destination, behavior string, flag tree.OpenerFlag, data any, f func(bool, error)) {}

// MinibufferLeaf は tree に属さない
// Explorer が生成されたタイミングで keymap をバインドしない
func NewMinibufferLeaf() *ExplorerLeaf {
	explorer := newExplorerLeaf()
	// ed.parentView = nil // tree に属さない leaf
	explorer.screen = screen.Get()

	return explorer
}
