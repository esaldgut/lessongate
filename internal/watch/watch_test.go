package watch

import (
	"context"
	"testing"
	"time"
)

func tm(day int) time.Time { return time.Date(2026, 6, day, 0, 0, 0, 0, time.UTC) }

// fakeLister returns a fixed page of closed PRs, simulating go-github's
// PullRequests.List without a network call.
type fakeLister struct{ prs []MergedPR }

func (f fakeLister) listClosed(_ context.Context, _ int) ([]MergedPR, error) {
	return f.prs, nil
}

func TestMergedSince_FiltersByMergedAt(t *testing.T) {
	src := fakeLister{prs: []MergedPR{
		{Number: 38, MergedAt: tm(1), MergeSHA: "a"},
		{Number: 39, MergedAt: tm(3), MergeSHA: "b"},
		{Number: 40, MergedAt: tm(5), MergeSHA: "c"},
	}}
	w := New(src)
	got, err := w.MergedSince(context.Background(), tm(2), 10)
	if err != nil {
		t.Fatalf("MergedSince: %v", err)
	}
	// Only PRs merged strictly after the cursor (day 2): 39 and 40.
	if len(got) != 2 {
		t.Fatalf("expected 2 PRs after cursor, got %d: %+v", len(got), got)
	}
	if got[0].Number != 39 || got[1].Number != 40 {
		t.Fatalf("wrong PRs returned: %+v", got)
	}
}

func TestMergedSince_OutOfOrderMergeIsCaught(t *testing.T) {
	// PR #310 merged BEFORE #305 in real time. A number-based cursor would skip
	// #305; a mergedAt cursor must catch it. Cursor = day 4.
	src := fakeLister{prs: []MergedPR{
		{Number: 310, MergedAt: tm(3), MergeSHA: "x"}, // before cursor → skip
		{Number: 305, MergedAt: tm(6), MergeSHA: "y"}, // after cursor → include despite lower number
	}}
	w := New(src)
	got, _ := w.MergedSince(context.Background(), tm(4), 10)
	if len(got) != 1 || got[0].Number != 305 {
		t.Fatalf("mergedAt cursor failed on out-of-order merge: %+v", got)
	}
}

func TestMergedSince_SkipsUnmergedClosedPRs(t *testing.T) {
	// A closed-but-not-merged PR has a zero MergedAt and must be excluded.
	src := fakeLister{prs: []MergedPR{
		{Number: 1, MergedAt: time.Time{}, MergeSHA: ""}, // closed, not merged
		{Number: 2, MergedAt: tm(5), MergeSHA: "m"},
	}}
	w := New(src)
	got, _ := w.MergedSince(context.Background(), tm(1), 10)
	if len(got) != 1 || got[0].Number != 2 {
		t.Fatalf("unmerged closed PR leaked through: %+v", got)
	}
}

func TestMergedSince_RespectsLimit(t *testing.T) {
	src := fakeLister{prs: []MergedPR{
		{Number: 1, MergedAt: tm(2), MergeSHA: "a"},
		{Number: 2, MergedAt: tm(3), MergeSHA: "b"},
		{Number: 3, MergedAt: tm(4), MergeSHA: "c"},
	}}
	w := New(src)
	got, _ := w.MergedSince(context.Background(), tm(1), 2)
	if len(got) != 2 {
		t.Fatalf("limit not respected: got %d, want 2", len(got))
	}
}

func TestMergedSince_ResultsAscendingByMergedAt(t *testing.T) {
	// Process oldest-first so earlier lessons can inform later ones.
	src := fakeLister{prs: []MergedPR{
		{Number: 3, MergedAt: tm(5), MergeSHA: "c"},
		{Number: 1, MergedAt: tm(2), MergeSHA: "a"},
		{Number: 2, MergedAt: tm(3), MergeSHA: "b"},
	}}
	w := New(src)
	got, _ := w.MergedSince(context.Background(), tm(1), 10)
	for i := 1; i < len(got); i++ {
		if got[i].MergedAt.Before(got[i-1].MergedAt) {
			t.Fatalf("results not ascending by mergedAt: %+v", got)
		}
	}
}
