// Package prefilter cheaply drops PRs that can't yield a generalizable lesson
// BEFORE spending an API token: docs-only changes, dependency bumps, and the
// pipeline's own "chore(lessons): capture PR#N" commits (never recurse into the
// upstream's own capture commits). Threat I6 (cost/rate-limit).
//
// Built in Fase 2. This file declares the contract.
package prefilter

import (
	"path/filepath"
	"strings"
)

// PR is the minimal view prefilter needs (no diff body required to decide).
type PR struct {
	Number       int
	Title        string
	ChangedFiles []string
}

// docExts are file extensions considered documentation/non-code.
var docExts = map[string]bool{".md": true, ".mdx": true, ".txt": true, ".rst": true, ".adoc": true}

// Skip reports whether the PR should be dropped without an API call, and why.
// Order matters: title-based rules (cheap, unambiguous) run before file-based.
func Skip(pr PR) (skip bool, reason string) {
	title := strings.ToLower(strings.TrimSpace(pr.Title))

	// 1. The pipeline's own capture commits — never recurse.
	if strings.HasPrefix(title, "chore(lessons):") {
		return true, "own capture commit"
	}

	// 2. Dependency bumps — no generalizable engineering lesson.
	if strings.HasPrefix(title, "chore(deps):") ||
		strings.HasPrefix(title, "build(deps):") ||
		strings.Contains(title, "update dependencies") ||
		strings.Contains(title, "bump ") {
		return true, "dependency bump"
	}

	// 3. Docs-only — every changed file is documentation.
	if len(pr.ChangedFiles) > 0 {
		allDocs := true
		for _, f := range pr.ChangedFiles {
			if !docExts[strings.ToLower(filepath.Ext(f))] {
				allDocs = false
				break
			}
		}
		if allDocs {
			return true, "docs-only"
		}
	}

	return false, ""
}
