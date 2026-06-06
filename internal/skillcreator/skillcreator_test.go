package skillcreator

import (
	"os"
	"path/filepath"
	"testing"
)

// fakePlugin builds a throwaway skill-creator layout under a temp dir whose
// scripts/quick_validate.py is a stub we control, so the exit-code contract is
// tested without depending on the real installed plugin.
func fakePlugin(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	scripts := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "quick_validate.py"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newValidator(t *testing.T, script string) *Validator {
	t.Helper()
	dir := fakePlugin(t, script)
	v, err := New(t.Context(), dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return v
}

func TestValidate_ValidSkillExitsZero(t *testing.T) {
	// Stub prints a message and exits 0 → valid.
	v := newValidator(t, "import sys; print('Skill is valid!'); sys.exit(0)")
	res, err := v.Validate(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Validate returned infra error on a valid skill: %v", err)
	}
	if !res.Valid {
		t.Fatalf("expected valid, got: %+v", res)
	}
}

func TestValidate_InvalidSkillExitsOne(t *testing.T) {
	// Exit 1 → content rejection, NOT an infra error.
	v := newValidator(t, "import sys; print('Name is too long'); sys.exit(1)")
	res, err := v.Validate(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("exit 1 must be a rejection, not an error; got err=%v", err)
	}
	if res.Valid {
		t.Fatal("exit 1 must yield Valid=false")
	}
	if res.Message == "" {
		t.Fatal("expected the validator message to be captured")
	}
}

func TestValidate_CrashIsInfraErrorNotRejection(t *testing.T) {
	// Exit 2 (or a traceback) → infrastructure error, distinct from "invalid".
	v := newValidator(t, "import sys; sys.stderr.write('Traceback...'); sys.exit(2)")
	res, err := v.Validate(t.Context(), t.TempDir())
	if err == nil {
		t.Fatal("a crash (exit!=0,1) must be an infra error, not a clean rejection")
	}
	if res.Valid {
		t.Fatal("a crashed validation must never report Valid=true")
	}
}

func TestNew_FailsClosedWhenScriptMissing(t *testing.T) {
	// An override dir with no quick_validate.py must fail the startup assert.
	empty := t.TempDir()
	if _, err := New(t.Context(), empty); err == nil {
		t.Fatal("New must fail closed when quick_validate.py is absent")
	}
}

func TestDiscover_PrefersExplicitOverride(t *testing.T) {
	dir := fakePlugin(t, "import sys; sys.exit(0)")
	got, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover with override: %v", err)
	}
	if got != dir {
		t.Fatalf("override not honored: got %q want %q", got, dir)
	}
}
