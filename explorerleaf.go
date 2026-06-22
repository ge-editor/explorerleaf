// Explorer Struct implements the gecore.tree.Leaf interface

package explorerleaf

import (
	"github.com/gdamore/tcell/v3"
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/gecore/styleresolver"
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/keychord"
	"github.com/ge-editor/locale"
	"github.com/ge-editor/utils"
)

func newExplorerLeaf() *ExplorerLeaf {

	e := &ExplorerLeaf{}

	// e.styleResolver.Add(&search.SearchResolver{})
	// e.styleResolver.Add(&styleresolver.SpecialCharResolver{})

	return e
}

// ------------------------------------------------------------------
// Editor implement gecore Leaf interface
// ------------------------------------------------------------------

// ExplorerLeaf Struct implements the gecore.tree.Leaf interface
type ExplorerLeaf struct {
	parentLeafType tree.LeafType
	screen         *screen.Screen
	active         bool

	viewArea     utils.Rect // include mode line
	explorerArea utils.Rect

	verticalThreshold int // Changes depending on screen size

	keyDispatcher *keychord.RootNode

	locale locale.Locale // Locale interface

	styleResolver       *styleresolver.Manager
	specialCharResolver *styleresolver.SpecialCharResolver
}

func (e *ExplorerLeaf) SetKeyDispatcher(km *keychord.RootNode) {
	e.keyDispatcher = km
}

func (e *ExplorerLeaf) DispatchKey(ev tcell.EventKey) (string, keychord.KeyDispatchTransition) {
	// gelog.Info("e.keyDispatcher")
	if e.keyDispatcher == nil {
		// gelog.Info("e.keyDispatcher == nil")
		return "", keychord.DispatchNotFound
	}

	s, res := e.keyDispatcher.Dispatch(ev)
	switch res {
	case keychord.DispatchNotFound:
	case keychord.DispatchPrefix:
	case keychord.DispatchExecuted:
	case keychord.DispatchInvalidAfterPrefix:
	}

	return s, res
}

// ------------------------------------------------------------------
// Methods of gecore Leaf interface
// ------------------------------------------------------------------

func (e *ExplorerLeaf) LeafType() tree.LeafType {
	return e.parentLeafType
}

func (e *ExplorerLeaf) Resize(viewArea utils.Rect) {
	e.viewArea = viewArea
	e.explorerArea = viewArea
}

func (e *ExplorerLeaf) Draw() bool {
	return false
}

func (e *ExplorerLeaf) Kill(leaf tree.Leaf, isActive bool) tree.Leaf {
	return leaf
}

func (e *ExplorerLeaf) Active(a bool) {
	e.active = a
}

func (e *ExplorerLeaf) IsActive() bool {
	return e.active
}

func (e *ExplorerLeaf) Resume() {
}

func (e *ExplorerLeaf) Init() {
}

func (e *ExplorerLeaf) WillClose() {
}
