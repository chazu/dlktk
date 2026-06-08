// Command dlktk is a defeasible-argumentation CLI for design dialectics, backed
// by pudl. This is the MVP skeleton (design §15 phase 1): the structural moves,
// status, and tree, with dual text/JSON output.
package main

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

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
	flagDisc  string
	flagJSON  string
	flagStore string
	flagRole  string
	flagAsOf  string
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

	c.AddCommand(
		cmdNew(), cmdUse(), cmdList(),
		cmdRaise(), cmdPropose(), cmdSupport(), cmdObject(), cmdPrefer(), cmdDecide(),
		cmdConcede("concede"), cmdConcede("retract"),
		cmdStatus(), cmdTree(), cmdAgenda(), cmdMoves(), cmdWhy(), cmdDiscover(),
	)
	return c
}

// asOf parses the --as-of flag into a Unix-seconds pointer (nil = now).
func asOf() (*int64, error) {
	if flagAsOf == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, flagAsOf); err == nil {
		u := t.Unix()
		return &u, nil
	}
	if u, err := strconv.ParseInt(flagAsOf, 10, 64); err == nil {
		return &u, nil
	}
	return nil, fail.New(fail.CodeGeneric, "bad_as_of", "--as-of %q is neither RFC3339 nor Unix seconds", flagAsOf)
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
	ao, err := asOf()
	if err != nil {
		return nil, nil, nil, err
	}
	s, err := openStore()
	if err != nil {
		return nil, nil, nil, err
	}
	defer s.Close()
	g, err := s.Graph(disc, ao)
	if err != nil {
		return nil, nil, nil, err
	}
	fw := af.Build(g)
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
			ds, err := s.Discussions(nil)
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
	var parent string
	c := &cobra.Command{
		Use:   "raise <text>",
		Short: "raise an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMover(func(disc string, m *proto.Mover) error {
				idv, err := m.Raise(disc, args[0], parent)
				if err != nil {
					return err
				}
				emitID("issue", idv)
				return nil
			})
		},
	}
	c.Flags().StringVar(&parent, "parent", "", "parent issue id")
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
			ao, err := asOf()
			if err != nil {
				return err
			}
			g, err := s.Graph(disc, ao)
			if err != nil {
				return err
			}
			fw := af.Build(g)
			labels := fw.Grounded()

			issues := targetIssues(g, args)
			var views []render.IssueStatus
			for _, iss := range issues {
				views = append(views, render.Status(g, fw, labels, iss))
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
	return &cobra.Command{
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
			ao, err := asOf()
			if err != nil {
				return err
			}
			g, err := s.Graph(disc, ao)
			if err != nil {
				return err
			}
			issue := ""
			if len(args) == 1 {
				issue = args[0]
			}
			fmt.Print(render.Tree(g, issue))
			return nil
		},
	}
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

func targetIssues(g *ibis.Graph, args []string) []string {
	if len(args) == 1 {
		return []string{args[0]}
	}
	var issues []string
	for id, n := range g.Nodes {
		if n.Kind == ibis.Issue {
			issues = append(issues, id)
		}
	}
	return issues
}
