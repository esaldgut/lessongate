package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/google/go-github/v88/github"

	"lessongate/internal/claude"
	"lessongate/internal/config"
	"lessongate/internal/emit"
	"lessongate/internal/extract"
	"lessongate/internal/gate"
	"lessongate/internal/lessons"
	"lessongate/internal/pipeline"
	"lessongate/internal/redact"
	"lessongate/internal/reconcile"
	"lessongate/internal/skillcreator"
	"lessongate/internal/watch"
)

// denyList is the literal NDA deny-list. Real values come from config in a
// follow-up; the structural patterns in internal/redact are the primary control.
var denyList = []string{}

// runConfig carries the per-invocation knobs from the CLI into the wiring.
type runConfig struct {
	repo        string // private watch target (owner/name)
	publicRepo  string // public repo for draft PRs (owner/name)
	lessonsFile string // override for the curated lesson registry
	dryRun      bool
	once        bool // bypass watch/prefilter; --repo not required
}

// buildDeps constructs the real pipeline collaborators, failing closed when a
// prerequisite is missing. Returns a cleanup func to release resources.
func buildDeps(ctx context.Context, log *slog.Logger, rc runConfig) (*pipeline.Deps, func(), error) {
	noop := func() {}

	// 1. Tokens / API keys. Fail closed with a clear message.
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return nil, noop, fmt.Errorf("ANTHROPIC_API_KEY is not set (required for the extract/verify stages)")
	}
	ghToken := os.Getenv("LESSONGATE_GITHUB_TOKEN")
	if !rc.dryRun && ghToken == "" {
		return nil, noop, fmt.Errorf("LESSONGATE_GITHUB_TOKEN is not set (required to open draft PRs; use dry-run to skip)")
	}

	// 2. Watch target. In --once mode the watcher is unused, so --repo is optional.
	owner, name, ok := splitRepo(rc.repo)
	if !ok && !rc.once {
		return nil, noop, fmt.Errorf("--repo must be owner/name, got %q", rc.repo)
	}

	// 2b. Public repo for draft PRs (required for non-dry runs).
	pubOwner, pubName, pubOK := splitRepo(rc.publicRepo)
	if !rc.dryRun && !pubOK {
		return nil, noop, fmt.Errorf("--public-repo must be owner/name, got %q", rc.publicRepo)
	}

	// 3. skill-creator startup assert (fail closed if python3/PyYAML/plugin absent).
	if _, err := skillcreator.New(ctx, os.Getenv(skillcreator.EnvOverride)); err != nil {
		return nil, noop, fmt.Errorf("skill-creator not ready: %w", err)
	}

	cfg, err := config.Default()
	if err != nil {
		return nil, noop, err
	}
	if rc.lessonsFile != "" {
		cfg.LessonsFile = rc.lessonsFile
	}
	cfg.PublicOwner, cfg.PublicRepo = pubOwner, pubName

	// 4. Clients. go-github v88: NewClient(opts...) returns (*Client, error);
	// auth is a ClientOptionsFunc (WithAuthToken), not a builder method.
	claudeClient := claude.NewSDKClient(cfg.ClaudeModel)
	var ghOpts []github.ClientOptionsFunc
	if ghToken != "" {
		ghOpts = append(ghOpts, github.WithAuthToken(ghToken))
	}
	gh, err := github.NewClient(ghOpts...)
	if err != nil {
		return nil, noop, fmt.Errorf("github client: %w", err)
	}

	det := gate.NewDeterministic(denyList)
	deps := &pipeline.Deps{
		Watcher:    watch.NewGitHubWatcher(gh, owner, name),
		Lessons:    newLessonSource(cfg.LessonsFile),
		Redactor:   redact.New(denyList),
		Extractor:  extract.Extractor{C: claudeClient},
		Gate:       det,
		Verifier:   gate.NewVerifier(claudeClient),
		Reconciler: reconcile.New(loadPublicSkillIndex()),
		Emitter:    emit.New(emit.NewGitHubForge(gh, cfg.PublicOwner, cfg.PublicRepo, cfg.PublicBase), det),
	}
	_ = log
	return deps, noop, nil
}

func splitRepo(s string) (owner, name string, ok bool) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	// Allow a bare repo name (owner resolved elsewhere) only when non-empty.
	if s != "" && !strings.Contains(s, "/") {
		return "", s, true
	}
	return "", "", false
}

// loadPublicSkillIndex returns the current public-repo skill index. v0.1 returns
// an empty index (everything classifies as Novel); a follow-up reads the repo.
func loadPublicSkillIndex() []reconcile.SkillRef { return nil }

// fileLessonSource reads the curated lesson registry and returns lesson bodies.
// v0.1 returns the full curated set for any PR (the curated registry is the
// low-NDA source per threat C1); a follow-up maps PR→specific lessons.
type fileLessonSource struct{ path string }

func newLessonSource(path string) pipeline.LessonSource { return fileLessonSource{path: path} }

func (s fileLessonSource) ForPR(_ context.Context, _ int) ([]string, error) {
	if s.path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	parsed, err := lessons.Parse(string(raw))
	if err != nil {
		return nil, err
	}
	bodies := make([]string, 0, len(parsed))
	for _, l := range parsed {
		bodies = append(bodies, l.Body)
	}
	return bodies, nil
}
