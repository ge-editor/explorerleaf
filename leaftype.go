package explorerleaf

import (
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/keychord"
)

type ExplorerKeymapBinder func(root *keychord.RootNode, explorer *ExplorerLeaf)

func NewLeafType(bind ExplorerKeymapBinder) *LeafType {
	return &LeafType{
		name:       "explorerleaf",
		bindKeymap: bind,
	}
}

// Implements LeafType interface
type LeafType struct {
	name       string
	bindKeymap ExplorerKeymapBinder
	ctx        tree.LeafContext
}

// Return *explorerleaf.Explorer as tree.Leaf *interface
// Explorer が生成されたタイミングで keymap を作成してバインドする
func (v *LeafType) NewLeaf() tree.Leaf {
	explorer := newExplorerLeaf()
	explorer.parentLeafType = v
	explorer.screen = screen.Get()

	// Explorer が生成されたタイミングで keymap を作成してバインド
	km := keychord.NewRootNode()
	if v.bindKeymap != nil {
		v.bindKeymap(km, explorer)
	}
	explorer.SetKeyDispatcher(km)

	var leaf tree.Leaf = explorer
	return leaf
}

// Create a new tree.Leaf (Explorer) from leaf *tree.Leaf information
// direction: "right", "bottom"
func (v *LeafType) NewSiblingLeaf(direction string, leaf tree.Leaf) tree.Leaf {
	newExplorer := newExplorerLeaf()
	newExplorer.parentLeafType = v
	newExplorer.screen = screen.Get()

	// Explorer が生成されたタイミングで keymap を作成してバインド
	km := keychord.NewRootNode()
	if v.bindKeymap != nil {
		v.bindKeymap(km, newExplorer)
	}
	newExplorer.SetKeyDispatcher(km)

	// Set the value of newExplorer from leafExplorer
	//leafExplorer := leaf.(*ExplorerLeaf)
	//newExplorer.editBuffer = leafExplorer.editBuffer  // same pointer
	//*newExplorer.meta = *leafExplorer.meta.DeepCopy() // copy value

	// Cast to tree.Leaf interface and return
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

// MinibufferLeaf は tree に属さない
// Explorer が生成されたタイミングで keymap をバインドしない
func NewMinibufferLeaf() *ExplorerLeaf {
	explorer := newExplorerLeaf()
	// ed.parentView = nil // tree に属さない leaf
	explorer.screen = screen.Get()

	// keymap をバインドしない
	/*
		km := keychord.NewRootNode()
		if v.bindKeymap != nil {
			v.bindKeymap(km, explorer)
		}
		explorer.SetKeyDispatcher(km)
	*/

	return explorer
}
