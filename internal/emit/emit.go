// Package emit opens (or updates) a DRAFT pull request on the public repo via
// go-github. It is idempotent: a deterministic branch name plus a fingerprint
// HTML-comment in the PR body let it detect an existing PR (open or merged) and
// skip/update rather than duplicate (threats I3, C-idem). The PR body is EMIT
// output and is gated exactly like the SKILL.md before creation (threat I5).
// It always opens DRAFTs and never touches protected branches directly.
//
// Built in Fase 4. This file declares the contract.
package emit

import (
	"context"
	"crypto/sha256"
	"fmt"

	"lessongate/internal/config"
	"lessongate/internal/gate"
)

// Fingerprint is the stable idempotency key embedded in the PR body.
func Fingerprint(id config.LessonID, contentHash string) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%s", id, contentHash))
	return fmt.Sprintf("%x", sum[:])
}

// BodyMarker renders the HTML-comment marker placed in every lessongate PR body.
func BodyMarker(fp string) string {
	return fmt.Sprintf("<!-- lessongate:fp=%s -->", fp)
}

// skillPath returns the in-repo path for a lesson's generated SKILL.md. The
// branch name already encodes the lesson identity, so the path stays stable per
// lesson (idempotent re-runs target the same file).
func skillPath(id config.LessonID) string {
	return fmt.Sprintf("global-skills/_incoming/%s/SKILL.md", id.Branch())
}

// Emitter opens draft PRs.
type Emitter interface {
	// EmitDraft creates or updates the draft PR for a gated candidate, returning
	// its URL. Idempotent on the fingerprint.
	EmitDraft(ctx context.Context, id config.LessonID, skillMD, body, fingerprint string) (url string, err error)
}

// Forge is the narrow GitHub surface emit needs: look up an existing PR by
// fingerprint, commit a file onto a new branch, and create a draft PR. A
// go-github adapter implements it in production; a fake implements it in tests.
type Forge interface {
	FindByFingerprint(ctx context.Context, fingerprint string) (url string, exists bool, err error)
	// CommitFile creates `branch` (from the default base) if needed and commits
	// `content` at `path` with the given commit message — via the Git Data API,
	// no local clone required.
	CommitFile(ctx context.Context, branch, path, content, message string) error
	CreateDraft(ctx context.Context, branch, title, body string) (url string, err error)
}

// emitter implements Emitter with idempotency and body re-gating. The gate is a
// gate.Deterministic — the PR body and SKILL.md are EMIT output and must pass
// the same trustworthy control as any other published artifact.
type emitter struct {
	forge Forge
	gate  gate.Deterministic
}

// New builds an Emitter over a Forge and the deterministic gate.
func New(forge Forge, g gate.Deterministic) Emitter {
	return &emitter{forge: forge, gate: g}
}

// EmitDraft re-gates the PR body (threat I5 — the body is EMIT output), checks
// the fingerprint for idempotency (threat I3), and only then creates a DRAFT PR
// carrying the fingerprint marker. It never updates protected branches directly.
func (e *emitter) EmitDraft(ctx context.Context, id config.LessonID, skillMD, body, fingerprint string) (string, error) {
	// 1. The PR body is EMIT output — it must pass the gate just like the SKILL.md.
	if v := e.gate.Check(body); !v.Safe {
		return "", fmt.Errorf("emit: PR body failed the gate: %v", v.Reasons)
	}
	if v := e.gate.Check(skillMD); !v.Safe {
		return "", fmt.Errorf("emit: SKILL.md failed the gate: %v", v.Reasons)
	}

	// 2. Idempotency: if a PR with this fingerprint already exists, skip.
	if url, exists, err := e.forge.FindByFingerprint(ctx, fingerprint); err != nil {
		return "", fmt.Errorf("emit: fingerprint lookup: %w", err)
	} else if exists {
		return url, nil
	}

	// 3. Commit the SKILL.md onto a new branch (Git Data API, no local clone).
	path := skillPath(id)
	msg := fmt.Sprintf("feat(skills): add generic skill from %s", id)
	if err := e.forge.CommitFile(ctx, id.Branch(), path, skillMD, msg); err != nil {
		return "", fmt.Errorf("emit: commit skill file: %w", err)
	}

	// 4. Create the draft PR with the fingerprint marker embedded in the body.
	fullBody := body + "\n\n" + BodyMarker(fingerprint)
	title := fmt.Sprintf("skill: %s (%s)", id.Branch(), id)
	return e.forge.CreateDraft(ctx, id.Branch(), title, fullBody)
}
