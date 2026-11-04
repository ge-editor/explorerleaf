// ExplorerLeaf implements the gecore.tree.Leaf interface.
//
// One ExplorerLeaf shows one Mode at a time (files, functions, marks ...).
// Every Mode keeps its own list, cursor and filter, so switching modes and
// coming back restores where the user was. The leaf itself only owns what is
// common: the list operations, the filter input, drawing and the key
// dispatch.

package explorerleaf

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/excommand"
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/keychord"
	"github.com/ge-editor/locale"
	"github.com/ge-editor/theme"
)

var _ tree.Leaf = (*ExplorerLeaf)(nil)

func newExplorerLeaf() *ExplorerLeaf {
	return &ExplorerLeaf{needReload: true}
}

// ExplorerLeaf Struct implements the gecore.tree.Leaf interface
type ExplorerLeaf struct {
	parentLeafType tree.LeafType
	screen         *screen.Screen
	active         bool

	viewArea     screen.Rect // include mode line
	explorerArea screen.Rect

	verticalThreshold int // Changes depending on screen size

	keyDispatcher *keychord.RootNode

	locale locale.Locale // Locale interface

	modes   []Mode
	current int
	// needReload makes the next Draw reload the current mode.
	needReload bool
}

func (e *ExplorerLeaf) SetKeyDispatcher(km *keychord.RootNode) {
	e.keyDispatcher = km
}

// AddMode adds a mode. The first one added is the initial mode.
func (e *ExplorerLeaf) AddMode(m Mode) {
	e.modes = append(e.modes, m)
}

// --------------------
// Modes
// --------------------

// Mode returns the current mode, or nil when there is none.
func (e *ExplorerLeaf) Mode() Mode {
	if e.current < 0 || e.current >= len(e.modes) {
		return nil
	}
	return e.modes[e.current]
}

// ModeNames returns the names of all modes.
func (e *ExplorerLeaf) ModeNames() []string {
	names := make([]string, len(e.modes))
	for i, m := range e.modes {
		names[i] = m.Name()
	}
	return names
}

// SetMode switches to the mode called name (case-insensitive).
func (e *ExplorerLeaf) SetMode(name string) bool {
	for i, m := range e.modes {
		if strings.EqualFold(m.Name(), name) {
			e.selectMode(i)
			return true
		}
	}
	return false
}

func (e *ExplorerLeaf) NextMode() { e.cycleMode(1) }
func (e *ExplorerLeaf) PrevMode() { e.cycleMode(-1) }

func (e *ExplorerLeaf) cycleMode(dir int) {
	n := len(e.modes)
	if n < 2 {
		return
	}
	e.selectMode((e.current + dir + n) % n)
}

func (e *ExplorerLeaf) selectMode(i int) {
	if i == e.current {
		return
	}
	e.current = i
	// The mode keeps its list, cursor and filter. Only refresh its items.
	e.needReload = true
}

// --------------------
// List operations (shared by every mode)
// --------------------

func (e *ExplorerLeaf) listOp(f func(l *List)) {
	if m := e.Mode(); m != nil {
		f(m.List())
	}
}

func (e *ExplorerLeaf) MoveUp()   { e.listOp(func(l *List) { l.Move(-1) }) }
func (e *ExplorerLeaf) MoveDown() { e.listOp(func(l *List) { l.Move(1) }) }
func (e *ExplorerLeaf) Home()     { e.listOp(func(l *List) { l.Home() }) }
func (e *ExplorerLeaf) End()      { e.listOp(func(l *List) { l.End() }) }

func (e *ExplorerLeaf) PageUp() {
	e.listOp(func(l *List) { l.Page(-1, e.listHeight()) })
}

func (e *ExplorerLeaf) PageDown() {
	e.listOp(func(l *List) { l.Page(1, e.listHeight()) })
}

// listHeight is the number of list rows (the area minus the header line).
func (e *ExplorerLeaf) listHeight() int {
	return e.explorerArea.Height - 1
}

// Reload rereads the data source of the current mode.
func (e *ExplorerLeaf) Reload() {
	e.needReload = false
	m := e.Mode()
	if m == nil {
		return
	}
	if err := m.Reload(); err != nil {
		gecore.Echo.AddText(err.Error())
	}
}

// Activate runs Enter on the selected row.
func (e *ExplorerLeaf) Activate() {
	m := e.Mode()
	if m == nil {
		return
	}
	it, ok := m.List().Selected()
	if !ok {
		return
	}
	act := m.Activate(it)
	if act.Message != "" {
		gecore.Echo.AddText(act.Message)
	}
	if act.Open == nil {
		return
	}

	tree.Opener("editorleaf", "openfile", tree.ActiveOrFirstLeaf, tree.OpenRequest{
		Path: act.Open.Path,
		Line: act.Open.Line,
		Col:  act.Open.Col,
	}, func(handled bool, err error) {
		if err != nil {
			gecore.Echo.AddText(err.Error())
		}
	})
}

// --------------------
// Filter input
// --------------------

func (e *ExplorerLeaf) InsertFilter(s string) {
	if m := e.Mode(); m != nil {
		m.SetFilter(m.List().Filter() + s)
	}
}

