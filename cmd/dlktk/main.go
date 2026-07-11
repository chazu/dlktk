// Command dlktk is a defeasible-argumentation CLI for design dialectics, backed
// by pudl. This is the MVP skeleton (design §15 phase 1): the structural moves,
// status, and tree, with dual text/JSON output.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/check"
	"github.com/chazu/dlktk/internal/discover"
	"github.com/chazu/dlktk/internal/fail"
	"github.com/chazu/dlktk/internal/ibis"
	"github.com/chazu/dlktk/internal/id"
	"github.com/chazu/dlktk/internal/mcpserv"
	"github.com/chazu/dlktk/internal/proto"
	"github.com/chazu/dlktk/internal/render"
	"github.com/chazu/dlktk/internal/store"
)

// global flags
var (
	flagDisc    string
	flagJSON    string
	flagStore   string
	flagRole    string
	flagAuthor  string
	flagAsOf    string
	flagValidAt string
	flagColor   string
)

const currentPointer = ".dlktk/current"

func main() {
	if err := root().Execute(); err != nil {
		e := fail.Classify(err)
		if wantJSON() {
			fmt.Fprintln(os.Stderr, e.JSON())
		} else {
			fmt.Fprintf(os.Stderr, "%s: %s\n", e.ErrKind, e.Detail)
			if h := errorHint(e.ErrKind); h != "" {
				fmt.Fprintln(os.Stderr, "hint: "+h)
			}
		}
		os.Exit(e.Code())
	}
}

func root() *cobra.Command {
	c := &cobra.Command{
		Use:           "dlktk",
		Short:         "defeasible-argumentation tool for design dialectics",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.PersistentFlags().StringVarP(&flagDisc, "discussion", "d", "", "discussion id (else $DLKTK_DISC, else ./.dlktk/current)")
	c.PersistentFlags().StringVar(&flagJSON, "format", "", "output format: text|json (default: text on a terminal, json when piped)")
	c.PersistentFlags().StringVar(&flagStore, "store", "", "pudl store dir (default: repo .pudl/ else ~/.pudl)")
	c.PersistentFlags().StringVar(&flagAuthor, "author", "", "stable identity attributed to moves and checked for ownership (default: OS user)")
	c.PersistentFlags().StringVar(&flagRole, "role", "", "persona a move is made under; auto-records an author↔role roster binding (metadata only)")
	c.PersistentFlags().StringVar(&flagAsOf, "as-of", "", "transaction-time travel: evaluate as of T (RFC3339 or Unix seconds)")
	c.PersistentFlags().StringVar(&flagValidAt, "valid-at", "", "valid-time: which decisions were in force at T (RFC3339 or Unix seconds)")
	c.PersistentFlags().StringVar(&flagColor, "color", "auto", "colorize text output: auto|always|never (auto = on for a terminal, off when piped or NO_COLOR is set)")

	// Configure the text renderers once the flags are parsed: color per
	// --color/NO_COLOR/TTY, and prose wrap width from the terminal.
	c.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		tty := term.IsTerminal(int(os.Stdout.Fd()))
		switch flagColor {
		case "always":
			render.SetColor(true)
		case "never":
			render.SetColor(false)
		default:
			render.SetColor(tty && os.Getenv("NO_COLOR") == "")
		}
		if tty {
			render.SetWidth(termWidth())
		}
		return nil
	}

	c.AddCommand(
		cmdNew(), cmdUse(), cmdList(), cmdRoster(),
		cmdRaise(), cmdReframe(), cmdPropose(), cmdSynthesize(), cmdSupport(), cmdObject(), cmdAssume(),
		cmdPrefer(), cmdPromote(), cmdAudience(), cmdDecide(), cmdSupersede(),
		cmdConcede("concede"), cmdConcede("retract"),
		cmdStatus(), cmdTree(), cmdShow(), cmdSearch(), cmdAgenda(), cmdMoves(), cmdWhy(), cmdExplain(),
		cmdWhatIf(), cmdCrux(), cmdWorlds(), cmdAudiences(), cmdDiscover(),
		cmdReplay(), cmdLog(), cmdCheck(),
		cmdExport(), cmdImport(), cmdSchema(), cmdAnchored(), cmdMCP(),
	)
	return c
}

// parseTime parses an RFC3339 or Unix-seconds string into a pointer (nil empty).
func parseTime(flag, val string) (*int64, error) {
	if val == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, val); err == nil {
		u := t.Unix()
		return &u, nil
	}
	if u, err := strconv.ParseInt(val, 10, 64); err == nil {
		return &u, nil
	}
	return nil, fail.New(fail.CodeGeneric, "bad_time", "%s %q is neither RFC3339 nor Unix seconds", flag, val)
}

// when assembles the temporal viewpoint from --as-of (tt) and --valid-at (vt).
func when() (store.When, error) {
	tx, err := parseTime("--as-of", flagAsOf)
	if err != nil {
		return store.When{}, err
	}
	va, err := parseTime("--valid-at", flagValidAt)
	if err != nil {
		return store.When{}, err
	}
	return store.When{Tx: tx, Valid: va}, nil
}

// --- helpers ---

func openStore() (*store.Store, error) {
	cwd, _ := os.Getwd()
	return store.Open(store.ResolveDir(flagStore, cwd))
}

