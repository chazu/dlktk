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

// NotFound is returned when a move references a node id that does not exist.
// fail maps it to exit code 3 (the discover contract reserves 3 for "a
// referenced discussion/issue/node id does not exist"; 2 is for moves that are
// ill-formed against nodes that do exist).
type NotFound struct {
	Detail string
	Node   string
}

func (e *NotFound) Error() string { return e.Detail }

func notFound(node, format string, a ...any) error {
	return &NotFound{Detail: fmt.Sprintf(format, a...), Node: node}
}

// CanRaise validates a raise: an optional parent must be an existing issue; an
// optional from-node (the position/argument that revealed the new question)
// must be an existing AF node. Parent and from are mutually exclusive.
func (g *Graph) CanRaise(parent, from string) error {
	if parent != "" && from != "" {
		return illegal(parent, "raise takes --parent or --from, not both")
	}
	if from != "" {
		n, ok := g.Nodes[from]
		if !ok {
			return notFound(from, "raise --from node %q not found", from)
		}
		if n.Kind != Position && n.Kind != Argument {
			return illegal(from, "raise --from must name a position or argument, got %s", n.Kind)
		}
		return nil
	}
	if parent == "" {
		return nil
	}
	n, ok := g.Nodes[parent]
	if !ok {
		return notFound(parent, "raise parent %q not found", parent)
	}
	if n.Kind != Issue {
		return illegal(parent, "raise parent must be an issue, got %s", n.Kind)
	}
	return nil
}

// CanReframe validates a reframe of an issue's framing: the old issue must
// exist, be an issue, and not already be reframed. (Whether it carries a
// standing decision is checked by the move layer, which reads decisions:
// reframing a decided issue is illegal — supersede the decision first, or the
// reframe would silently bury it, the Q4 defect in a new hat.)
func (g *Graph) CanReframe(old string) error {
	n, ok := g.Nodes[old]
	if !ok {
		return notFound(old, "reframe target %q not found", old)
	}
	if n.Kind != Issue {
		return illegal(old, "reframe target must be an issue, got %s", n.Kind)
	}
	if to, done := g.ReframedTo(old); done {
		return illegal(old, "issue %s is already reframed (-> %s); reframe %s instead", old, to, to)
	}
	return nil
}

// CanSynthesize validates a synthesis: a hybrid position built from two or
// more distinct existing positions on the same issue.
func (g *Graph) CanSynthesize(issue string, froms []string) error {
	n, ok := g.Nodes[issue]
	if !ok {
		return notFound(issue, "synthesize issue %q not found", issue)
	}
	if n.Kind != Issue {
		return illegal(issue, "synthesize target must be an issue, got %s", n.Kind)
	}
	if len(froms) < 2 {
		return illegal(issue, "synthesize needs at least two --from parent positions (a hybrid of one is just a variant)")
	}
	seen := map[string]bool{}
	for _, f := range froms {
		if seen[f] {
			return illegal(f, "synthesize --from %s given twice", f)
		}
		seen[f] = true
		p, ok := g.Nodes[f]
		if !ok {
			return notFound(f, "synthesize --from position %q not found", f)
		}
		if p.Kind != Position {
			return illegal(f, "synthesize --from must be a position, got %s", p.Kind)
		}
		if !g.respondsTo(f, issue) {
			return illegal(f, "synthesize --from position %s does not respond_to issue %s", f, issue)
		}
	}
	return nil
}

// CanPromote validates tagging a node with the value it promotes: the node
// must be an AF node owned by the author (a value changes the node's fate
// under every audience lens and is otherwise unretractable, so a stranger must
// not be able to stamp it), and not already valued. Changing a value = the
// owner concedes the node and restates it.
func (g *Graph) CanPromote(node, author string) error {
	n, ok := g.Nodes[node]
	if !ok {
		return notFound(node, "promote target %q not found", node)
	}
	if n.Kind != Position && n.Kind != Argument {
		return illegal(node, "promote target must be a position or argument, got %s", n.Kind)
	}
	if n.Author != author {
		return illegal(node, "cannot promote %s: owned by %s, not %s", node, n.Author, author)
	}
	if v, ok := g.Values[node]; ok {
		return illegal(node, "node %s already promotes %q; to change it, concede the node and restate", node, v)
	}
	return nil
}

// CanAudience validates declaring (or superseding) a named value ranking.
func (g *Graph) CanAudience(name string, ranking []string, supersede bool) error {
	if name == "" {
		return illegal("", "audience needs a name")
	}
	if len(ranking) < 2 {
		return illegal(name, "audience %s needs a ranking of at least two values", name)
	}
	seen := map[string]bool{}
	for _, v := range ranking {
		if v == "" {
			return illegal(name, "audience %s ranking contains an empty value", name)
		}
		if seen[v] {
			return illegal(name, "audience %s ranks %q twice (ranking must be a strict order)", name, v)
		}
		seen[v] = true
	}
	_, exists := g.Audiences[name]
	if exists && !supersede {
		return illegal(name, "audience %s is already declared; re-declare with --supersede --basis <label>", name)
	}
	if !exists && supersede {
		return illegal(name, "audience %s does not exist; declare it without --supersede", name)
	}
	return nil
}

// respondsTo reports whether src responds_to dst.
func (g *Graph) respondsTo(src, dst string) bool {
	for _, l := range g.Links {
		if l.Rel == RespondsTo && l.Src == src && l.Dst == dst {
			return true
		}
	}
	return false
}

// CanPropose validates a propose: target must be an existing issue.
func (g *Graph) CanPropose(issue string) error {
	n, ok := g.Nodes[issue]
	if !ok {
		return notFound(issue, "propose target %q not found", issue)
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
		return notFound(target, "%s target %q not found", rel, target)
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
	for _, id := range []string{winner, loser} {
		if _, ok := g.Nodes[id]; !ok {
			return notFound(id, "prefer endpoint %q not found", id)
		}
		if !g.IsAFNode(id) {
			return illegal(id, "prefer endpoints must be positions or arguments, got %s", g.Nodes[id].Kind)
		}
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
		return notFound(issue, "decide issue %q not found", issue)
	}
	if n.Kind != Issue {
		return illegal(issue, "decide issue must be an issue, got %s", n.Kind)
	}
	p, ok := g.Nodes[position]
	if !ok {
		return notFound(position, "decide position %q not found", position)
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
		return notFound(node, "concede target %q not found", node)
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