func (e *ExplorerLeaf) BackspaceFilter() {
	m := e.Mode()
	if m == nil {
		return
	}
	r := []rune(m.List().Filter())
	if len(r) == 0 {
		return
	}
	m.SetFilter(string(r[:len(r)-1]))
}

func (e *ExplorerLeaf) ClearFilter() {
	if m := e.Mode(); m != nil {
		m.SetFilter("")
	}
}

// --------------------
// Key dispatch
// --------------------

func (e *ExplorerLeaf) DispatchKey(ev tcell.EventKey) (string, keychord.KeyDispatchTransition) {
	if e.keyDispatcher == nil {
		return "", keychord.DispatchNotFound
	}

	s, res := e.keyDispatcher.Dispatch(ev)
	if res == keychord.DispatchNotFound {
		// Unbound printable keys narrow the list, like typing in the editor.
		// An unbound Ctrl/Alt/Meta combination is not text.
		if ev.Modifiers()&(tcell.ModCtrl|tcell.ModAlt|tcell.ModMeta) == 0 {
			if str := ev.Str(); isPrintable(str) {
				e.InsertFilter(str)
			}
		}
	}

	return s, res
}

func isPrintable(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// --------------------
// Methods of gecore Leaf interface
// --------------------

func (e *ExplorerLeaf) LeafType() tree.LeafType {
	return e.parentLeafType
}

func (e *ExplorerLeaf) Resize(viewArea screen.Rect) {
	e.viewArea = viewArea
	e.explorerArea = viewArea
}

// Draw returns true when it was canceled by ctx.
func (e *ExplorerLeaf) Draw(ctx context.Context) bool {
	if ctx.Err() != nil {
		return true
	}
	if e.needReload {
		e.Reload()
	}

	a := e.explorerArea
	if a.IsEmpty() {
		return false
	}

	style := theme.ColorDefault
	e.screen.FillRect(a, ' ', style)

	m := e.Mode()
	if m == nil {
		e.screen.DrawString(a.X, a.Y, a.Width, "(no explorer mode)", style)
		return false
	}
	l := m.List()

	// Header: [Mode] filter (shown/total)
	header := "[" + m.Name() + "]"
	if f := l.Filter(); f != "" {
		header += " " + f + fmt.Sprintf("  (%d/%d)", l.Len(), l.Total())
	}
	e.screen.DrawString(a.X, a.Y, a.Width, header, style.Bold(true))

	rows, selected := l.Visible(a.Height - 1)
	if len(rows) == 0 && a.Height > 1 {
		e.screen.DrawString(a.X, a.Y+1, a.Width, "(empty)", style)
	}
	for i, it := range rows {
		st := style
		if i == selected {
			st = style.Reverse(true)
		}
		e.screen.DrawString(a.X, a.Y+1+i, a.Width, it.display(), st)
	}

	// The explorer has no text cursor: hide the one the editor left.
	if e.active {
		e.screen.Screen.ShowCursor(-1, -1)
	}
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
	// The files may have changed while another leaf was in use.
	e.needReload = true
}

func (e *ExplorerLeaf) Init() {
}

func (e *ExplorerLeaf) WillClose() {
}

func (e *ExplorerLeaf) DispatchMouse(ev tcell.EventMouse) {
	btn := ev.Buttons()
	switch {
	case btn&tcell.WheelDown != 0:
		e.listOp(func(l *List) { l.Move(3) })
	case btn&tcell.WheelUp != 0:
		e.listOp(func(l *List) { l.Move(-3) })
	}
}

// --------------------
// Ex commands (ESC x)
// --------------------

func (e *ExplorerLeaf) Commands() []excommand.ExCommand {
	return []excommand.ExCommand{
		&command{
			name: "explorer-refresh",
			desc: "Reload the list of the explorer.",
			run: func(ctx context.Context, args []string) (bool, error) {
				e.Reload()
				return true, nil
			},
		},
		&command{
			name: "explorer-mode",
			desc: "Switch the explorer mode.",
			run: func(ctx context.Context, args []string) (bool, error) {
				if len(args) == 0 {
					e.NextMode()
					return true, nil
				}
				if !e.SetMode(strings.Join(args, " ")) {
					return true, fmt.Errorf("explorer: unknown mode %q", strings.Join(args, " "))
				}
				return true, nil
			},
			complete: func(input string) []string {
				var out []string
				for _, n := range e.ModeNames() {
					if strings.HasPrefix(strings.ToLower(n), strings.ToLower(input)) {
						out = append(out, n)
					}
				}
				return out
			},
		},
	}
}

// command implements excommand.ExCommand and excommand.ArgCompleter.
type command struct {
	name     string
	desc     string
	run      func(ctx context.Context, args []string) (bool, error)
	complete func(input string) []string
}

func (c *command) Name() string        { return c.name }
func (c *command) Aliases() []string   { return nil }
func (c *command) Description() string { return c.desc }

func (c *command) Execute(ctx context.Context, args []string) (bool, error) {
	return c.run(ctx, args)
}

// CompleteArgs implements excommand.ArgCompleter.
func (c *command) CompleteArgs(input string) []string {
	if c.complete == nil {
		return nil
	}
	return c.complete(input)
}