// authorID is the stable ownership identity attributed to moves: --author if
// given, else the OS user. concede/retract ownership checks ride on this, never
// on the persona (design §2, §16 Q6). Historically --role doubled as identity;
// it no longer does.
func authorID() string {
	if flagAuthor != "" {
		return flagAuthor
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "unknown"
}

// roleOf is the optional persona a move is made under (--role). Empty means no
// persona; a non-empty role auto-records the author↔role roster binding.
func roleOf() string { return flagRole }

// errorHint returns a short next-step suggestion for a failed command, keyed by
// error kind, so a human sees what to try rather than just what went wrong.
func errorHint(kind string) string {
	switch kind {
	case "illegal_move":
		return "see legal moves with `dlktk moves <issue>`, or the full contract with `dlktk discover`"
	case "not_found":
		return "list the discussion's ids with `dlktk tree` (or `dlktk status`); discussions with `dlktk list`"
	case "check_failed":
		return "inspect the drift with `dlktk replay <issue> --diff`; overturn a stale decision with `dlktk supersede`"
	}
	return ""
}

func mover(s *store.Store) *proto.Mover { return proto.New(s, authorID(), roleOf()) }

// wantJSON reports whether output should be JSON. Explicit --format wins;
// otherwise it auto-detects: text on an interactive terminal, JSON when piped
// (design §6.1) so agents reading stdout get structured output for free.
func wantJSON() bool {
	switch strings.ToLower(flagJSON) {
	case "json":
		return true
	case "text":
		return false
	default:
		return !term.IsTerminal(int(os.Stdout.Fd()))
	}
}

// resolveDisc resolves the active discussion: -d, then $DLKTK_DISC, then the
// ./.dlktk/current pointer.
func resolveDisc() (string, error) {
	if flagDisc != "" {
		return flagDisc, nil
	}
	if env := os.Getenv("DLKTK_DISC"); env != "" {
		return env, nil
	}
	b, err := os.ReadFile(currentPointer)
	if err != nil {
		return "", fmt.Errorf("no discussion: pass -d, set $DLKTK_DISC, or `dlktk use <disc>`")
	}
	return strings.TrimSpace(string(b)), nil
}

func setCurrent(disc string) error {
	if err := os.MkdirAll(filepath.Dir(currentPointer), 0o755); err != nil {
		return err
	}
	return os.WriteFile(currentPointer, []byte(disc+"\n"), 0o644)
}

// emit prints id (or a json envelope) for a move result.
func emitID(kind, id string) {
	if wantJSON() {
		out, _ := render.JSON(map[string]string{"id": id})
		fmt.Println(out)
		return
	}
	fmt.Printf("%s %s%s\n", kind, ibis.PrefixFor(kindOf(kind)), id)
}

func kindOf(label string) ibis.Kind {
	switch label {
	case "issue":
		return ibis.Issue
	case "position":
		return ibis.Position
	case "argument":
		return ibis.Argument
	}
	return ""
}

// loadFramework resolves disc + as-of, opens the store, and returns the graph,
// framework, grounded labels, and in-force decisions for a read command.
func loadFramework() (*ibis.Graph, *af.Framework, map[string]af.Label, []ibis.Decision, error) {
	disc, err := resolveDisc()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	w, err := when()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	s, err := openStore()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	defer s.Close()
	g, err := s.Graph(disc, w)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	decs, err := s.Decisions(disc, w)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	fw, err := af.Build(g)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return g, fw, fw.Grounded(), decs, nil
}

func cmdAgenda() *cobra.Command {
	return &cobra.Command{
		Use:   "agenda",
		Short: "the worklist: UNDEC nodes, issues ready to decide, issues with no positions, untested winners, unexamined assumptions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			g, fw, labels, decs, err := loadFramework()
			if err != nil {
				return err
			}
			v := render.Agenda(g, fw, labels, decs)
			if wantJSON() {
				out, _ := render.JSON(v)
				fmt.Println(out)
				return nil
			}
			fmt.Print(render.AgendaText(v))
			return nil
		},
	}
}

func cmdWhatIf() *cobra.Command {
	var objects, prefers, withouts []string
	c := &cobra.Command{
		Use:   "whatif <issue>",
		Short: "counterfactual: apply hypothetical moves in memory and report the label diff (nothing written)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, _, labels, _, err := loadFramework()
			if err != nil {
				return err
			}
			if n, ok := g.Nodes[args[0]]; !ok || n.Kind != ibis.Issue {
				return fail.NotFound(args[0], "issue %q not found", args[0])
			}
			hyps, err := parseHypFlags(objects, prefers, withouts)
			if err != nil {
				return err
			}
			v, err := render.WhatIf(g, labels, args[0], hyps)
			if err != nil {
				return err
			}
			if wantJSON() {
				out, _ := render.JSON(v)
				fmt.Println(out)
				return nil
			}
			fmt.Print(render.WhatIfText(v))
			return nil
		},
	}
	c.Flags().StringArrayVar(&objects, "object", nil, "hypothetically object to this node (repeatable)")
	c.Flags().StringArrayVar(&prefers, "prefer", nil, "hypothetical preference winner:loser (repeatable)")
	c.Flags().StringArrayVar(&withouts, "without", nil, "hypothetically remove this node — a simulated concede (repeatable)")
	return c
}

// parseHypFlags assembles hypotheticals from the repeatable whatif flags.
func parseHypFlags(objects, prefers, withouts []string) ([]render.Hypothetical, error) {
	var hyps []render.Hypothetical
	for _, t := range objects {
		hyps = append(hyps, render.HypObject(t))
	}
	for _, p := range prefers {
		w, l, found := strings.Cut(p, ":")
		if !found || w == "" || l == "" {
			return nil, fail.New(fail.CodeIllegal, "bad_args", "--prefer %q must be winner:loser", p)
		}
		hyps = append(hyps, render.HypPrefer(w, l))
	}
	for _, n := range withouts {
		hyps = append(hyps, render.HypWithout(n))
	}
	if len(hyps) == 0 {
		return nil, fail.New(fail.CodeIllegal, "bad_args", "whatif needs at least one hypothetical: --object, --prefer, or --without")
	}
	return hyps, nil
}

