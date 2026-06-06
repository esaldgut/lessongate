package skillcreator

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRealPlugin_ValidatesAGenericSkill exercises the full chain against the
// actually-installed skill-creator plugin: glob discovery (no hardcoded hash),
// startup assert (python3 + PyYAML), and a real quick_validate.py run on a
// minimal valid SKILL.md. Skipped if the plugin or python3 isn't present.
func TestRealPlugin_ValidatesAGenericSkill(t *testing.T) {
	if os.Getenv("LESSONGATE_REAL_PLUGIN") == "" {
		t.Skip("set LESSONGATE_REAL_PLUGIN=1 to run against the installed plugin")
	}
	v, err := New(t.Context(), "") // "" → auto-discover by glob
	if err != nil {
		t.Skipf("skill-creator plugin / python3 not available: %v", err)
	}
	t.Logf("discovered plugin at: %s", v.Dir())

	// Build a minimal valid skill on disk.
	skill := t.TempDir()
	md := "---\n" +
		"name: example-generic-skill\n" +
		"description: A minimal valid skill for the integration test.\n" +
		"---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := v.Validate(t.Context(), skill)
	if err != nil {
		t.Fatalf("Validate infra error against real plugin: %v", err)
	}
	if !res.Valid {
		t.Fatalf("real plugin rejected a valid skill: %s", res.Message)
	}
	t.Logf("real plugin validated OK: %s", res.Message)
}
