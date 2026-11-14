package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtwidgets"

	"termite/internal/config"
)

// Node is a split tree: either a pane leaf or a splitter with children.
type Node struct {
	parent   *Node
	pane     *Pane
	split    *qtwidgets.QSplitter
	children []*Node
}

func (n *Node) widget() *qtwidgets.QWidget {
	if n.pane != nil {
		return n.pane.Widget
	}
	return n.split.AsQWidget()
}

func (n *Node) panes(out []*Pane) []*Pane {
	if n.pane != nil {
		return append(out, n.pane)
	}
	for _, c := range n.children {
		out = c.panes(out)
	}
	return out
}

func (n *Node) find(p *Pane) *Node {
	if n.pane == p {
		return n
	}
	for _, c := range n.children {
		if f := c.find(p); f != nil {
			return f
		}
	}
	return nil
}

// Tab is one page of a window, holding a split tree.
type Tab struct {
	win       *Window
	container *qtwidgets.QWidget
	layout    *qtwidgets.QVBoxLayout
	root      *Node
	focused   *Pane
	dropZone  *qtwidgets.QLabel // where a dragged pane would go
	dragging  *Pane             // the pane whose header is being dragged
}

// Panes lists the panes in the tab.
func (t *Tab) Panes() []*Pane { return t.root.panes(nil) }

func newTab(w *Window) *Tab {
	t := &Tab{win: w}
	t.container = qtwidgets.NewQWidget(nil)
	t.container.SetObjectName("termiteTab")
	t.layout = qtwidgets.NewQVBoxLayout(t.container)
	t.layout.SetContentsMargins(qtcore.QMargins{})
	t.layout.SetSpacing(0)
	return t
}

func (t *Tab) setRoot(n *Node) {
	t.root = n
	n.parent = nil
	t.layout.AddWidget(n.widget())
	n.widget().Show()
}

// Split divides pane p; orient is the QSplitter orientation
// (Horizontal = side by side, Vertical = stacked).
func (t *Tab) Split(p *Pane, orient qtcore.Orientation, np *Pane) {
	t.insert(p, orient, np, false)
}

// insert puts np beside p, splitting p's space in half: after p (right or
// below) or, with before, ahead of it (left or above).
func (t *Tab) insert(p *Pane, orient qtcore.Orientation, np *Pane, before bool) {
	node := t.root.find(p)
	if node == nil {
		return
	}
	leaf := &Node{pane: np}
	par := node.parent
	if par != nil && par.split.Orientation() == orient {
		// add next to p in the existing splitter, halving p's share
		idx := par.split.IndexOf(node.widget())
		sizes := par.split.Sizes()
		at := idx + 1
		if before {
			at = idx
		}
		par.split.InsertWidget(at, np.Widget)
		leaf.parent = par
		par.children = append(par.children[:at], append([]*Node{leaf}, par.children[at:]...)...)
		if idx < len(sizes) {
			half := sizes[idx] / 2
			sizes[idx] -= half
			sizes = append(sizes[:at], append([]int{half}, sizes[at:]...)...)
			par.split.SetSizes(sizes)
		}
		np.Widget.Show()
		return
	}
	// Replace the leaf with a new splitter holding it and the new pane. The
	// splitter starts inside the tab, so the panes never leave the window
	// (a GL widget changing windows loses its context).
	sp := qtwidgets.NewQSplitterWithOrientation(orient, t.container)
	sp.SetObjectName("termiteSplit")
	sp.SetChildrenCollapsible(false)
	sp.SetHandleWidth(3)
	var parentSizes []int
	idx := 0
	if par != nil {
		idx = par.split.IndexOf(node.widget())
		parentSizes = par.split.Sizes()
	}
	inner := &Node{pane: node.pane, parent: node}
	first, second := node.widget(), np.Widget
	node.children = []*Node{inner, leaf}
	if before {
		first, second = second, first
		node.children = []*Node{leaf, inner}
	}
	sp.AddWidget(first) // moves p into the new splitter
	sp.AddWidget(second)
	first.Show()
	second.Show()
	node.pane, node.split = nil, sp
	leaf.parent = node
	if par != nil {
		par.split.InsertWidget(idx, sp)
		par.split.SetSizes(parentSizes)
	} else {
		t.layout.AddWidget(sp)
	}
	sp.Show()
	sz := sp.Sizes()
	total := 0
	for _, s := range sz {
		total += s
	}
	if total > 0 {
		sp.SetSizes([]int{total / 2, total - total/2})
	}
}

