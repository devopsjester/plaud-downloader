package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeHelperFile creates a temp file with the given content and returns its path.
func writeHelperFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

// ---------------------------------------------------------------------------
// buildSplitContent
// ---------------------------------------------------------------------------

func TestBuildSplitContent_RewritesTitleWithEmDash(t *testing.T) {
	t.Parallel()
	path := writeHelperFile(t, "rec_summary.md",
		"---\ntitle: \"Weekly review\"\ntype: summary\n---\nOriginal body.\n")

	got, err := buildSplitContent(path, "Acme", "Split body content.\n")
	if err != nil {
		t.Fatalf("buildSplitContent error: %v", err)
	}
	s := string(got)
	if !strings.Contains(s, `title: "Weekly review — Acme"`) {
		t.Errorf("title not rewritten with em dash; got:\n%s", s)
	}
}

func TestBuildSplitContent_AddsSourceRecording(t *testing.T) {
	t.Parallel()
	path := writeHelperFile(t, "rec_summary.md",
		"---\ntitle: \"Meeting\"\n---\nBody.\n")

	got, err := buildSplitContent(path, "Acme", "Split.\n")
	if err != nil {
		t.Fatalf("buildSplitContent error: %v", err)
	}
	s := string(got)
	wantField := "source_recording: rec_summary.md"
	if !strings.Contains(s, wantField) {
		t.Errorf("missing %q in output:\n%s", wantField, s)
	}
}

func TestBuildSplitContent_EmptyCustomerName(t *testing.T) {
	t.Parallel()
	path := writeHelperFile(t, "rec_summary.md",
		"---\ntitle: \"Meeting\"\n---\nBody.\n")

	got, err := buildSplitContent(path, "", "Split.\n")
	if err != nil {
		t.Fatalf("buildSplitContent error: %v", err)
	}
	s := string(got)
	if strings.Contains(s, "—") {
		t.Errorf("em dash should not be added for empty customer name; got:\n%s", s)
	}
	if !strings.Contains(s, `title: "Meeting"`) {
		t.Errorf("title should remain unchanged; got:\n%s", s)
	}
}

func TestBuildSplitContent_NoFrontMatter(t *testing.T) {
	t.Parallel()
	path := writeHelperFile(t, "rec_summary.md", "Just plain text.\n")
	splitBody := "My split body.\n"

	got, err := buildSplitContent(path, "Acme", splitBody)
	if err != nil {
		t.Fatalf("buildSplitContent error: %v", err)
	}
	// No front matter → return splitBody as-is.
	if string(got) != splitBody {
		t.Errorf("expected splitBody passthrough, got:\n%s", string(got))
	}
}

func TestBuildSplitContent_UnclosedFrontMatter(t *testing.T) {
	t.Parallel()
	// Front matter starts but has no closing "---".
	path := writeHelperFile(t, "rec_summary.md",
		"---\ntitle: \"Meeting\"\nBody without closing.\n")
	splitBody := "Fallback body.\n"

	got, err := buildSplitContent(path, "Acme", splitBody)
	if err != nil {
		t.Fatalf("buildSplitContent error: %v", err)
	}
	if string(got) != splitBody {
		t.Errorf("expected splitBody fallback for unclosed front matter, got:\n%s", string(got))
	}
}

func TestBuildSplitContent_SplitBodyAppended(t *testing.T) {
	t.Parallel()
	path := writeHelperFile(t, "rec_summary.md",
		"---\ntitle: \"Meeting\"\n---\nOld body.\n")
	splitBody := "New LLM body.\n"

	got, err := buildSplitContent(path, "Acme", splitBody)
	if err != nil {
		t.Fatalf("buildSplitContent error: %v", err)
	}
	s := string(got)
	if !strings.Contains(s, splitBody) {
		t.Errorf("split body not found in output:\n%s", s)
	}
	if strings.Contains(s, "Old body") {
		t.Errorf("original body should not appear in split output:\n%s", s)
	}
}

func TestBuildSplitContent_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := buildSplitContent("/nonexistent/file.md", "Acme", "body")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

// ---------------------------------------------------------------------------
// buildLeftoverContent
// ---------------------------------------------------------------------------

func TestBuildLeftoverContent_PreservesFrontMatter(t *testing.T) {
	t.Parallel()
	path := writeHelperFile(t, "rec_summary.md",
		"---\ntitle: \"Original meeting\"\ndate: 2026-02-24T10:00:00Z\n---\nOld body.\n")
	otherBody := "Leftover content.\n"

	got, err := buildLeftoverContent(path, otherBody)
	if err != nil {
		t.Fatalf("buildLeftoverContent error: %v", err)
	}
	s := string(got)
	if !strings.Contains(s, `title: "Original meeting"`) {
		t.Errorf("front matter title should be preserved; got:\n%s", s)
	}
	if !strings.Contains(s, "date: 2026-02-24T10:00:00Z") {
		t.Errorf("front matter date should be preserved; got:\n%s", s)
	}
}

func TestBuildLeftoverContent_ReplacesBody(t *testing.T) {
	t.Parallel()
	path := writeHelperFile(t, "rec_summary.md",
		"---\ntitle: \"Meeting\"\n---\nOriginal body.\n")
	otherBody := "Replaced body text.\n"

	got, err := buildLeftoverContent(path, otherBody)
	if err != nil {
		t.Fatalf("buildLeftoverContent error: %v", err)
	}
	s := string(got)
	if strings.Contains(s, "Original body") {
		t.Errorf("original body should be replaced; got:\n%s", s)
	}
	if !strings.Contains(s, otherBody) {
		t.Errorf("new body not found in output:\n%s", s)
	}
}

func TestBuildLeftoverContent_DoesNotAddSourceRecording(t *testing.T) {
	t.Parallel()
	path := writeHelperFile(t, "rec_summary.md",
		"---\ntitle: \"Meeting\"\n---\nBody.\n")

	got, err := buildLeftoverContent(path, "other body\n")
	if err != nil {
		t.Fatalf("buildLeftoverContent error: %v", err)
	}
	if strings.Contains(string(got), "source_recording:") {
		t.Errorf("leftover content must not add source_recording field; got:\n%s", string(got))
	}
}

func TestBuildLeftoverContent_DoesNotModifyTitle(t *testing.T) {
	t.Parallel()
	path := writeHelperFile(t, "rec_summary.md",
		"---\ntitle: \"Original title\"\n---\nBody.\n")

	got, err := buildLeftoverContent(path, "other\n")
	if err != nil {
		t.Fatalf("buildLeftoverContent error: %v", err)
	}
	s := string(got)
	if strings.Contains(s, "—") {
		t.Errorf("title must not be modified in leftover content; got:\n%s", s)
	}
}

func TestBuildLeftoverContent_NoFrontMatter(t *testing.T) {
	t.Parallel()
	path := writeHelperFile(t, "rec_summary.md", "Plain text file.\n")
	otherBody := "Leftover.\n"

	got, err := buildLeftoverContent(path, otherBody)
	if err != nil {
		t.Fatalf("buildLeftoverContent error: %v", err)
	}
	if string(got) != otherBody {
		t.Errorf("expected otherBody passthrough for no-front-matter; got:\n%s", string(got))
	}
}

func TestBuildLeftoverContent_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := buildLeftoverContent("/nonexistent/file.md", "body")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}
