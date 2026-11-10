package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/go-bumbu/db/gormdb"
	"github.com/go-bumbu/reference-app/app/logger"
	"github.com/go-bumbu/reference-app/app/router"
	"github.com/go-bumbu/reference-app/internal/notes"
	"github.com/go-bumbu/reference-app/internal/tenants"
	"github.com/spf13/cobra"
)

func seedCmd() *cobra.Command {
	var configFile string
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "add demo notes and tags for the user of auth method none",
		Long: fmt.Sprintf("seed adds a demo tag tree and notes for user %q of tenant %q, the identity of auth "+
			"method none, to the configured database. It is not idempotent: every run adds the demo data again.",
			router.DefaultUser, router.DefaultTenant),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSeed(cmd.Context(), configFile)
		},
	}
	cmd.Flags().StringVarP(&configFile, "config", "c", "",
		fmt.Sprintf("config file; default: %s then %s", configSearchPaths[0], configSearchPaths[1]))
	return cmd
}

// runSeed opens the configured database the way runServer does and writes the
// demo data through the modules' public APIs.
func runSeed(ctx context.Context, configFile string) (err error) {
	path, mandatory := resolveConfigFile(configFile, configSearchPaths)
	cfg, err := getAppCfg(path, mandatory)
	if err != nil {
		return err
	}
	l, err := logger.GetDefault(logger.GetLogLevel(cfg.Env.LogLevel))
	if err != nil {
		return err
	}
	db, err := openDB(cfg.DB, l)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := gormdb.Close(db); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close database: %w", cerr))
		}
	}()

	tenantsSvc, err := tenants.New(ctx, db, tenants.Cfg{})
	if err != nil {
		return fmt.Errorf("init tenants: %w", err)
	}
	notesSvc, err := notes.New(ctx, db, notes.Cfg{})
	if err != nil {
		return fmt.Errorf("init notes: %w", err)
	}
	n, err := seedDemoData(ctx, tenantsSvc, notesSvc)
	if err != nil {
		return err
	}
	l.Info("demo data added", slog.String("component", "seed"),
		slog.String("tenant", router.DefaultTenant), slog.String("user", router.DefaultUser),
		slog.Int("notes", n))
	return nil
}

// demoTag is a tag of the demo tree, with the tags below it.
type demoTag struct {
	name     string
	children []demoTag
}

var demoTags = []demoTag{
	{name: "work", children: []demoTag{{name: "projects"}, {name: "meetings"}}},
	{name: "personal", children: []demoTag{{name: "recipes"}, {name: "travel"}}},
	{name: "reading"},
}

// demoNote is a note of the demo data; tags are names from demoTags.
type demoNote struct {
	title string
	body  string
	tags  []string
}

var demoNotes = []demoNote{
	{
		title: "Welcome to refapp",
		body: "This is demo data, added by `make run` on a fresh database.\n\n" +
			"Run `make clean` to delete the database and start over.",
	},
	{
		title: "Q4 roadmap",
		body:  "- ship tag filters\n- versioned migrations\n- OIDC login",
		tags:  []string{"projects"},
	},
	{
		title: "Weekly sync",
		body:  "Agenda: release status, open reviews, on-call handover.",
		tags:  []string{"meetings", "projects"},
	},
	{
		title: "Pancakes",
		body:  "200 g flour, 2 eggs, 300 ml milk, a pinch of salt. Rest the batter for 30 minutes.",
		tags:  []string{"recipes"},
	},
	{
		title: "Lisbon trip",
		body:  "Book the tram 28 early in the morning; Belém on day two.",
		tags:  []string{"travel"},
	},
	{
		title: "A Philosophy of Software Design",
		body:  "Deep modules: a simple interface over a lot of functionality.",
		tags:  []string{"reading", "work"},
	},
}

// seedDemoData provisions the identity of auth method none and adds demoTags
// and demoNotes to its tenant. It returns the number of notes added.
func seedDemoData(ctx context.Context, t *tenants.Service, n *notes.Service) (int, error) {
	p, err := t.Provision(ctx, router.DefaultTenant, router.DefaultUser)
	if err != nil {
		return 0, fmt.Errorf("provision %s/%s: %w", router.DefaultTenant, router.DefaultUser, err)
	}
	a := notes.Actor{TenantID: p.TenantID, UserID: p.UserID}

	tagIDs := map[string]uint{}
	if err := createDemoTags(ctx, n, a, 0, demoTags, tagIDs); err != nil {
		return 0, err
	}
	for _, dn := range demoNotes {
		note, err := n.Create(ctx, a, notes.Input{Title: dn.title, Body: dn.body})
		if err != nil {
			return 0, fmt.Errorf("create note %q: %w", dn.title, err)
		}
		for _, tag := range dn.tags {
			id, ok := tagIDs[tag]
			if !ok {
				return 0, fmt.Errorf("note %q: unknown demo tag %q", dn.title, tag)
			}
			if _, err := n.AddNoteTag(ctx, a, note.ID, id); err != nil {
				return 0, fmt.Errorf("tag note %q with %q: %w", dn.title, tag, err)
			}
		}
	}
	return len(demoNotes), nil
}

// createDemoTags creates tags under parent in the given order (a nil AfterID
// would place each one first), recursing into their children, and records
// every id by name in ids.
func createDemoTags(ctx context.Context, n *notes.Service, a notes.Actor, parent uint, tags []demoTag, ids map[string]uint) error {
	var after uint
	for _, dt := range tags {
		tag, err := n.CreateTag(ctx, a, notes.TagInput{Name: dt.name, ParentID: parent, AfterID: &after})
		if err != nil {
			return fmt.Errorf("create tag %q: %w", dt.name, err)
		}
		ids[dt.name] = tag.ID
		after = tag.ID
		if err := createDemoTags(ctx, n, a, tag.ID, dt.children, ids); err != nil {
			return err
		}
	}
	return nil
}