// Remove takes p out of the tree and deletes its widgets; it reports
// whether the tab is now empty.
func (t *Tab) Remove(p *Pane) bool {
	if t.root.find(p) == nil {
		return len(t.Panes()) == 0
	}
	p.releaseGL()
	// A pane closing while being dragged never sees the release that would
	// end the drag; any drag ends, as the targets change.
	t.cancelDrag()
	empty := t.unlink(p)
	p.Widget.DeleteLater()
	return empty
}

// Move takes p out of its place and puts it against edge e of target, as
// dragging p's header onto target does.
func (t *Tab) Move(p, target *Pane, e dropEdge) {
	if p == target || t.root.find(p) == nil || t.root.find(target) == nil {
		return
	}
	t.unlink(p)
	t.insert(target, e.orient(), p, e.before())
	p.Widget.Show()
}

// unlink takes p out of the tree, leaving its widget hidden in the tab, and
// reports whether the tab is now empty.
func (t *Tab) unlink(p *Pane) bool {
	node := t.root.find(p)
	if node == t.root {
		t.layout.RemoveWidget(p.Widget)
		p.Widget.Hide()
		return true
	}
	par := node.parent
	for i, c := range par.children {
		if c == node {
			par.children = append(par.children[:i], par.children[i+1:]...)
			break
		}
	}
	// The pane's space goes to its neighbour (the one before it, or after
	// it when it was first), rather than wherever Qt would put it.
	idx := par.split.IndexOf(p.Widget)
	sizes := par.split.Sizes()
	p.Widget.Hide()
	// Out of the splitter, which may be deleted below, but kept in the
	// window (see insert).
	p.Widget.SetParent(t.container)
	if idx >= 0 && idx < len(sizes) && len(sizes) > 1 {
		freed := sizes[idx]
		sizes = append(sizes[:idx], sizes[idx+1:]...)
		sizes[max(idx-1, 0)] += freed
		par.split.SetSizes(sizes)
	}
	if len(par.children) == 1 {
		// Collapse the splitter into its remaining child.
		only := par.children[0]
		gp := par.parent
		if gp == nil {
			t.layout.AddWidget(only.widget())
			t.layout.RemoveWidget(par.split)
			only.parent = nil
			t.root = only
		} else if only.split != nil && only.split.Orientation() == gp.split.Orientation() {
			t.splice(gp, par, only)
			return false
		} else {
			// ReplaceWidget puts the child exactly where the splitter was,
			// with its geometry, so the sibling sizes are left untouched.
			gp.split.ReplaceWidget(gp.split.IndexOf(par.split), only.widget())
			for i, c := range gp.children {
				if c == par {
					gp.children[i] = only
				}
			}
			only.parent = gp
		}
		only.widget().Show()
		par.split.Hide()
		par.split.DeleteLater()
	}
	return false
}

// splice replaces gp's child par, a splitter left holding only inner, a
// splitter of gp's orientation, with inner's children, sharing par's space
// as inner did. Splitters of one orientation never nest: a saved layout
// could not be built again from columns within columns.
func (t *Tab) splice(gp, par, inner *Node) {
	idx := gp.split.IndexOf(par.split)
	outer, sizes := gp.split.Sizes(), inner.split.Sizes()
	total := 0
	for _, s := range sizes {
		total += s
	}
	var space int
	if idx >= 0 && idx < len(outer) {
		space = outer[idx]
		outer = slices.Delete(outer, idx, idx+1)
	}
	if total > 0 {
		left := space
		for i := range sizes[:len(sizes)-1] {
			sizes[i] = sizes[i] * space / total
			left -= sizes[i]
		}
		sizes[len(sizes)-1] = left
	}
	// Out of gp first, so its widgets and sizes line up again.
	par.split.Hide()
	par.split.SetParent(t.container)
	for i, c := range inner.children {
		gp.split.InsertWidget(idx+i, c.widget())
		c.parent = gp
		c.widget().Show()
	}
	at := slices.Index(gp.children, par)
	gp.children = slices.Replace(gp.children, at, at+1, inner.children...)
	if idx >= 0 && len(outer)+len(sizes) == gp.split.Count() {
		gp.split.SetSizes(slices.Insert(outer, idx, sizes...))
	}
	par.split.DeleteLater() // and inner's, now empty, with it
}

