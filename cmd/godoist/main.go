// Command godoist is a Todoist TUI and scriptable CLI.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/biomassa/godoist/internal/cache"
	"github.com/biomassa/godoist/internal/config"
	"github.com/biomassa/godoist/internal/todoist"
	"github.com/biomassa/godoist/internal/tui"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// client returns an API client for the configured token.
func client() (*todoist.Client, error) {
	cl, _, err := clientAndToken()
	return cl, err
}

// clientAndToken returns an API client and the token. The cache uses the token to identify the account.
func clientAndToken() (*todoist.Client, string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, "", err
	}
	return todoist.New(cfg.Token), cfg.Token, nil
}

// synced returns the account replica. It reads the cache, pulls changes, and saves the cache.
func synced(ctx context.Context) (todoist.SyncState, error) {
	cl, token, err := clientAndToken()
	if err != nil {
		return todoist.SyncState{}, err
	}
	st := cache.Load(token)
	if err := cl.Sync(ctx, &st); err != nil {
		return st, err
	}
	// The Sync API does not order projects. Use the sidebar order: Inbox first, then child_order.
	sort.SliceStable(st.Projects, func(i, j int) bool {
		a, b := st.Projects[i], st.Projects[j]
		if a.InboxProject != b.InboxProject {
			return a.InboxProject
		}
		return a.ChildOrder < b.ChildOrder
	})
	return st, cache.Save(token, st)
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// isTTY reports whether stdout is a terminal.
func isTTY() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "godoist",
		Short:         "Todoist in the terminal — run without arguments for the TUI",
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, token, err := clientAndToken()
			if err != nil {
				return err
			}
			_, err = tea.NewProgram(tui.New(c, token)).Run()
			return err
		},
	}
	root.AddCommand(loginCmd(), projectsCmd(), lsCmd(), addCmd(), closeCmd("done", "Complete tasks by ID"),
		closeCmd("reopen", "Reopen completed tasks by ID"), rmCmd())
	return root
}

func loginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Save your API token to the config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprint(os.Stderr, "Todoist API token (Settings → Integrations → Developer): ")
			line, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil && line == "" {
				return err
			}
			token := strings.TrimSpace(line)
			c, cancel := ctx()
			defer cancel()
			if _, err := todoist.New(token).Projects(c); err != nil {
				return fmt.Errorf("token check failed: %w", err)
			}
			p, err := config.Save(config.Config{Token: token})
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "saved to", p)
			return nil
		},
	}
}

func projectsCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "List projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, cancel := ctx()
			defer cancel()
			st, err := synced(c)
			if err != nil {
				return err
			}
			ps := st.Projects
			if asJSON {
				return printJSON(ps)
			}
			tw := newTable()
			for _, p := range ps {
				fmt.Fprintf(tw, "%s\t%s\t%s\n", p.ID, p.Name, p.Color)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

// table is a column writer for command output.
type table interface {
	io.Writer
	Flush() error
}

// tsv writes tab-separated lines without alignment.
type tsv struct{ io.Writer }

func (tsv) Flush() error { return nil }

// newTable aligns columns on a terminal and emits plain TSV when piped.
func newTable() table {
	if isTTY() {
		return tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	}
	return tsv{os.Stdout}
}

// findProject finds a project by ID, by name (case-insensitive), or by a unique name prefix.
func findProject(ps []todoist.Project, q string) (*todoist.Project, error) {
	var hits []*todoist.Project
	for i := range ps {
		if ps[i].ID == q || strings.EqualFold(ps[i].Name, q) {
			return &ps[i], nil
		}
		if strings.HasPrefix(strings.ToLower(ps[i].Name), strings.ToLower(q)) {
			hits = append(hits, &ps[i])
		}
	}
	if len(hits) == 1 {
		return hits[0], nil
	}
	if len(hits) > 1 {
		return nil, fmt.Errorf("project %q is ambiguous", q)
	}
	return nil, fmt.Errorf("no project %q", q)
}

func lsCmd() *cobra.Command {
	var project, filter string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List active tasks",
		Example: `  godoist ls
  godoist ls -p work
  godoist ls -f "today | overdue" --json | jq -r '.[].content'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, cancel := ctx()
			defer cancel()
			st, err := synced(c)
			if err != nil {
				return err
			}
			ps, ts := st.Projects, st.Tasks
			if filter != "" {
				// The server evaluates filter queries.
				cl, err := client()
				if err != nil {
					return err
				}
				if ts, err = cl.Filter(c, filter); err != nil {
					return err
				}
			}
			if project != "" {
				p, err := findProject(ps, project)
				if err != nil {
					return err
				}
				out := ts[:0]
				for _, t := range ts {
					if t.ProjectID == p.ID {
						out = append(out, t)
					}
				}
				ts = out
			}
			names := map[string]string{}
			order := map[string]int{}
			for i, p := range ps {
				names[p.ID], order[p.ID] = p.Name, i
			}
			sort.SliceStable(ts, func(i, j int) bool {
				if ts[i].ProjectID != ts[j].ProjectID {
					return order[ts[i].ProjectID] < order[ts[j].ProjectID]
				}
				return ts[i].ChildOrder < ts[j].ChildOrder
			})
			if asJSON {
				if ts == nil {
					ts = []todoist.Task{}
				}
				return printJSON(ts)
			}
			now := time.Now()
			tw := newTable()
			for _, t := range ts {
				due := todoist.FormatDue(t.Due, now)
				if due == "" {
					due = "-"
				}
				fmt.Fprintf(tw, "%s\tp%d\t%s\t%s\t%s\n", t.ID, t.UIPriority(), due, names[t.ProjectID], t.Content)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVarP(&project, "project", "p", "", "only tasks in this project (name, prefix, or ID)")
	cmd.Flags().StringVarP(&filter, "filter", "f", "", `Todoist filter query, e.g. "today | overdue"`)
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

func addCmd() *cobra.Command {
	var project string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "add <text>",
		Short: "Quick-add a task using Todoist's natural language parsing",
		Example: `  godoist add "Buy cables tomorrow 10am #work p2 @errand"
  godoist add -p home "Fix the shelf friday"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := client()
			if err != nil {
				return err
			}
			c, cancel := ctx()
			defer cancel()
			t, err := cl.QuickAdd(c, strings.Join(args, " "))
			if err != nil {
				return err
			}
			if project != "" {
				ps, err := cl.Projects(c)
				if err != nil {
					return err
				}
				p, err := findProject(ps, project)
				if err != nil {
					return fmt.Errorf("task %s added to inbox, but: %w", t.ID, err)
				}
				if p.ID != t.ProjectID {
					if err := cl.Move(c, t.ID, p.ID, ""); err != nil {
						return err
					}
					t.ProjectID = p.ID
				}
			}
			switch {
			case asJSON:
				return printJSON(t)
			case isTTY():
				fmt.Println(addedLine(c, cl, t))
			default:
				fmt.Println(t.ID)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&project, "project", "p", "", "put the task in this project")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the created task as JSON (default: a summary in a terminal, the ID in a pipe)")
	return cmd
}

// addedLine is the summary that add shows in a terminal. If the project or section
// names are not available, the line shows their IDs.
func addedLine(c context.Context, cl *todoist.Client, t todoist.Task) string {
	where := t.ProjectID
	if ps, err := cl.Projects(c); err == nil {
		for _, p := range ps {
			if p.ID == t.ProjectID {
				where = p.Name
			}
		}
	}
	if sid := t.Section(); sid != "" {
		name := sid
		if ss, err := cl.Sections(c); err == nil {
			for _, sec := range ss {
				if sec.ID == sid {
					name = sec.Name
				}
			}
		}
		where += " / " + name
	}
	parts := []string{fmt.Sprintf("✓ Added %q", t.Content), where}
	if t.Due != nil {
		due := todoist.FormatDue(t.Due, time.Now())
		if t.Due.IsRecurring {
			due += " ↻ " + t.Due.String
		}
		parts = append(parts, due)
	}
	return strings.Join(parts, " · ") + "  (" + t.ID + ")"
}

func closeCmd(name, short string) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <id>...",
		Short: short,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := client()
			if err != nil {
				return err
			}
			c, cancel := ctx()
			defer cancel()
			for _, id := range args {
				if name == "done" {
					err = cl.Close(c, id)
				} else {
					err = cl.Reopen(c, id)
				}
				if err != nil {
					return fmt.Errorf("%s: %w", id, err)
				}
			}
			return nil
		},
	}
}

func rmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id>...",
		Short: "Delete tasks permanently",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := client()
			if err != nil {
				return err
			}
			c, cancel := ctx()
			defer cancel()
			for _, id := range args {
				if err := cl.Delete(c, id); err != nil {
					return fmt.Errorf("%s: %w", id, err)
				}
			}
			return nil
		},
	}
}
