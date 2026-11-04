package explorerleaf

import (
	"fmt"
	"testing"
)

func newItems(n int) []Item {
	items := make([]Item, n)
	for i := range items {
		items[i] = Item{Key: fmt.Sprint("k", i), Text: fmt.Sprint("item", i)}
	}
	return items
}

func selectedKey(t *testing.T, l *List) string {
	t.Helper()
	it, ok := l.Selected()
	if !ok {
		t.Fatal("nothing selected")
	}
	return it.Key
}

func TestListMoveClamps(t *testing.T) {
	var l List
	l.SetItems(newItems(3))
	l.Move(-5)
	if got := selectedKey(t, &l); got != "k0" {
		t.Fatalf("got %s, want k0", got)
	}
	l.Move(10)
	if got := selectedKey(t, &l); got != "k2" {
		t.Fatalf("got %s, want k2", got)
	}
	l.Home()
	if got := selectedKey(t, &l); got != "k0" {
		t.Fatalf("Home: got %s", got)
	}
	l.End()
	if got := selectedKey(t, &l); got != "k2" {
		t.Fatalf("End: got %s", got)
	}
}

func TestListEmpty(t *testing.T) {
	var l List
	if _, ok := l.Selected(); ok {
		t.Fatal("empty list has a selection")
	}
	l.Move(1)
	l.End()
	if rows, sel := l.Visible(5); rows != nil || sel != -1 {
		t.Fatalf("Visible on empty list = %v, %d", rows, sel)
	}
}

func TestListFilter(t *testing.T) {
	var l List
	l.SetItems([]Item{
		{Key: "a", Text: "Main.go"},
		{Key: "b", Text: "util/helper.go"},
		{Key: "c", Text: "README.md"},
	})
	l.Move(2)

	l.SetFilter("GO")
	if l.Len() != 2 || l.Total() != 3 {
		t.Fatalf("Len/Total = %d/%d, want 2/3", l.Len(), l.Total())
	}
	if got := selectedKey(t, &l); got != "a" {
		t.Fatalf("filtering moves to the first match, got %s", got)
	}

	l.SetFilter("util go") // every term must match
	if l.Len() != 1 || selectedKey(t, &l) != "b" {
		t.Fatalf("Len = %d", l.Len())
	}

	l.SetFilter("zzz")
	if l.Len() != 0 {
		t.Fatalf("Len = %d, want 0", l.Len())
	}
	if _, ok := l.Selected(); ok {
		t.Fatal("no match must select nothing")
	}
}

func TestListClearFilterKeepsSelection(t *testing.T) {
	var l List
	l.SetItems(newItems(5))
	l.SetFilter("item3")
	l.SetFilter("")
	if got := selectedKey(t, &l); got != "k3" {
		t.Fatalf("got %s, want k3", got)
	}
}

func TestListSetItemsKeepsCursorByKey(t *testing.T) {
	var l List
	l.SetItems(newItems(5))
	l.Move(3) // k3

	// An item is inserted before the selected one.
	items := newItems(5)
	items = append([]Item{{Key: "new", Text: "new"}}, items...)
	l.SetItems(items)
	if got := selectedKey(t, &l); got != "k3" {
		t.Fatalf("got %s, want k3", got)
	}

	// The selected item disappears: the index is kept and clamped.
	l.SetItems(newItems(2))
	if got := selectedKey(t, &l); got != "k1" {
		t.Fatalf("got %s, want k1", got)
	}
}

func TestListVisibleScrolls(t *testing.T) {
	var l List
	l.SetItems(newItems(10))

	rows, sel := l.Visible(3)
	if len(rows) != 3 || sel != 0 || rows[0].Key != "k0" {
		t.Fatalf("rows=%v sel=%d", rows, sel)
	}

	l.Move(5) // cursor k5 goes below the window
	rows, sel = l.Visible(3)
	if rows[0].Key != "k3" || sel != 2 {
		t.Fatalf("first=%s sel=%d, want k3/2", rows[0].Key, sel)
	}

	l.Move(-4) // cursor k1 goes above the window
	rows, sel = l.Visible(3)
	if rows[0].Key != "k1" || sel != 0 {
		t.Fatalf("first=%s sel=%d, want k1/0", rows[0].Key, sel)
	}

	l.End()
	rows, sel = l.Visible(3)
	if rows[0].Key != "k7" || sel != 2 {
		t.Fatalf("first=%s sel=%d, want k7/2", rows[0].Key, sel)
	}
}

func TestListPage(t *testing.T) {
	var l List
	l.SetItems(newItems(20))
	l.Page(1, 5) // moves by height-1
	if got := selectedKey(t, &l); got != "k4" {
		t.Fatalf("got %s, want k4", got)
	}
	l.Page(-1, 5)
	if got := selectedKey(t, &l); got != "k0" {
		t.Fatalf("got %s, want k0", got)
	}
	l.Page(1, 1) // a tiny area still moves
	if got := selectedKey(t, &l); got != "k1" {
		t.Fatalf("got %s, want k1", got)
	}
}