func cmdCrux() *cobra.Command {
	return &cobra.Command{
		Use:   "crux <issue>",
		Short: "the load-bearing arguments: which single argument's removal flips a position",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, _, labels, _, err := loadFramework()
			if err != nil {
				return err
			}
			if n, ok := g.Nodes[args[0]]; !ok || n.Kind != ibis.Issue {
				return fail.NotFound(args[0], "issue %q not found", args[0])
			}
			v, err := render.Crux(g, labels, args[0])
			if err != nil {
				return err
			}
			if wantJSON() {
				out, _ := render.JSON(v)
				fmt.Println(out)
				return nil
			}
			fmt.Print(render.CruxText(v))
			return nil
		},
	}
}

func cmdWorlds() *cobra.Command {
	var under string
	c := &cobra.Command{
		Use:   "worlds <issue>",
		Short: "the coherent maximal stances (preferred extensions) a contested issue admits",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, fw, _, _, err := loadFramework()
			if err != nil {
				return err
			}
			if n, ok := g.Nodes[args[0]]; !ok || n.Kind != ibis.Issue {
				return fail.NotFound(args[0], "issue %q not found", args[0])
			}
			if under != "" {
				if fw, _, err = frameworkUnder(g, under); err != nil {
					return err
				}
			}
			v := render.Worlds(g, fw, args[0])
			v.Under = under
			if wantJSON() {
				out, _ := render.JSON(v)
				fmt.Println(out)
				return nil
			}
			fmt.Print(render.WorldsText(v))
			return nil
		},
	}
	c.Flags().StringVar(&under, "under", "", "audience name: enumerate under that value ranking")
	return c
}

// frameworkUnder rebuilds the framework and labelling under a named audience.
func frameworkUnder(g *ibis.Graph, name string) (*af.Framework, map[string]af.Label, error) {
	aud, ok := g.Audiences[name]
	if !ok {
		return nil, nil, fail.NotFound(name, "audience %q not declared", name)
	}
	fw, err := af.BuildUnder(g, aud)
	if err != nil {
		return nil, nil, err
	}
	return fw, fw.Grounded(), nil
}

func cmdAudiences() *cobra.Command {
	return &cobra.Command{
		Use:   "audiences",
		Short: "cross-audience sensitivity: which positions are justified under every declared value ranking",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			disc, err := resolveDisc()
			if err != nil {
				return err
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			w, err := when()
			if err != nil {
				return err
			}
			g, err := s.Graph(disc, w)
			if err != nil {
				return err
			}
			v, err := render.Audiences(g)
			if err != nil {
				return err
			}
			if wantJSON() {
				out, _ := render.JSON(v)
				fmt.Println(out)
				return nil
			}
			fmt.Print(render.AudiencesText(v))
			return nil
		},
	}
}

func cmdMoves() *cobra.Command {
	return &cobra.Command{
		Use:   "moves <issue>",
		Short: "legal + useful next moves for an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, fw, labels, decs, err := loadFramework()
			if err != nil {
				return err
			}
			if n, ok := g.Nodes[args[0]]; !ok || n.Kind != ibis.Issue {
				return fail.NotFound(args[0], "issue %q not found", args[0])
			}
			v := render.Moves(g, fw, labels, args[0], decs)
			if wantJSON() {
				out, _ := render.JSON(v)
				fmt.Println(out)
				return nil
			}
			fmt.Print(render.MovesText(v))
			return nil
		},
	}
}

func cmdSearch() *cobra.Command {
	var all bool
	c := &cobra.Command{
		Use:   "search <query>",
		Short: "find nodes whose text matches (check for an existing argument before duplicating it)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			w, err := when()
			if err != nil {
				return err
			}
			var discs []string
			if all {
				ds, err := s.Discussions(w)
				if err != nil {
					return err
				}
				for _, d := range ds {
					discs = append(discs, d.ID)
				}
				sort.Strings(discs)
			} else {
				disc, err := resolveDisc()
				if err != nil {
					return err
				}
				discs = []string{disc}
			}
			v := render.SearchView{Query: args[0]}
			for _, disc := range discs {
				g, err := s.Graph(disc, w)
				if err != nil {
					return err
				}
				var labels map[string]af.Label
				if fw, err := af.Build(g); err == nil { // unlabellable graphs still searchable
					labels = fw.Grounded()
				}
				v.Hits = append(v.Hits, render.Search(disc, g, labels, args[0])...)
			}
			if wantJSON() {
				out, _ := render.JSON(v)
				fmt.Println(out)
				return nil
			}
			fmt.Print(render.SearchText(v))
			return nil
		},
	}
	c.Flags().BoolVar(&all, "all", false, "search every discussion in the store (else the current one)")
	return c
}

func cmdShow() *cobra.Command {
	return &cobra.Command{
		Use:   "show <node>",
		Short: "one node in full: text, author, label, and every incident link",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, _, labels, decs, err := loadFramework()
			if err != nil {
				return err
			}
			if _, ok := g.Nodes[args[0]]; !ok {
				return fail.NotFound(args[0], "node %q not found", args[0])
			}
			v := render.Show(g, labels, args[0], decs)
			if wantJSON() {
				out, _ := render.JSON(v)
				fmt.Println(out)
				return nil
			}
			fmt.Print(render.ShowText(v))
			return nil
		},
	}
}

