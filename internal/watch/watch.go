// Package watch polls the private repo for newly-merged PRs using go-github.
// There is no native "merged since cursor" filter, so it lists closed PRs sorted
// by updated-desc and gates on MergedAt against the ledger's mergedAt cursor,
// de-duping via the ledger (not a numeric high-water mark — PRs can merge out of
// numeric order; threats C3, N3).
//
// Built in Fase 2. This file declares the contract.
package watch

import (
	"context"
	"slices"
	"time"
)

// MergedPR is a merged pull request discovered by the watcher.
type MergedPR struct {
	Number    int
	Title     string
	MergedAt  time.Time
	MergeSHA  string
	Files     []string
}

// Watcher lists merged PRs since a cursor.
type Watcher interface {
	MergedSince(ctx context.Context, since time.Time, limit int) ([]MergedPR, error)
}

// lister is the narrow dependency on a PR source (a go-github adapter in
// production, a fake in tests). It returns closed PRs; merge filtering and
// cursor logic live in the Watcher so they're testable without a network.
type lister interface {
	listClosed(ctx context.Context, limit int) ([]MergedPR, error)
}

type watcher struct{ src lister }

// New builds a Watcher over a PR source.
func New(src lister) Watcher { return &watcher{src: src} }

// MergedSince returns PRs merged strictly after `since`, ascending by mergedAt
// (oldest first, so earlier lessons can inform later ones), capped at limit.
// It uses the mergedAt timestamp — not the PR number — as the cursor, so PRs
// that merge out of numeric order are not skipped (threats C3, N1, N3).
func (w *watcher) MergedSince(ctx context.Context, since time.Time, limit int) ([]MergedPR, error) {
	all, err := w.src.listClosed(ctx, limit)
	if err != nil {
		return nil, err
	}

	var merged []MergedPR
	for _, pr := range all {
		if pr.MergedAt.IsZero() {
			continue // closed but never merged
		}
		if pr.MergedAt.After(since) {
			merged = append(merged, pr)
		}
	}

	slices.SortFunc(merged, func(a, b MergedPR) int {
		return a.MergedAt.Compare(b.MergedAt)
	})

	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}
	return merged, nil
}
