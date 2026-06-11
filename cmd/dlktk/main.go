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

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/discover"
	"github.com/chazu/dlktk/internal/fail"
	"github.com/chazu/dlktk/internal/ibis"
	"github.com/chazu/dlktk/internal/id"
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
	flagAsOf    string
	flagValidAt string
)

const currentPointer = ".dlktk/current"

func main() {
	if err := root().Execute(); err != nil {
		e := fail.Classify(err)
		if wantJSON() {
			fmt.Fprintln(os.Stderr, e.JSON())
		} else {
			fmt.Fprintf(os.Stderr, "%s: %s\n", e.ErrKind, e.Detail)
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
	c.PersistentFlags().StringVar(&flagJSON, "format", "", "output format: text|json")
	c.PersistentFlags().StringVar(&flagStore, "store", "", "pudl store dir (default: repo .pudl/ else ~/.pudl)")
	c.PersistentFlags().StringVar(&flagRole, "role", "", "author/role attribution for moves (default: OS user)")
	c.PersistentFlags().StringVar(&flagAsOf, "as-of", "", "transaction-time travel: evaluate as of T (RFC3339 or Unix seconds)")
	c.PersistentFlags().StringVar(&flagValidAt, "valid-at", "", "valid-time: which decisions were in force at T (RFC3339 or Unix seconds)")

	c.AddCommand(
		cmdNew(), cmdUse(), cmdList(),
		cmdRaise(), cmdPropose(), cmdSupport(), cmdObject(), cmdPrefer(), cmdDecide(), cmdSupersede(),
		cmdConcede("concede"), cmdConcede("retract"),
		cmdStatus(), cmdTree(), cmdAgenda(), cmdMoves(), cmdWhy(), cmdExplain(), cmdDiscover(),
		cmdReplay(), cmdLog(),
		cmdExport(), cmdImport(), cmdSchema(), cmdAnchored(),
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

func author() string {
	if flagRole != "" {
		return flagRole
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "unknown"
}

func mover(s *store.Store) *proto.Mover { return proto.New(s, author()) }

func wantJSON() bool { return strings.EqualFold(flagJSON, "json") }

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
// framework, and grounded labels for a read command.
func loadFramework() (*ibis.Graph, *af.Framework, map[string]af.Label, error) {
	disc, err := resolveDisc()
	if err != nil {
		return nil, nil, nil, err
	}
	w, err := when()
	if err != nil {
		return nil, nil, nil, err
	}
	s, err := openStore()
	if err != nil {
		return nil, nil, nil, err
	}
	defer s.Close()
	g, err := s.Graph(disc, w)
	if err != nil {
		return nil, nil, nil, err
	}
	fw, err := af.Build(g)
	if err != nil {
		return nil, nil, nil, err
	}
	return g, fw, fw.Grounded(), nil
}

func cmdAgenda() *cobra.Command {
	return &cobra.Command{
		Use:   "agenda",
		Short: "all UNDEC nodes = the live questions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			g, _, labels, err := loadFramework()
			if err != nil {
				return err
			}
			v := render.Agenda(g, labels)
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

func cmdMoves() *cobra.Command {
	return &cobra.Command{
		Use:   "moves <issue>",
		Short: "legal + useful next moves for an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, fw, labels, err := loadFramework()
			if err != nil {
				return err
			}
			if n, ok := g.Nodes[args[0]]; !ok || n.Kind != ibis.Issue {
				return fail.NotFound(args[0], "issue %q not found", args[0])
			}
			v := render.Moves(g, fw, labels, args[0])
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

func cmdWhy() *cobra.Command {
	return &cobra.Command{
		Use:   "why <node>",
		Short: "explain a node's label and how to flip it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, fw, labels, err := loadFramework()
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
	return &cobra.Command{
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
			recs, err := s.Export(disc)
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

func cmdSchema() *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "emit the pudl/dlktk CUE schema for dlktk/* fact relations",
		Args:  cobra.NoArgs,
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
			if err := s.AddDiscussion(ibis.Discussion{ID: disc, Title: args[0], Subject: subject, CreatedBy: author()}); err != nil {
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

// --- move commands ---

func cmdRaise() *cobra.Command {
	var parent, card string
	c := &cobra.Command{
		Use:   "raise <text>",
		Short: "raise an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch card {
			case "", string(ibis.SelectOne), string(ibis.Open):
			default:
				return fail.New(fail.CodeIllegal, "bad_cardinality", "--card must be %q or %q, got %q", ibis.SelectOne, ibis.Open, card)
			}
			return withMover(func(disc string, m *proto.Mover) error {
				idv, err := m.Raise(disc, args[0], parent, ibis.Cardinality(card))
				if err != nil {
					return err
				}
				emitID("issue", idv)
				return nil
			})
		},
	}
	c.Flags().StringVar(&parent, "parent", "", "parent issue id")
	c.Flags().StringVar(&card, "card", "", "cardinality: select_one (default) or open; fixed at creation")
	return c
}

func cmdPropose() *cobra.Command {
	return &cobra.Command{
		Use:   "propose <issue> <text>",
		Short: "propose a position on an issue",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMover(func(disc string, m *proto.Mover) error {
				idv, err := m.Propose(disc, args[0], args[1])
				if err != nil {
					return err
				}
				emitID("position", idv)
				return nil
			})
		},
	}
}

func cmdSupport() *cobra.Command {
	return &cobra.Command{
		Use:   "support <target> <text>",
		Short: "argue in support of a position or argument",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMover(func(disc string, m *proto.Mover) error {
				idv, err := m.Support(disc, args[0], args[1])
				if err != nil {
					return err
				}
				emitID("argument", idv)
				return nil
			})
		},
	}
}

func cmdObject() *cobra.Command {
	return &cobra.Command{
		Use:   "object <target> <text>",
		Short: "object to a position or argument",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMover(func(disc string, m *proto.Mover) error {
				idv, err := m.Object(disc, args[0], args[1])
				if err != nil {
					return err
				}
				emitID("argument", idv)
				return nil
			})
		},
	}
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
	var basis string
	c := &cobra.Command{
		Use:   "decide <issue> <position>",
		Short: "decide an issue by accepting a position",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMover(func(disc string, m *proto.Mover) error {
				if err := m.Decide(disc, args[0], args[1], basis); err != nil {
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
	return c
}

func cmdSupersede() *cobra.Command {
	var basis string
	c := &cobra.Command{
		Use:   "supersede <issue> <position>",
		Short: "overturn the standing decision on an issue (basis required)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMover(func(disc string, m *proto.Mover) error {
				if err := m.Supersede(disc, args[0], args[1], basis); err != nil {
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
	return c
}

func cmdConcede(name string) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <node>",
		Short: "withdraw one of your own nodes",
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
	return &cobra.Command{
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

			issues, err := targetIssues(g, args)
			if err != nil {
				return err
			}
			var views []render.IssueStatus
			for _, iss := range issues {
				views = append(views, render.Status(g, fw, labels, iss, decs))
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
			fmt.Print(render.Tree(g, issue, opts, fw, labels, decs))
			return nil
		},
	}
	f := c.Flags()
	f.BoolVar(&opts.Labels, "labels", false, "annotate with grounded label (IN/OUT/UNDEC) and ↩ reinstated marks")
	f.BoolVar(&opts.Authors, "authors", false, "show each node's author")
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

func targetIssues(g *ibis.Graph, args []string) ([]string, error) {
	if len(args) == 1 {
		if n, ok := g.Nodes[args[0]]; !ok || n.Kind != ibis.Issue {
			return nil, fail.NotFound(args[0], "issue %q not found", args[0])
		}
		return []string{args[0]}, nil
	}
	var issues []string
	for id, n := range g.Nodes {
		if n.Kind == ibis.Issue {
			issues = append(issues, id)
		}
	}
	// Canonical (proquint) order: same inputs must give byte-identical output
	// (design §8.1); map iteration order must never leak into the envelope.
	sort.Strings(issues)
	return issues, nil
}
