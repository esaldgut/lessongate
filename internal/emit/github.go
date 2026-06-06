package emit

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v88/github"
)

// githubForge implements Forge against the real GitHub API via go-github v88.
// It opens DRAFT PRs only (NewPullRequest{Draft: true}) and never pushes to
// protected branches. Exercised by the Fase 5 smoke test, not unit tests.
type githubForge struct {
	client *github.Client
	owner  string
	repo   string
	base   string // target branch, e.g. "develop"
}

// NewGitHubForge builds a Forge for owner/repo targeting base.
func NewGitHubForge(client *github.Client, owner, repo, base string) Forge {
	return &githubForge{client: client, owner: owner, repo: repo, base: base}
}

// FindByFingerprint searches open AND closed PRs for the fingerprint marker in
// the body, so a re-run never duplicates a PR that was already opened or merged.
func (g *githubForge) FindByFingerprint(ctx context.Context, fingerprint string) (string, bool, error) {
	marker := BodyMarker(fingerprint)
	opt := &github.PullRequestListOptions{
		State:       "all",
		Sort:        "created",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		prs, resp, err := g.client.PullRequests.List(ctx, g.owner, g.repo, opt)
		if err != nil {
			return "", false, fmt.Errorf("emit: list PRs: %w", err)
		}
		for _, pr := range prs {
			if pr.Body != nil && strings.Contains(*pr.Body, marker) {
				return pr.GetHTMLURL(), true, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return "", false, nil
}

// CommitFile creates `branch` off the configured base (if it doesn't exist) and
// commits `content` at `path` via the Contents API — no local clone needed.
func (g *githubForge) CommitFile(ctx context.Context, branch, path, content, message string) error {
	// Resolve the base branch's head SHA.
	baseRef, _, err := g.client.Git.GetRef(ctx, g.owner, g.repo, "refs/heads/"+g.base)
	if err != nil {
		return fmt.Errorf("emit: get base ref %q: %w", g.base, err)
	}

	// Create the branch ref only if it doesn't already exist. We check existence
	// explicitly via GetRef rather than string-matching a "already exists" error
	// message (which is fragile and provider-version-dependent).
	branchRef := "refs/heads/" + branch
	if _, _, gerr := g.client.Git.GetRef(ctx, g.owner, g.repo, branchRef); gerr != nil {
		if _, _, cerr := g.client.Git.CreateRef(ctx, g.owner, g.repo, github.CreateRef{
			Ref: branchRef,
			SHA: baseRef.GetObject().GetSHA(),
		}); cerr != nil {
			return fmt.Errorf("emit: create branch %q: %w", branch, cerr)
		}
	}

	// Does the file already exist on the branch? Need its SHA to update.
	var existingSHA *string
	if fc, _, _, gerr := g.client.Repositories.GetContents(ctx, g.owner, g.repo, path,
		&github.RepositoryContentGetOptions{Ref: branch}); gerr == nil && fc != nil {
		existingSHA = fc.SHA
	}

	_, _, err = g.client.Repositories.CreateFile(ctx, g.owner, g.repo, path, &github.RepositoryContentFileOptions{
		Message: github.Ptr(message),
		Content: []byte(content),
		Branch:  github.Ptr(branch),
		SHA:     existingSHA,
	})
	if err != nil {
		return fmt.Errorf("emit: write %q: %w", path, err)
	}
	return nil
}

// CreateDraft opens a DRAFT pull request from branch into the configured base.
func (g *githubForge) CreateDraft(ctx context.Context, branch, title, body string) (string, error) {
	pr, _, err := g.client.PullRequests.Create(ctx, g.owner, g.repo, &github.NewPullRequest{
		Title: github.Ptr(title),
		Head:  github.Ptr(branch),
		Base:  github.Ptr(g.base),
		Body:  github.Ptr(body),
		Draft: github.Ptr(true), // DRAFT only — human reviews before merge
	})
	if err != nil {
		return "", fmt.Errorf("emit: create draft PR: %w", err)
	}
	return pr.GetHTMLURL(), nil
}
