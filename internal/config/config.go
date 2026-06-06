// Package config holds lessongate's runtime configuration and the shared
// domain types that flow through the pipeline. It also performs process
// hardening at startup (disabling core dumps so an in-memory diff can't leak
// to /cores on a crash).
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config is the resolved runtime configuration for a single invocation.
type Config struct {
	// PrivateRepo is the watch target (owner/name), e.g. the source of lessons.
	PrivateOwner string
	PrivateRepo  string

	// PublicRepo is where draft PRs are opened.
	PublicOwner string
	PublicRepo  string
	PublicBase  string // target branch for drafts, e.g. "develop"

	// LessonsFile is the curated lesson registry consumed by the pipeline
	// (NOT the raw diff). Low NDA density; already human-distilled.
	LessonsFile string

	// StateDir holds the ledger + lockfile. Outside any synced/backed-up path.
	StateDir string

	// QuarantineDir holds content that has NOT passed the gate. Outside synced
	// paths, chmod 700, TTL-shredded. Never committable (see .gitignore).
	QuarantineDir string

	// SkillCreatorDir overrides plugin discovery (for tests / version pinning).
	// Empty means auto-discover by glob. See internal/skillcreator.
	SkillCreatorDir string

	// MaxPRsPerRun caps backfill so a first run can't blow the rate limit.
	MaxPRsPerRun int

	// ClaudeModel is the pinned model ID, recorded in the ledger per lesson so
	// emissions can be re-audited if the gate's behavior drifts across models.
	ClaudeModel string

	// DryRun lists candidates and estimates cost without opening any PR.
	DryRun bool
}

// LessonID uniquely identifies one lesson extracted from one PR. A PR may yield
// several lessons (1:N), so the PR number alone is not a key.
type LessonID struct {
	PR     int
	Index  int    // k-th lesson within the PR, 1-based
	SHA    string // merge commit SHA — stable identity (PR numbers can be fragile)
}

func (id LessonID) String() string {
	return fmt.Sprintf("PR#%d::lesson-%d", id.PR, id.Index)
}

// Branch returns the deterministic branch name used for idempotent draft PRs.
func (id LessonID) Branch() string {
	return fmt.Sprintf("lessongate/pr-%d-lesson-%d", id.PR, id.Index)
}

// Stage is a node in the per-lesson state machine. Persisted in the ledger so a
// crash mid-run resumes idempotently from wherever each lesson stopped.
type Stage string

const (
	StageDiscovered Stage = "discovered"
	StageExtracted  Stage = "extracted"
	StageGated      Stage = "gated"
	StageReconciled Stage = "reconciled"
	StageEmitted    Stage = "emitted"
	StageDone       Stage = "done"
	StageRejected   Stage = "rejected" // failed the gate; quarantined, not emitted
)

// Default values for fields not otherwise supplied.
const (
	DefaultPublicBase   = "develop"
	DefaultMaxPRsPerRun = 10
	// DefaultClaudeModel is pinned, not "latest" — see the gate-drift threat.
	DefaultClaudeModel = "claude-opus-4-8"
)

// home returns $HOME or errors if unset (we never silently fall back to ".").
func home() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return "", fmt.Errorf("config: cannot resolve home directory: %w", err)
	}
	return h, nil
}

// Default returns a Config wired to the lessongate state/quarantine layout under
// the user's home, with the conservative defaults applied. Callers override
// fields (repos, lessons file) before use.
func Default() (Config, error) {
	h, err := home()
	if err != nil {
		return Config{}, err
	}
	root := filepath.Join(h, ".lessongate")
	return Config{
		PublicBase:    DefaultPublicBase,
		StateDir:      filepath.Join(root, "state"),
		QuarantineDir: filepath.Join(root, "quarantine"),
		MaxPRsPerRun:  DefaultMaxPRsPerRun,
		ClaudeModel:   DefaultClaudeModel,
	}, nil
}