func cmdWhy() *cobra.Command {
	return &cobra.Command{
		Use:   "why <node>",
		Short: "explain a node's label and how to flip it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, fw, labels, _, err := loadFramework()
			if err != nil {
				return err
			}
			if !g.IsAFNode(args[0]) {
				return fail.NotFound(args[0], "%q is not a position or argument", args[0])
			}
			v := render.Why(g, fw, labels, args[0])
			if wantJSON() {
				out, _ := render.JSON(v)
				fmt.Println(out)
				return nil
			}
			fmt.Print(render.WhyText(v))
			return nil
		},
	}
}

func cmdExplain() *cobra.Command {
	var brief bool
	var under string
	c := &cobra.Command{
		Use:   "explain <issue>",
		Short: "trace how an issue's labelling was derived (the automated reasoning)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			disc, err := resolveDisc()
			if err != nil {
				return err
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			w, err := when()
			if err != nil {
				return err
			}
			g, err := s.Graph(disc, w)
			if err != nil {
				return err
			}
			if n, ok := g.Nodes[args[0]]; !ok || n.Kind != ibis.Issue {
				return fail.NotFound(args[0], "issue %q not found", args[0])
			}
			decs, err := s.Decisions(disc, w)
			if err != nil {
				return err
			}
			fw, err := af.Build(g)
			if err != nil {
				return err
			}
			if under != "" {
				if fw, _, err = frameworkUnder(g, under); err != nil {
					return err
				}
			}
			v := render.Explain(g, fw, args[0], decs)
			if wantJSON() {
				out, _ := render.JSON(v)
				fmt.Println(out)
				return nil
			}
			fmt.Print(render.ExplainText(v, brief))
			return nil
		},
	}
	c.Flags().BoolVar(&brief, "brief", false, "omit the conceptual primer; show only the trace and outcome")
	c.Flags().StringVar(&under, "under", "", "audience name: derive under that value ranking")
	return c
}

func cmdReplay() *cobra.Command {
	var diff bool
	c := &cobra.Command{
		Use:   "replay [issue]",
		Short: "labelling at --as-of T (and what changed since, with --diff)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagAsOf == "" {
				return fail.New(fail.CodeGeneric, "missing_as_of", "replay requires --as-of T")
			}
			disc, err := resolveDisc()
			if err != nil {
				return err
			}
			tx, err := parseTime("--as-of", flagAsOf)
			if err != nil {
				return err
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()

			gThen, err := s.Graph(disc, store.When{Tx: tx})
			if err != nil {
				return err
			}
			fwThen, err := af.Build(gThen)
			if err != nil {
				return err
			}
			lThen := fwThen.Grounded()

			if diff {
				gNow, err := s.Graph(disc, store.Now())
				if err != nil {
					return err
				}
				fwNow, err := af.Build(gNow)
				if err != nil {
					return err
				}
				v := render.Diff(flagAsOf, gThen, gNow, lThen, fwNow.Grounded())
				if wantJSON() {
					out, _ := render.JSON(v)
					fmt.Println(out)
					return nil
				}
				fmt.Print(render.DiffText(v))
				return nil
			}

			// No --diff: the grounded labelling as it stood at T.
			decs, err := s.Decisions(disc, store.When{Tx: tx})
			if err != nil {
				return err
			}
			issues, err := targetIssues(gThen, args)
			if err != nil {
				return err
			}
			var views []render.IssueStatus
			for _, iss := range issues {
				views = append(views, render.Status(gThen, fwThen, lThen, iss, decs))
			}
			if wantJSON() {
				out, _ := render.JSON(views)
				fmt.Println(out)
				return nil
			}
			for _, v := range views {
				fmt.Print(render.StatusText(v))
			}
			return nil
		},
	}
	c.Flags().BoolVar(&diff, "diff", false, "diff the as-of labelling against now")
	return c
}