// ---------------------------------------------------------------- layouts

// LayoutNode is the saved form of a split tree.
type LayoutNode struct {
	Split    string       `json:"split,omitempty"` // "columns" (side by side) or "rows" (stacked)
	Sizes    []int        `json:"sizes,omitempty"`
	Children []LayoutNode `json:"children,omitempty"`
	Cwd      string       `json:"cwd,omitempty"`
	Command  []string     `json:"command,omitempty"`
}

// LayoutFile is a saved window layout.
type LayoutFile struct {
	Tabs []LayoutNode `json:"tabs"`
}

func layoutDir() string { return filepath.Join(config.Dir(), "layouts") }

func layoutPath(name string) string {
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == 0 {
			return '_'
		}
		return r
	}, name)
	return filepath.Join(layoutDir(), name+".json")
}

// ListLayouts returns saved layout names.
func ListLayouts() []string {
	m, _ := filepath.Glob(filepath.Join(layoutDir(), "*.json"))
	var out []string
	for _, f := range m {
		out = append(out, strings.TrimSuffix(filepath.Base(f), ".json"))
	}
	sort.Strings(out)
	return out
}

func (n *Node) save() LayoutNode {
	if n.pane != nil {
		return LayoutNode{Cwd: n.pane.cwd(), Command: n.pane.command}
	}
	ln := LayoutNode{Split: "columns", Sizes: n.split.Sizes()}
	if n.split.Orientation() == qtcore.Vertical {
		ln.Split = "rows"
	}
	for _, c := range n.children {
		ln.Children = append(ln.Children, c.save())
	}
	return ln
}

// SaveLayout writes the window's tabs to a named layout file.
func (w *Window) SaveLayout(name string) error {
	var f LayoutFile
	for i := 0; i < w.tabs.Count(); i++ {
		if t := w.tabAt(i); t != nil {
			f.Tabs = append(f.Tabs, t.root.save())
		}
	}
	if err := os.MkdirAll(layoutDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(layoutPath(name), append(data, '\n'), 0o644)
}

// LoadLayout reads a named layout.
func LoadLayout(name string) (*LayoutFile, error) {
	var f LayoutFile
	data, err := os.ReadFile(layoutPath(name))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("layout %s: %w", name, err)
	}
	return &f, nil
}

// RemoveLayout deletes a named layout.
func RemoveLayout(name string) error { return os.Remove(layoutPath(name)) }

// buildLayout creates panes for ln inside tab t, splitting from p.
func (a *App) buildLayout(t *Tab, p *Pane, ln LayoutNode) {
	if len(ln.Children) == 0 {
		return
	}
	orient := qtcore.Horizontal
	if ln.Split == "rows" {
		orient = qtcore.Vertical
	}
	// panes[i] holds ln.Children[i], nil where its pane failed to start,
	// so each nested split is still built in the right pane.
	panes := []*Pane{p}
	cur := p
	for _, c := range ln.Children[1:] {
		leaf := firstLeaf(c) // a nested split starts as its first pane
		np, err := a.NewPane(t, leaf.Cwd, leaf.Command)
		if err != nil {
			panes = append(panes, nil)
			continue
		}
		t.Split(cur, orient, np)
		panes = append(panes, np)
		cur = np
	}
	// The splitter now holding these panes, found before the nested splits
	// are built: p may be inside one of them afterwards. With no pane but p
	// started, no splitter was made, and p's parent is an outer one.
	var level *Node
	if node := t.root.find(p); node != nil && slices.ContainsFunc(panes[1:], func(q *Pane) bool { return q != nil }) {
		level = node.parent
	}
	for i, c := range ln.Children {
		if panes[i] != nil {
			a.buildLayout(t, panes[i], c)
		}
	}
	// Sizes go on last: building a nested split resets its parent's.
	if level != nil && len(ln.Sizes) == len(level.children) {
		level.split.SetSizes(ln.Sizes)
	}
}

// firstLeaf returns the first pane description of a layout subtree.
func firstLeaf(ln LayoutNode) LayoutNode {
	for len(ln.Children) > 0 {
		ln = ln.Children[0]
	}
	return ln
}
