package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-bumbu/tempo"
)

// ScanNotes is the name of the job that walks every note of a tenant.
const ScanNotes = "scan-notes"

// catalogue is every job the app defines, in the order the API lists them. A
// new job is one more entry here and its run function.
func (s *Service) catalogue() []definition {
	return []definition{
		{
			name:        ScanNotes,
			title:       "Scan notes",
			description: "Goes over every note of the tenant and processes it.",
			run:         s.scanNotes,
		},
	}
}

// scanNotes runs ScanNotes: it lists the tenant's notes and hands each to
// scanNote, reporting progress per note. It stops at the first failing note,
// and returns ctx's error when a cancel or a shutdown stops it, so the run
// ends canceled rather than failed.
func (s *Service) scanNotes(ctx context.Context, log *slog.Logger, prog tempo.Progress, tenantID string) error {
	if err := ctx.Err(); err != nil {
		return err // a waiting run that a shutdown drains
	}
	prog.SetStage("Listing notes")
	ids, err := s.notes.NoteIDs(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("list notes: %w", err)
	}
	prog.SetTotal(int64(len(ids)))
	prog.SetStage("Scanning notes")
	log.Info("scanning notes", slog.Int("notes", len(ids)))
	for i, id := range ids {
		if err := ctx.Err(); err != nil {
			log.Info("scan stopped", slog.Int("scanned", i))
			return err
		}
		if err := s.scanNote(ctx, tenantID, id); err != nil {
			return fmt.Errorf("note %s: %w", id, err)
		}
		log.Info("note scanned", slog.String("note", id), slog.Int("n", i+1), slog.Int("of", len(ids)))
		prog.Inc(1)
	}
	log.Info("scan finished", slog.Int("scanned", len(ids)))
	return nil
}

// stubNoteDelay is the time the stub pretends to work on one note, so a run's
// progress can be watched in the UI.
const stubNoteDelay = 50 * time.Millisecond

// scanNoteStub stands in for the real work on a note. Replace it with a call
// into the module that owns that work (an indexer, a link checker, ...),
// passed in like NoteSource.
func scanNoteStub(ctx context.Context, _, _ string) error {
	t := time.NewTimer(stubNoteDelay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