func cmdCheck() *cobra.Command {
	var all, strict bool
	c := &cobra.Command{
		Use:   "check",
		Short: "verify standing decisions: drift, stalemates, store invariants (CI-friendly, exit 5 on findings)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			w, err := when()
			if err != nil {
				return err
			}
			var discs []string
			if all {
				ds, err := s.Discussions(w)
				if err != nil {
					return err
				}
				for _, d := range ds {
					discs = append(discs, d.ID)
				}
			} else {
				disc, err := resolveDisc()
				if err != nil {
					return err
				}
				discs = []string{disc}
			}
			// Review horizons are judged against the --as-of time when
			// travelling, the wall clock otherwise.
			now := time.Now().Unix()
			if w.Tx != nil {
				now = *w.Tx
			}
			v, err := check.Run(s, discs, w, now)
			if err != nil {
				return err
			}
			if wantJSON() {
				out, _ := render.JSON(v)
				fmt.Println(out)
			} else {
				fmt.Print(check.Text(v))
			}
			errs, warns := 0, 0
			for _, f := range v.Findings {
				if f.Severity == "error" {
					errs++
				} else {
					warns++
				}
			}
			if !v.OK {
				return fail.New(fail.CodeCheck, "check_failed", "%d error finding(s), %d warning(s)", errs, warns)
			}
			if strict && warns > 0 {
				return fail.New(fail.CodeCheck, "check_failed", "%d warning(s) under --strict", warns)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&all, "all", false, "check every discussion in the store (else the current one)")
	c.Flags().BoolVar(&strict, "strict", false, "fail on warnings (stalemates) too")
	return c
}

func cmdLog() *cobra.Command {
	return &cobra.Command{
		Use:   "log [node]",
		Short: "transaction-time history (audit trail), optionally for one node",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			disc, err := resolveDisc()
			if err != nil {
				return err
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			node := ""
			if len(args) == 1 {
				node = args[0]
			}
			entries, err := s.History(disc, node)
			if err != nil {
				return err
			}
			if wantJSON() {
				out, _ := render.JSON(entries)
				fmt.Println(out)
				return nil
			}
			fmt.Print(render.LogText(entries))
			return nil
		},
	}
}

func cmdExport() *cobra.Command {
	var history bool
	c := &cobra.Command{
		Use:   "export",
		Short: "dump the discussion's facts as NDJSON (git-native, re-importable)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			disc, err := resolveDisc()
			if err != nil {
				return err
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			var recs []store.ExportRecord
			if history {
				recs, err = s.ExportHistory(disc)
			} else {
				recs, err = s.Export(disc)
			}
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout) // NDJSON: one compact object per line
			for _, r := range recs {
				if err := enc.Encode(r); err != nil {
					return err
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&history, "history", false, "full tt history: asserts + retract/invalidate events (replayable audit trail)")
	return c
}

func cmdImport() *cobra.Command {
	return &cobra.Command{
		Use:   "import <file>",
		Short: "load an NDJSON move log (idempotent by content)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := os.Open(args[0])
			if err != nil {
				return fail.NotFound(args[0], "cannot open %s: %v", args[0], err)
			}
			defer f.Close()
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()

			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
			var recs []store.ExportRecord
			line := 0
			for sc.Scan() {
				line++
				text := strings.TrimSpace(sc.Text())
				if text == "" {
					continue
				}
				var rec store.ExportRecord
				if err := json.Unmarshal([]byte(text), &rec); err != nil {
					return fail.New(fail.CodeGeneric, "bad_ndjson", "line %d: %v", line, err)
				}
				recs = append(recs, rec)
			}
			if err := sc.Err(); err != nil {
				return err
			}
			// Validate the whole batch (shape + invariants) before writing any
			// of it; import is a write path and must not bypass move legality.
			n, err := s.ImportAll(recs)
			if err != nil {
				return err
			}
			if !wantJSON() {
				fmt.Printf("imported %d facts\n", n)
			} else {
				out, _ := render.JSON(map[string]int{"imported": n})
				fmt.Println(out)
			}
			return nil
		},
	}
}

func cmdMCP() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "serve dlktk over the Model Context Protocol (stdio)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			return mcpserv.Serve(cmd.Context(), s, authorID(), roleOf(), &mcp.StdioTransport{})
		},
	}
}

