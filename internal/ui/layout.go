package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	qt "github.com/mappu/miqt/qt6"

	"gotti/internal/config"
)

// Node is a split tree: either a pane leaf or a splitter with children.
type Node struct {
	parent   *Node
	pane     *Pane
	split    *qt.QSplitter
	children []*Node
}

func (n *Node) widget() *qt.QWidget {
	if n.pane != nil {
		return n.pane.Widget
	}
	return n.split.QWidget
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
	container *qt.QWidget
	layout    *qt.QVBoxLayout
	root      *Node
	focused   *Pane
}

// Panes lists the panes in the tab.
func (t *Tab) Panes() []*Pane { return t.root.panes(nil) }

func newTab(w *Window) *Tab {
	t := &Tab{win: w}
	t.container = qt.NewQWidget(nil)
	t.container.SetObjectName("gottiTab")
	t.layout = qt.NewQVBoxLayout(t.container)
	t.layout.SetContentsMargins(0, 0, 0, 0)
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
func (t *Tab) Split(p *Pane, orient qt.Orientation, np *Pane) {
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
		par.split.InsertWidget(idx+1, np.Widget)
		leaf.parent = par
		par.children = append(par.children[:idx+1], append([]*Node{leaf}, par.children[idx+1:]...)...)
		if idx < len(sizes) {
			half := sizes[idx] / 2
			sizes[idx] = sizes[idx] - half
			sizes = append(sizes[:idx+1], append([]int{half}, sizes[idx+1:]...)...)
			par.split.SetSizes(sizes)
		}
		np.Widget.Show()
		return
	}
	// Replace the leaf with a new splitter holding it and the new pane.
	sp := qt.NewQSplitter3(orient)
	sp.SetObjectName("gottiSplit")
	sp.SetChildrenCollapsible(false)
	sp.SetHandleWidth(3)
	var parentSizes []int
	idx := 0
	if par != nil {
		idx = par.split.IndexOf(node.widget())
		parentSizes = par.split.Sizes()
	}
	inner := &Node{pane: node.pane, parent: node}
	sp.AddWidget(node.widget()) // moves p into the new splitter
	sp.AddWidget(np.Widget)
	node.widget().Show()
	np.Widget.Show()
	node.pane, node.split = nil, sp
	leaf.parent = node
	node.children = []*Node{inner, leaf}
	if par != nil {
		par.split.InsertWidget(idx, sp.QWidget)
		par.split.SetSizes(parentSizes)
	} else {
		t.layout.AddWidget(sp.QWidget)
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

// Remove takes p out of the tree; it reports whether the tab is now empty.
func (t *Tab) Remove(p *Pane) bool {
	node := t.root.find(p)
	if node == nil {
		return len(t.Panes()) == 0
	}
	p.releaseGL()
	if node == t.root {
		t.layout.RemoveWidget(p.Widget)
		p.Widget.DeleteLater()
		return true
	}
	par := node.parent
	for i, c := range par.children {
		if c == node {
			par.children = append(par.children[:i], par.children[i+1:]...)
			break
		}
	}
	// The closed pane's space goes to its neighbour (the one before it, or
	// after it when it was first), rather than wherever Qt would put it.
	idx := par.split.IndexOf(p.Widget)
	sizes := par.split.Sizes()
	p.Widget.Hide()
	p.Widget.SetParent(nil)
	p.Widget.DeleteLater()
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
			t.layout.RemoveWidget(par.split.QWidget)
			only.parent = nil
			t.root = only
		} else {
			// ReplaceWidget puts the child exactly where the splitter was,
			// with its geometry, so the sibling sizes are left untouched.
			gp.split.ReplaceWidget(gp.split.IndexOf(par.split.QWidget), only.widget())
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
	if n.split.Orientation() == qt.Vertical {
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

// buildLayout creates panes for ln inside tab t, splitting from p.
func (a *App) buildLayout(t *Tab, p *Pane, ln LayoutNode) {
	if len(ln.Children) == 0 {
		return
	}
	orient := qt.Horizontal
	if ln.Split == "rows" {
		orient = qt.Vertical
	}
	panes := []*Pane{p}
	cur := p
	for _, c := range ln.Children[1:] {
		leaf := firstLeaf(c) // a nested split starts as its first pane
		np, err := a.NewPane(t, leaf.Cwd, leaf.Command)
		if err != nil {
			continue
		}
		t.Split(cur, orient, np)
		panes = append(panes, np)
		cur = np
	}
	// apply sizes to the splitter now holding these panes
	if node := t.root.find(p); node != nil && node.parent != nil && len(ln.Sizes) == len(node.parent.children) {
		node.parent.split.SetSizes(ln.Sizes)
	}
	for i, c := range ln.Children {
		if i < len(panes) {
			a.buildLayout(t, panes[i], c)
		}
	}
}

// firstLeaf returns the first pane description of a layout subtree.
func firstLeaf(ln LayoutNode) LayoutNode {
	for len(ln.Children) > 0 {
		ln = ln.Children[0]
	}
	return ln
}
