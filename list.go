package explorerleaf

import (
	"strings"

	"github.com/gdamore/tcell/v3"
)

// Item is one row of a List.
//
// The list only knows how to show, select and filter rows. What a row
// means (a file, a symbol, a search hit, a mark ...) lives in Data and is
// interpreted by the Mode that created it. The modes do not have to share a
// node type.
type Item struct {
	// Key identifies the row across SetItems calls (e.g. a file path), so
	// the cursor stays on the same row when a mode rebuilds its items.
	// Optional.
	Key string
	// Text is what the filter matches.
	Text string
	// Display is what is drawn. Text is drawn when Display is empty.
	Display string
	// Data is the mode-specific payload.
	Data any
	// Badge is a short mark drawn at the right end of the row (a git status
	// letter). BadgeColor is its color; the default color when zero.
	Badge      string
	BadgeColor tcell.Color
}

func (it Item) display() string {
	if it.Display != "" {
		return it.Display
	}
	return it.Text
}

// List is the part of the explorer shared by every mode: items, filter,
// cursor and scrolling. It has no knowledge of the screen or of any data
// source.
type List struct {
	items []Item
	// view holds the indexes into items that pass the filter.
	view   []int
	filter string
	terms  []string
	// cursor and top are indexes into view.
	cursor int
	top    int
}

// SetItems replaces the items. The cursor stays on the row with the same
// Key when there is one, otherwise it keeps its index (clamped).
func (l *List) SetItems(items []Item) {
	key := ""
	if it, ok := l.Selected(); ok {
		key = it.Key
	}
	cursor := l.cursor

	l.items = items
	l.rebuild()

	l.cursor = cursor
	if key != "" {
		for vi, ii := range l.view {
			if l.items[ii].Key == key {
				l.cursor = vi
				break
			}
		}
	}
	l.clamp()
}

// SetFilter sets the filter text. Space separated terms must all appear in
// Item.Text (case-insensitive). A non-empty filter moves the cursor to the
// first match; clearing it keeps the selected row when it is still shown.
func (l *List) SetFilter(s string) {
	if s == l.filter {
		return
	}
	key := ""
	if it, ok := l.Selected(); ok {
		key = it.Key
	}

	l.filter = s
	l.terms = strings.Fields(strings.ToLower(s))
	l.rebuild()

	l.cursor = 0
	if s == "" && key != "" {
		for vi, ii := range l.view {
			if l.items[ii].Key == key {
				l.cursor = vi
				break
			}
		}
	}
	l.top = 0
	l.clamp()
}

func (l *List) Filter() string { return l.filter }

// Len is the number of rows that pass the filter.
func (l *List) Len() int { return len(l.view) }

// Total is the number of items before filtering.
func (l *List) Total() int { return len(l.items) }

// Selected returns the item under the cursor.
func (l *List) Selected() (Item, bool) {
	if len(l.view) == 0 || l.cursor < 0 || l.cursor >= len(l.view) {
		return Item{}, false
	}
	return l.items[l.view[l.cursor]], true
}

// SelectRow puts the cursor on the i-th drawn row (0 is the top row of the
// last Visible call). It reports whether there is such a row.
func (l *List) SelectRow(i int) bool {
	idx := l.top + i
	if i < 0 || idx >= len(l.view) {
		return false
	}
	l.cursor = idx
	return true
}

// SelectKey puts the cursor on the row with the given Key. It reports
// whether the row is shown.
func (l *List) SelectKey(key string) bool {
	for vi, ii := range l.view {
		if l.items[ii].Key == key {
			l.cursor = vi
			l.clamp()
			return true
		}
	}
	return false
}

// Move moves the cursor by delta rows and stops at both ends.
func (l *List) Move(delta int) {
	l.cursor += delta
	l.clamp()
}

func (l *List) Home() { l.cursor = 0; l.clamp() }

func (l *List) End() { l.cursor = len(l.view) - 1; l.clamp() }

// Page moves the cursor by one page of the given height. dir is +1 or -1.
func (l *List) Page(dir, height int) {
	step := height - 1
	if step < 1 {
		step = 1
	}
	l.Move(dir * step)
}

// Visible returns the rows to draw in an area of the given height, scrolling
// as needed to keep the cursor in view. selected is the index of the cursor
// row within the returned slice, or -1.
func (l *List) Visible(height int) (rows []Item, selected int) {
	if height <= 0 || len(l.view) == 0 {
		return nil, -1
	}
	if l.cursor < l.top {
		l.top = l.cursor
	}
	if l.cursor >= l.top+height {
		l.top = l.cursor - height + 1
	}
	// Do not leave blank rows at the bottom when there are rows above.
	if limit := len(l.view) - height; l.top > limit {
		l.top = limit
	}
	if l.top < 0 {
		l.top = 0
	}

	end := l.top + height
	if end > len(l.view) {
		end = len(l.view)
	}
	rows = make([]Item, 0, end-l.top)
	for _, ii := range l.view[l.top:end] {
		rows = append(rows, l.items[ii])
	}
	return rows, l.cursor - l.top
}

func (l *List) rebuild() {
	l.view = l.view[:0]
	for i, it := range l.items {
		if l.matches(it) {
			l.view = append(l.view, i)
		}
	}
}

func (l *List) matches(it Item) bool {
	if len(l.terms) == 0 {
		return true
	}
	text := strings.ToLower(it.Text)
	for _, term := range l.terms {
		if !strings.Contains(text, term) {
			return false
		}
	}
	return true
}

func (l *List) clamp() {
	if len(l.view) == 0 {
		l.cursor, l.top = 0, 0
		return
	}
	if l.cursor < 0 {
		l.cursor = 0
	}
	if l.cursor > len(l.view)-1 {
		l.cursor = len(l.view) - 1
	}
}
