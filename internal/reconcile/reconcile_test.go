package reconcile

import "testing"

func index() []SkillRef {
	return []SkillRef{
		{Name: "swift-actor-isolation", Description: "Handle actor isolation in Swift 6.", Path: "global-skills/apple/swift-actor-isolation/SKILL.md"},
		{Name: "lambda-go-lazy-init", Description: "Segregate sync.Once by use path in Go Lambdas.", Path: "global-skills/aws-go/lambda-go-lazy-init/SKILL.md"},
	}
}

func TestClassify_NovelWhenNoMatch(t *testing.T) {
	r := New(index())
	dec, ref := r.Classify("android-compose-paging", "Paginate Compose lists with Paging 3.")
	if dec != Novel {
		t.Fatalf("expected Novel for an unrelated skill, got %v", dec)
	}
	if ref.Name != "" {
		t.Fatalf("Novel must not return a matched ref, got %q", ref.Name)
	}
}

func TestClassify_DuplicateOnExactName(t *testing.T) {
	r := New(index())
	dec, ref := r.Classify("lambda-go-lazy-init", "Segregate sync.Once by use path in Go Lambdas.")
	if dec != Duplicate {
		t.Fatalf("expected Duplicate for an exact name match, got %v", dec)
	}
	if ref.Name != "lambda-go-lazy-init" {
		t.Fatalf("Duplicate must return the matched ref, got %q", ref.Name)
	}
}

func TestClassify_OverlapsOnSimilarName(t *testing.T) {
	r := New(index())
	// Same domain/topic, different exact name → should propose editing the
	// existing skill rather than creating a near-duplicate.
	dec, ref := r.Classify("lambda-go-lazy-init-segregated", "Segregate init by use path.")
	if dec != Overlaps {
		t.Fatalf("expected Overlaps for a near-duplicate name, got %v (ref=%q)", dec, ref.Name)
	}
	if ref.Name != "lambda-go-lazy-init" {
		t.Fatalf("Overlaps must point at the existing skill, got %q", ref.Name)
	}
}

func TestClassify_EmptyIndexAlwaysNovel(t *testing.T) {
	r := New(nil)
	dec, _ := r.Classify("anything", "any summary")
	if dec != Novel {
		t.Fatalf("empty index must yield Novel, got %v", dec)
	}
}
