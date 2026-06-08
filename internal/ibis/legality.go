package ibis

import "fmt"

// IllegalMove is returned when a move violates a legality rule. proto maps it to
// exit code 2 (ill-formed move, nothing written).
type IllegalMove struct {
	Detail string
	Node   string
}

func (e *IllegalMove) Error() string { return e.Detail }

func illegal(node, format string, a ...any) error {
	return &IllegalMove{Detail: fmt.Sprintf(format, a...), Node: node}
}

// CanRaise validates a raise: an optional parent must be an existing issue.
func (g *Graph) CanRaise(parent string) error {
	if parent == "" {
		return nil
	}
	n, ok := g.Nodes[parent]
	if !ok {
		return illegal(parent, "raise parent %q not found", parent)
	}
	if n.Kind != Issue {
		return illegal(parent, "raise parent must be an issue, got %s", n.Kind)
	}
	return nil
}

// CanPropose validates a propose: target must be an existing issue.
func (g *Graph) CanPropose(issue string) error {
	n, ok := g.Nodes[issue]
	if !ok {
		return illegal(issue, "propose target %q not found", issue)
	}
	if n.Kind != Issue {
		return illegal(issue, "propose target must be an issue, got %s", n.Kind)
	}
	return nil
}

// CanAttach validates support/object: target must be a position or argument.
func (g *Graph) CanAttach(target string, rel Rel) error {
	n, ok := g.Nodes[target]
	if !ok {
		return illegal(target, "%s target %q not found", rel, target)
	}
	if n.Kind != Position && n.Kind != Argument {
		return illegal(target, "%s target must be a position or argument, got %s", rel, n.Kind)
	}
	return nil
}

// CanPrefer validates a prefer: both endpoints must be AF nodes and the new edge
// must not create a preference cycle.
func (g *Graph) CanPrefer(winner, loser string) error {
	if winner == loser {
		return illegal(winner, "cannot prefer a node over itself")
	}
	if !g.IsAFNode(winner) {
		return illegal(winner, "prefer winner must be a position or argument")
	}
	if !g.IsAFNode(loser) {
		return illegal(loser, "prefer loser must be a position or argument")
	}
	// Reject if loser already (transitively) preferred over winner — that would
	// close a cycle.
	if g.preferredReaches(loser, winner) {
		return illegal(winner, "prefer(%s,%s) would create a preference cycle", winner, loser)
	}
	return nil
}

// CanDecide validates a decide: the position must respond_to the issue.
func (g *Graph) CanDecide(issue, position string) error {
	n, ok := g.Nodes[issue]
	if !ok {
		return illegal(issue, "decide issue %q not found", issue)
	}
	if n.Kind != Issue {
		return illegal(issue, "decide issue must be an issue, got %s", n.Kind)
	}
	p, ok := g.Nodes[position]
	if !ok {
		return illegal(position, "decide position %q not found", position)
	}
	if p.Kind != Position {
		return illegal(position, "decide position must be a position, got %s", p.Kind)
	}
	for _, l := range g.Links {
		if l.Rel == RespondsTo && l.Src == position && l.Dst == issue {
			return nil
		}
	}
	return illegal(position, "position %s does not respond_to issue %s", position, issue)
}

// CanConcede validates a concede/retract: the node must exist and be owned by
// the author.
func (g *Graph) CanConcede(node, author string) error {
	n, ok := g.Nodes[node]
	if !ok {
		return illegal(node, "concede target %q not found", node)
	}
	if n.Author != author {
		return illegal(node, "cannot concede %s: owned by %s, not %s", node, n.Author, author)
	}
	return nil
}

// preferredReaches reports whether `from` is preferred over `to` transitively.
func (g *Graph) preferredReaches(from, to string) bool {
	adj := map[string][]string{}
	for _, p := range g.Preferences {
		adj[p.Winner] = append(adj[p.Winner], p.Loser)
	}
	seen := map[string]bool{}
	var dfs func(string) bool
	dfs = func(cur string) bool {
		if cur == to {
			return true
		}
		if seen[cur] {
			return false
		}
		seen[cur] = true
		for _, nxt := range adj[cur] {
			if dfs(nxt) {
				return true
			}
		}
		return false
	}
	return dfs(from)
}
