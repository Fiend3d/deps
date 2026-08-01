package main

import (
	"strings"
)

// treeNode is one module in the recursive dependency tree.
type treeNode struct {
	dep    dependency
	depth  int
	parent *treeNode

	expanded bool
	// loaded records that children were resolved already, so a module with no
	// dependencies of its own is not re-parsed on every expand.
	loaded   bool
	loadErr  string
	children []*treeNode

	// cycle marks a module that already appears in its own ancestor chain.
	// Expanding it would recurse forever, so it is shown but not opened.
	cycle bool
}

// expandable reports whether the node can be opened. Modules with no file on
// disk have nothing to read, and cycles would not terminate.
func (n *treeNode) expandable() bool {
	return n.dep.found && !n.cycle
}

// ancestorOf reports whether path is already open above this node. Windows
// paths are case-insensitive, so the comparison is too.
func (n *treeNode) hasAncestor(path string) bool {
	for a := n; a != nil; a = a.parent {
		if strings.EqualFold(a.dep.path, path) {
			return true
		}
	}
	return false
}

// toggle opens or closes a node, resolving its children the first time it is
// opened. Nothing here is fatal: a module that fails to parse records the
// failure on the node and stays closed.
func (n *treeNode) toggle(resolve resolver) {
	if !n.expandable() {
		return
	}

	if n.expanded {
		n.expanded = false
		return
	}

	if !n.loaded {
		deps, err := resolve(n.dep.path)
		if err != nil {
			n.loadErr = err.Error()
			n.loaded = true
			return
		}

		n.children = make([]*treeNode, 0, len(deps))
		for _, dep := range deps {
			child := &treeNode{
				dep:    dep,
				depth:  n.depth + 1,
				parent: n,
			}
			// A module already open above this point would recurse forever.
			child.cycle = dep.found && n.hasAncestor(dep.path)
			n.children = append(n.children, child)
		}
		n.loaded = true
	}

	n.expanded = true
}

// newTree turns the direct dependencies of the root image into tree roots.
func newTree(deps []dependency, rootPath string) []*treeNode {
	// A synthetic parent carrying the root image's path, so a dependency that
	// points back at the image it came from is caught as a cycle like any other.
	root := &treeNode{dep: dependency{path: rootPath}, depth: -1}

	nodes := make([]*treeNode, 0, len(deps))
	for _, dep := range deps {
		node := &treeNode{dep: dep, parent: root}
		node.cycle = dep.found && root.hasAncestor(dep.path)
		nodes = append(nodes, node)
	}
	// Give the synthetic parent its children so sibling queries work the same
	// at the top level as anywhere else.
	root.children = nodes

	return nodes
}

// isLast reports whether the node is the last of its siblings, which decides
// the branch glyph it is drawn with.
func (n *treeNode) isLast() bool {
	if n.parent == nil {
		return true
	}
	siblings := n.parent.children
	return len(siblings) == 0 || siblings[len(siblings)-1] == n
}

// prefix draws the indent and branch glyph, with continuation bars for
// ancestors that still have siblings below them.
func (n *treeNode) prefix() string {
	if n.depth <= 0 {
		return ""
	}

	parts := make([]string, n.depth)
	if n.isLast() {
		parts[n.depth-1] = "└─ "
	} else {
		parts[n.depth-1] = "├─ "
	}
	for a, i := n.parent, n.depth-2; a != nil && i >= 0; a, i = a.parent, i-1 {
		if a.isLast() {
			parts[i] = "   "
		} else {
			parts[i] = "│  "
		}
	}

	return strings.Join(parts, "")
}

// marker describes the node's state to the right of its name.
func (n *treeNode) marker() string {
	switch {
	case n.cycle:
		return " (cycle)"
	case n.loadErr != "":
		return " (unreadable)"
	case !n.expandable():
		return ""
	case n.expanded:
		return " [-]"
	default:
		return " [+]"
	}
}

// flattenTree walks the open nodes depth-first into the row order shown on
// screen.
func flattenTree(roots []*treeNode) []*treeNode {
	var visible []*treeNode

	var walk func(nodes []*treeNode)
	walk = func(nodes []*treeNode) {
		for _, node := range nodes {
			visible = append(visible, node)
			if node.expanded {
				walk(node.children)
			}
		}
	}
	walk(roots)

	return visible
}

// treeText renders the visible rows as indented plain text, for copying.
func treeText(visible []*treeNode) string {
	var b strings.Builder
	for i, node := range visible {
		if i > 0 {
			b.WriteRune('\n')
		}
		b.WriteString(strings.Repeat("  ", node.depth))
		b.WriteString(node.dep.label())
		if node.cycle {
			b.WriteString(" (cycle)")
		}
	}
	return b.String()
}