func cmdSchema() *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "emit the pudl/dlktk CUE schema for dlktk/* fact relations",
		Long: `Emit the pudl/dlktk CUE schema package: the typed args shape of every
dlktk/* fact relation.

pudl >= v0.1.3 ships this package as a built-in bootstrap schema, registered
automatically on pudl init/import — no manual install needed. On older pudl
versions, write the output under the pudl schema dir (pudl/dlktk/dlktk.cue)
to register it. The two copies are maintained in lockstep; this command is
the reference for inspection and for keeping pudl's copy in sync.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Print(discover.CUESchema())
			return nil
		},
	}
}

func cmdAnchored() *cobra.Command {
	return &cobra.Command{
		Use:   "anchored <subject-substring>",
		Short: "find discussions whose subject anchors to a code artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			ds, err := s.Discussions(store.Now())
			if err != nil {
				return err
			}
			var hits []ibis.Discussion
			for _, d := range ds {
				if d.Subject != "" && strings.Contains(d.Subject, args[0]) {
					hits = append(hits, d)
				}
			}
			if wantJSON() {
				out, _ := render.JSON(hits)
				fmt.Println(out)
				return nil
			}
			for _, d := range hits {
				fmt.Printf("%s  %q  %s\n", d.ID, d.Title, d.Subject)
			}
			if len(hits) == 0 {
				fmt.Println("no discussions anchored to that subject")
			}
			return nil
		},
	}
}

func cmdDiscover() *cobra.Command {
	return &cobra.Command{
		Use:   "discover",
		Short: "machine-readable capability schema (CUE; JSON with --format json)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if wantJSON() {
				out, err := discover.JSON()
				if err != nil {
					return err
				}
				fmt.Println(out)
				return nil
			}
			fmt.Print(discover.CUE())
			return nil
		},
	}
}

// --- discussion commands ---

func cmdNew() *cobra.Command {
	var subject string
	c := &cobra.Command{
		Use:   "new <title>",
		Short: "create a discussion and set it current",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			disc := id.New()
			if err := s.AddDiscussion(ibis.Discussion{ID: disc, Title: args[0], Subject: subject, CreatedBy: authorID()}); err != nil {
				return err
			}
			if err := setCurrent(disc); err != nil {
				return err
			}
			emitID("discussion", disc)
			return nil
		},
	}
	c.Flags().StringVar(&subject, "subject", "", "subject ref (file:… pkg:… commit:… q:…)")
	return c
}

func cmdUse() *cobra.Command {
	return &cobra.Command{
		Use:   "use <disc>",
		Short: "set the current discussion",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return setCurrent(args[0])
		},
	}
}

func cmdList() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list discussions",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			ds, err := s.Discussions(store.Now())
			if err != nil {
				return err
			}
			if wantJSON() {
				out, _ := render.JSON(ds)
				fmt.Println(out)
				return nil
			}
			for _, d := range ds {
				fmt.Printf("%s  %q  %s\n", d.ID, d.Title, d.Subject)
			}
			return nil
		},
	}
}

// RosterView is the roster read envelope.
type RosterView struct {
	Discussion string        `json:"discussion"`
	Bindings   []ibis.Roster `json:"bindings"`
}

func cmdRoster() *cobra.Command {
	return &cobra.Command{
		Use:   "roster [<author> <role>]",
		Short: "list author↔role bindings, or pre-declare one (auto-recorded by moves otherwise)",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return fail.New(fail.CodeIllegal, "bad_args", "roster takes either no args (list) or <author> <role> (bind)")
			}
			disc, err := resolveDisc()
			if err != nil {
				return err
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			if len(args) == 2 {
				if err := s.AddRoster(ibis.Roster{Disc: disc, Author: args[0], Role: args[1]}); err != nil {
					return err
				}
				if wantJSON() {
					out, _ := render.JSON(ibis.Roster{Disc: disc, Author: args[0], Role: args[1]})
					fmt.Println(out)
				} else {
					fmt.Printf("bound %s -> %s\n", args[0], args[1])
				}
				return nil
			}
			w, err := when()
			if err != nil {
				return err
			}
			rs, err := s.Rosters(disc, w)
			if err != nil {
				return err
			}
			if wantJSON() {
				out, _ := render.JSON(RosterView{Discussion: disc, Bindings: rs})
				fmt.Println(out)
				return nil
			}
			for _, r := range rs {
				fmt.Printf("%s  %s\n", r.Author, r.Role)
			}
			return nil
		},
	}
}

// --- move commands ---

// validCard rejects a bad --card value before the move layer sees it.
func validCard(card string) error {
	switch card {
	case "", string(ibis.SelectOne), string(ibis.Open):
		return nil
	}
	return fail.New(fail.CodeIllegal, "bad_cardinality", "--card must be %q or %q, got %q", ibis.SelectOne, ibis.Open, card)
}

func cmdRaise() *cobra.Command {
	var parent, from, card string
	c := &cobra.Command{
		Use:   "raise <text>",
		Short: "raise an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validCard(card); err != nil {
				return err
			}
			return withMover(func(disc string, m *proto.Mover) error {
				idv, err := m.Raise(disc, args[0], parent, from, ibis.Cardinality(card))
				if err != nil {
					return err
				}
				emitID("issue", idv)
				return nil
			})
		},
	}
	c.Flags().StringVar(&parent, "parent", "", "parent issue id")
	c.Flags().StringVar(&from, "from", "", "the position/argument that revealed this question (records provenance; mutually exclusive with --parent)")
	c.Flags().StringVar(&card, "card", "", "cardinality: select_one (default) or open; fixed at creation")
	return c
}

func cmdReframe() *cobra.Command {
	var basis, card string
	c := &cobra.Command{
		Use:   "reframe <issue> <text>",
		Short: "replace an issue's framing with a fresh issue (basis required; positions do not carry over)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validCard(card); err != nil {
				return err
			}
			return withMover(func(disc string, m *proto.Mover) error {
				idv, err := m.Reframe(disc, args[0], args[1], basis, ibis.Cardinality(card))
				if err != nil {
					return err
				}
				emitID("issue", idv)
				return nil
			})
		},
	}
	c.Flags().StringVar(&basis, "basis", "", "why the framing is replaced (required)")
	c.Flags().StringVar(&card, "card", "", "cardinality of the new issue: select_one (default) or open")
	return c
}

func cmdPropose() *cobra.Command {
	var promotes string
	c := &cobra.Command{
		Use:   "propose <issue> <text>",
		Short: "propose a position on an issue",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMover(func(disc string, m *proto.Mover) error {
				idv, err := m.Propose(disc, args[0], args[1], promotes)
				if err != nil {
					return err
				}
				emitID("position", idv)
				return nil
			})
		},
	}
	c.Flags().StringVar(&promotes, "promotes", "", "the value this position promotes (audience lens input)")
	return c
}

func cmdSynthesize() *cobra.Command {
	var froms []string
	var promotes string
	c := &cobra.Command{
		Use:   "synthesize <issue> <text>",
		Short: "propose a hybrid position recombining two or more existing positions (lineage recorded)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMover(func(disc string, m *proto.Mover) error {
				idv, err := m.Synthesize(disc, args[0], args[1], froms, promotes)
				if err != nil {
					return err
				}
				emitID("position", idv)
				return nil
			})
		},
	}
	c.Flags().StringArrayVar(&froms, "from", nil, "parent position id (repeat; at least two)")
	c.Flags().StringVar(&promotes, "promotes", "", "the value the hybrid promotes")
	return c
}

func cmdSupport() *cobra.Command {
	var promotes string
	c := &cobra.Command{
		Use:   "support <target> <text>",
		Short: "argue in support of a position or argument",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMover(func(disc string, m *proto.Mover) error {
				idv, err := m.Support(disc, args[0], args[1], promotes)
				if err != nil {
					return err
				}
				emitID("argument", idv)
				return nil
			})
		},
	}
	c.Flags().StringVar(&promotes, "promotes", "", "the value this argument promotes (audience lens input)")
	return c
}

func cmdObject() *cobra.Command {
	var promotes string
	c := &cobra.Command{
		Use:   "object <target> <text>",
		Short: "object to a position or argument",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMover(func(disc string, m *proto.Mover) error {
				idv, err := m.Object(disc, args[0], args[1], promotes)
				if err != nil {
					return err
				}
				emitID("argument", idv)
				return nil
			})
		},
	}
	c.Flags().StringVar(&promotes, "promotes", "", "the value this argument promotes (audience lens input)")
	return c
}

func cmdAssume() *cobra.Command {
	return &cobra.Command{
		Use:   "assume <target> <text>",
		Short: "record an assumption the target rests on (challengeable premise; agenda tracks unexamined ones)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMover(func(disc string, m *proto.Mover) error {
				idv, err := m.Assume(disc, args[0], args[1])
				if err != nil {
					return err
				}
				emitID("argument", idv)
				return nil
			})
		},
	}
}

func cmdPromote() *cobra.Command {
	return &cobra.Command{
		Use:   "promote <node> <value>",
		Short: "tag one of your own nodes with the value it promotes (audience lens input)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMover(func(disc string, m *proto.Mover) error {
				if err := m.Promote(disc, args[0], args[1]); err != nil {
					return err
				}
				if wantJSON() {
					out, _ := render.JSON(map[string]string{"node": args[0], "value": args[1]})
					fmt.Println(out)
				} else {
					fmt.Printf("%s promotes %s\n", args[0], args[1])
				}
				return nil
			})
		},
	}
}

func cmdAudience() *cobra.Command {
	var supersede bool
	var basis string
	c := &cobra.Command{
		Use:   "audience [<name> <value>...]",
		Short: "declare a named strict value ranking (most important first), or list declared audiences",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				// List the declared audiences.
				disc, err := resolveDisc()
				if err != nil {
					return err
				}
				s, err := openStore()
				if err != nil {
					return err
				}
				defer s.Close()
				w, err := when()
				if err != nil {
					return err
				}
				auds, err := s.Audiences(disc, w)
				if err != nil {
					return err
				}
				sort.Slice(auds, func(i, j int) bool { return auds[i].Name < auds[j].Name })
				if wantJSON() {
					out, _ := render.JSON(auds)
					fmt.Println(out)
					return nil
				}
				for _, a := range auds {
					fmt.Printf("%s  %s\n", a.Name, strings.Join(a.Ranking, " > "))
				}
				if len(auds) == 0 {
					fmt.Println("no audiences declared")
				}
				return nil
			}
			if len(args) < 3 {
				return fail.New(fail.CodeIllegal, "bad_args", "audience takes <name> followed by at least two values (most important first)")
			}
			return withMover(func(disc string, m *proto.Mover) error {
				if err := m.DeclareAudience(disc, args[0], args[1:], supersede, basis); err != nil {
					return err
				}
				if wantJSON() {
					out, _ := render.JSON(map[string]any{"name": args[0], "ranking": args[1:]})
					fmt.Println(out)
				} else {
					fmt.Printf("audience %s: %s\n", args[0], strings.Join(args[1:], " > "))
				}
				return nil
			})
		},
	}
	c.Flags().BoolVar(&supersede, "supersede", false, "replace an existing declaration of this name (requires --basis)")
	c.Flags().StringVar(&basis, "basis", "", "why the prior ranking is retired (required with --supersede)")
	return c
}

func cmdPrefer() *cobra.Command {
	var basis string
	c := &cobra.Command{
		Use:   "prefer <winner> <loser>",
		Short: "state a preference (winner over loser)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMover(func(disc string, m *proto.Mover) error {
				idv, err := m.Prefer(disc, args[0], args[1], basis)
				if err != nil {
					return err
				}
				emitID("preference", idv)
				return nil
			})
		},
	}
	c.Flags().StringVar(&basis, "basis", "", "basis label (security, velocity, …)")
	return c
}

func cmdDecide() *cobra.Command {
	var basis, reviewBy string
	c := &cobra.Command{
		Use:   "decide <issue> <position>",
		Short: "decide an issue by accepting a position",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			rb, err := parseReviewBy(reviewBy)
			if err != nil {
				return err
			}
			return withMover(func(disc string, m *proto.Mover) error {
				if err := m.Decide(disc, args[0], args[1], basis, rb); err != nil {
					return err
				}
				if !wantJSON() {
					fmt.Printf("decided %s -> %s\n", args[0], args[1])
				} else {
					out, _ := render.JSON(map[string]string{"issue": args[0], "position": args[1]})
					fmt.Println(out)
				}
				return nil
			})
		},
	}
	c.Flags().StringVar(&basis, "basis", "", "basis label")
	c.Flags().StringVar(&reviewBy, "review-by", "", "re-examination horizon (RFC3339 or Unix seconds); check reports the decision once it passes")
	return c
}

// parseReviewBy parses the --review-by flag (empty = none).
func parseReviewBy(val string) (int64, error) {
	t, err := parseTime("--review-by", val)
	if err != nil {
		return 0, err
	}
	if t == nil {
		return 0, nil
	}
	return *t, nil
}

func cmdSupersede() *cobra.Command {
	var basis, reviewBy string
	c := &cobra.Command{
		Use:   "supersede <issue> <position>",
		Short: "overturn the standing decision on an issue (basis required); same position + new --review-by re-arms a review horizon",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			rb, err := parseReviewBy(reviewBy)
			if err != nil {
				return err
			}
			return withMover(func(disc string, m *proto.Mover) error {
				if err := m.Supersede(disc, args[0], args[1], basis, rb); err != nil {
					return err
				}
				if !wantJSON() {
					fmt.Printf("superseded: %s -> %s\n", args[0], args[1])
				} else {
					out, _ := render.JSON(map[string]string{"issue": args[0], "position": args[1]})
					fmt.Println(out)
				}
				return nil
			})
		},
	}
	c.Flags().StringVar(&basis, "basis", "", "why the prior decision is overturned (required)")
	c.Flags().StringVar(&reviewBy, "review-by", "", "re-examination horizon (RFC3339 or Unix seconds)")
	return c
}

func cmdConcede(name string) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <node>",
		Short: "withdraw one of your own nodes (ownership checked against --author, not --role)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMover(func(disc string, m *proto.Mover) error {
				if err := m.Concede(disc, args[0]); err != nil {
					return err
				}
				if !wantJSON() {
					fmt.Printf("conceded %s\n", args[0])
				}
				return nil
			})
		},
	}
}

// --- read commands ---

func cmdStatus() *cobra.Command {
	var under string
	c := &cobra.Command{
		Use:   "status [issue]",
		Short: "grounded labelling of positions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			disc, err := resolveDisc()
			if err != nil {
				return err
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			w, err := when()
			if err != nil {
				return err
			}
			g, err := s.Graph(disc, w)
			if err != nil {
				return err
			}
			decs, err := s.Decisions(disc, w)
			if err != nil {
				return err
			}
			fw, err := af.Build(g)
			if err != nil {
				return err
			}
			labels := fw.Grounded()
			if under != "" {
				if fw, labels, err = frameworkUnder(g, under); err != nil {
					return err
				}
			}

			issues, err := targetIssues(g, args)
			if err != nil {
				return err
			}
			var views []render.IssueStatus
			for _, iss := range issues {
				st := render.Status(g, fw, labels, iss, decs)
				st.Under = under
				views = append(views, st)
			}
			if wantJSON() {
				out, _ := render.JSON(views)
				fmt.Println(out)
				return nil
			}
			for _, v := range views {
				fmt.Print(render.StatusText(v))
			}
			if len(views) == 0 {
				fmt.Println("no issues yet")
			}
			return nil
		},
	}
	c.Flags().StringVar(&under, "under", "", "audience name: evaluate under that value ranking")
	return c
}

func cmdTree() *cobra.Command {
	var opts render.TreeOpts
	c := &cobra.Command{
		Use:   "tree [issue]",
		Short: "the IBIS graph, indented",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			disc, err := resolveDisc()
			if err != nil {
				return err
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			w, err := when()
			if err != nil {
				return err
			}
			g, err := s.Graph(disc, w)
			if err != nil {
				return err
			}
			issue := ""
			if len(args) == 1 {
				if n, ok := g.Nodes[args[0]]; !ok || n.Kind != ibis.Issue {
					return fail.NotFound(args[0], "issue %q not found", args[0])
				}
				issue = args[0]
			}
			if opts.Width == 0 {
				opts.Width = termWidth()
			}
			// Decisions are always shown (the ★/decided marker); the grounded
			// labelling is only computed for the --labels view.
			decs, err := s.Decisions(disc, w)
			if err != nil {
				return err
			}
			var fw *af.Framework
			var labels map[string]af.Label
			if opts.Labels {
				if fw, err = af.Build(g); err != nil {
					return err
				}
				labels = fw.Grounded()
			}
			rosters, err := s.Rosters(disc, w)
			if err != nil {
				return err
			}
			fmt.Print(render.Tree(g, issue, opts, fw, labels, decs, rosters))
			return nil
		},
	}
	f := c.Flags()
	f.BoolVar(&opts.Labels, "labels", false, "annotate with grounded label (IN/OUT/UNDEC) and ↩ reinstated marks")
	f.BoolVar(&opts.Authors, "authors", false, "also show each participant's stable author id alongside their role")
	f.BoolVar(&opts.NoWho, "no-who", false, "omit participant attribution (role/identity) from each node")
	f.BoolVar(&opts.ASCII, "ascii", false, "ASCII connectors/glyphs instead of Unicode")
	f.BoolVar(&opts.NoWrap, "no-wrap", false, "one truncated line per node (dense overview) instead of wrapping full text")
	f.BoolVar(&opts.NoIDs, "no-ids", false, "omit node id suffixes")
	f.BoolVar(&opts.NoLegend, "no-legend", false, "suppress the glyph legend header")
	f.IntVar(&opts.Width, "width", 0, "max line width (0 = autodetect terminal)")
	return c
}

// termWidth reports the terminal width for text wrapping. It queries the
// controlling tty (stdout) directly — interactive shells usually do not export
// $COLUMNS to children — then falls back to $COLUMNS, then 100.
func termWidth() int {
	if ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ); err == nil && ws.Col > 0 {
		return int(ws.Col)
	}
	if c := os.Getenv("COLUMNS"); c != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(c)); err == nil && n > 0 {
			return n
		}
	}
	return 100
}

// withMover resolves the discussion, opens the store, and runs fn with a Mover.
func withMover(fn func(disc string, m *proto.Mover) error) error {
	disc, err := resolveDisc()
	if err != nil {
		return err
	}
	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()
	return fn(disc, mover(s))
}

// targetIssues resolves the issue argument, or — in the all-issues sweep —
// every live issue: reframed (dead) framings are excluded, though naming one
// explicitly still works.
func targetIssues(g *ibis.Graph, args []string) ([]string, error) {
	if len(args) == 1 {
		if n, ok := g.Nodes[args[0]]; !ok || n.Kind != ibis.Issue {
			return nil, fail.NotFound(args[0], "issue %q not found", args[0])
		}
		return []string{args[0]}, nil
	}
	var out []string
	for _, iss := range g.Issues() {
		if _, reframed := g.ReframedTo(iss); !reframed {
			out = append(out, iss)
		}
	}
	return out, nil
}
