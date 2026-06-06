package watch

import (
	"context"

	"github.com/google/go-github/v88/github"
)

// githubLister implements the lister using go-github v88: it lists closed PRs
// for the repo. Merge filtering + cursor logic stay in the Watcher (tested
// without a network). Exercised by the Fase 5 smoke test.
type githubLister struct {
	client *github.Client
	owner  string
	repo   string
}

// NewGitHubWatcher builds a Watcher backed by the GitHub API.
func NewGitHubWatcher(client *github.Client, owner, repo string) Watcher {
	return New(&githubLister{client: client, owner: owner, repo: repo})
}

func (g *githubLister) listClosed(ctx context.Context, limit int) ([]MergedPR, error) {
	opt := &github.PullRequestListOptions{
		State:       "closed",
		Sort:        "updated",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}
	var out []MergedPR
	for {
		prs, resp, err := g.client.PullRequests.List(ctx, g.owner, g.repo, opt)
		if err != nil {
			return nil, err
		}
		for _, pr := range prs {
			if pr.MergedAt == nil {
				continue // closed but not merged
			}
			mp := MergedPR{
				Number:   pr.GetNumber(),
				Title:    pr.GetTitle(),
				MergedAt: pr.GetMergedAt().Time,
				MergeSHA: pr.GetMergeCommitSHA(),
			}
			out = append(out, mp)
		}
		// Stop early once we have well over the limit (the Watcher trims).
		if resp.NextPage == 0 || (limit > 0 && len(out) >= limit*2) {
			break
		}
		opt.Page = resp.NextPage
	}
	return out, nil
}
