// Package skillcreator integrates the official Claude Code `skill-creator`
// plugin by shelling out to its Python scripts (primary mode). It NEVER
// hardcodes the plugin's version-hash path segment: the path is discovered by
// glob at startup and the newest cached version is chosen. A fail-closed startup
// assert verifies python3 + the script + PyYAML are present before the pipeline
// processes any lesson — lessongate refuses to degrade silently past validation.
//
// Security (threats I7, I8): invocation is via exec.Command with an explicit
// argv slice (never `sh -c` with interpolation), the subprocess inherits a
// minimal env (no GitHub PAT), and stdout/stderr are captured to memory and
// re-gated before reaching any sink. Exit codes distinguish "invalid skill"
// (1) from "script crashed" (anything else).
package skillcreator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// glob locates quick_validate.py across all cached plugin versions. The "*"
// matches the version hash so an update doesn't break us.
const pluginGlob = "/.claude/plugins/cache/claude-plugins-official/skill-creator/*/skills/skill-creator"

// EnvOverride lets tests / version-pinning bypass discovery.
const EnvOverride = "LESSONGATE_SKILLCREATOR_DIR"

// Validator wraps the discovered skill-creator install.
type Validator struct {
	dir        string // .../skills/skill-creator
	python3    string // resolved absolute path to python3
}

// Discover resolves the skill-creator directory: an explicit override wins;
// otherwise the newest cached plugin version (by mtime) is selected.
func Discover(override string) (string, error) {
	if override != "" {
		if _, err := os.Stat(filepath.Join(override, "scripts", "quick_validate.py")); err != nil {
			return "", fmt.Errorf("skillcreator: override dir invalid: %w", err)
		}
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("skillcreator: cannot resolve home: %w", err)
	}
	matches, err := filepath.Glob(home + pluginGlob)
	if err != nil {
		return "", fmt.Errorf("skillcreator: glob failed: %w", err)
	}
	if len(matches) == 0 {
		return "", errors.New("skillcreator: plugin not found in cache (is skill-creator installed?)")
	}
	// Newest by mtime wins when multiple versions are cached.
	sort.Slice(matches, func(i, j int) bool {
		fi, _ := os.Stat(matches[i])
		fj, _ := os.Stat(matches[j])
		if fi == nil || fj == nil {
			return false
		}
		return fi.ModTime().After(fj.ModTime())
	})
	return matches[0], nil
}

// New runs the fail-closed startup assert and returns a ready Validator.
// It verifies: python3 on PATH (not `python`), quick_validate.py present, and
// PyYAML importable. Any failure aborts — the pipeline must not skip validation.
func New(ctx context.Context, override string) (*Validator, error) {
	dir, err := Discover(override)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(dir, "scripts", "quick_validate.py")); err != nil {
		return nil, fmt.Errorf("skillcreator: quick_validate.py missing: %w", err)
	}
	python3, err := exec.LookPath("python3")
	if err != nil {
		return nil, fmt.Errorf("skillcreator: python3 not on PATH (note: `python` is not used): %w", err)
	}
	// PyYAML importable?
	check := exec.CommandContext(ctx, python3, "-c", "import yaml")
	check.Env = minimalEnv()
	if err := check.Run(); err != nil {
		return nil, fmt.Errorf("skillcreator: PyYAML not importable under python3: %w", err)
	}
	return &Validator{dir: dir, python3: python3}, nil
}

// ValidateResult is the outcome of quick_validate.py for one skill dir.
type ValidateResult struct {
	Valid   bool
	Message string // human-readable validator message (re-gated before any sink)
}

// Validate runs quick_validate.py against a skill directory. It distinguishes a
// content rejection (exit 1) from an infrastructure crash (anything else),
// returning an error only for the latter. stdout/stderr are captured to memory
// and never written to a sink here — the caller re-gates the message.
func (v *Validator) Validate(ctx context.Context, skillDir string) (ValidateResult, error) {
	cmd := exec.CommandContext(ctx, v.python3,
		filepath.Join(v.dir, "scripts", "quick_validate.py"), skillDir)
	cmd.Env = minimalEnv() // no GitHub PAT / secrets inherited by the subprocess

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		// exit 0 → valid
		return ValidateResult{Valid: true, Message: strings.TrimSpace(stdout.String())}, nil
	}

	// Distinguish a clean exit-1 rejection from any other failure (crash).
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		if ee.ExitCode() == 1 {
			return ValidateResult{Valid: false, Message: strings.TrimSpace(stdout.String())}, nil
		}
		// Any other exit code is an infrastructure error, NOT a content rejection.
		return ValidateResult{Valid: false}, fmt.Errorf(
			"skillcreator: quick_validate.py crashed (exit %d): %s",
			ee.ExitCode(), strings.TrimSpace(stderr.String()))
	}
	// exec error (couldn't start, context cancelled, etc.) — infra error.
	return ValidateResult{Valid: false}, fmt.Errorf("skillcreator: validate exec failed: %w", err)
}

// Dir returns the resolved skill-creator directory (for diagnostics).
func (v *Validator) Dir() string { return v.dir }

// minimalEnv returns an environment for the subprocess that excludes secrets
// (no GitHub PAT, no Anthropic key). Keeps PATH/HOME/LANG so python3 + PyYAML
// resolve correctly.
func minimalEnv() []string {
	keep := []string{"PATH", "HOME", "LANG", "LC_ALL", "TMPDIR"}
	var env []string
	for _, k := range keep {
		if val, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+val)
		}
	}
	return env
}
