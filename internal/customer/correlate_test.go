package customer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTempSummary creates a temporary summary file with the given content
// and returns its path. Cleaned up automatically via t.Cleanup.
func writeTempSummary(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test_summary.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp summary: %v", err)
	}
	return path
}

// ---------------------------------------------------------------------------
// CorrelateFile
// ---------------------------------------------------------------------------

func TestCorrelateFile_TitleMatchIsHigh(t *testing.T) {
	reg := &Registry{Customers: []Customer{
		{Name: "Acme", Aliases: []string{"Acme Corp"}},
	}}
	path := writeTempSummary(t, "---\ntitle: \"Acme quarterly review\"\n---\nSome body text.\n")

	matches, err := CorrelateFile(path, reg)
	if err != nil {
		t.Fatalf("CorrelateFile error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	if matches[0].Confidence != ConfidenceHigh {
		t.Errorf("confidence = %q, want %q", matches[0].Confidence, ConfidenceHigh)
	}
	if matches[0].Customer.Name != "Acme" {
		t.Errorf("customer name = %q, want %q", matches[0].Customer.Name, "Acme")
	}
}

func TestCorrelateFile_BodyMatchIsMedium(t *testing.T) {
	reg := &Registry{Customers: []Customer{
		{Name: "Acme"},
	}}
	path := writeTempSummary(t, "---\ntitle: \"Weekly sync\"\n---\nWe discussed Acme's roadmap.\n")

	matches, err := CorrelateFile(path, reg)
	if err != nil {
		t.Fatalf("CorrelateFile error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	if matches[0].Confidence != ConfidenceMedium {
		t.Errorf("confidence = %q, want %q", matches[0].Confidence, ConfidenceMedium)
	}
}

func TestCorrelateFile_AliasMatch(t *testing.T) {
	reg := &Registry{Customers: []Customer{
		{Name: "McDonalds", Aliases: []string{"McDonald's", "mcd"}},
	}}
	path := writeTempSummary(t, "---\ntitle: \"Meeting notes\"\n---\nThe McDonald's rollout plan was reviewed.\n")

	matches, err := CorrelateFile(path, reg)
	if err != nil {
		t.Fatalf("CorrelateFile error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	if matches[0].Customer.Name != "McDonalds" {
		t.Errorf("customer = %q, want McDonalds", matches[0].Customer.Name)
	}
}

func TestCorrelateFile_NoMatch(t *testing.T) {
	reg := &Registry{Customers: []Customer{
		{Name: "Acme"},
	}}
	path := writeTempSummary(t, "---\ntitle: \"Internal team sync\"\n---\nNo external customers present.\n")

	matches, err := CorrelateFile(path, reg)
	if err != nil {
		t.Fatalf("CorrelateFile error: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("got %d matches, want 0", len(matches))
	}
}

func TestCorrelateFile_MultipleCustomers(t *testing.T) {
	reg := &Registry{Customers: []Customer{
		{Name: "Acme"},
		{Name: "Globex"},
	}}
	path := writeTempSummary(t, "---\ntitle: \"Acme and Globex joint session\"\n---\nBoth teams were present.\n")

	matches, err := CorrelateFile(path, reg)
	if err != nil {
		t.Fatalf("CorrelateFile error: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2", len(matches))
	}
}

func TestCorrelateFile_MissingFile(t *testing.T) {
	reg := &Registry{Customers: []Customer{{Name: "Acme"}}}
	_, err := CorrelateFile("/nonexistent/path/file.md", reg)
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestCorrelateFile_NoFrontMatter(t *testing.T) {
	reg := &Registry{Customers: []Customer{{Name: "Acme"}}}
	// File has no front matter — the whole text is the body.
	path := writeTempSummary(t, "We discussed Acme's plans today.\n")

	matches, err := CorrelateFile(path, reg)
	if err != nil {
		t.Fatalf("CorrelateFile error: %v", err)
	}
	// Body match → medium confidence.
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	if matches[0].Confidence != ConfidenceMedium {
		t.Errorf("confidence = %q, want medium", matches[0].Confidence)
	}
}

// ---------------------------------------------------------------------------
// ParseRecordingInfo
// ---------------------------------------------------------------------------

func TestParseRecordingInfo_FullFrontMatter(t *testing.T) {
	content := "---\nrecording_id: abc123\ndate: 2026-02-24T10:29:15-05:00\nduration: \"58:00\"\ntitle: \"Planning meeting\"\ntype: summary\n---\n\nMeeting body here.\n"
	path := writeTempSummary(t, content)

	info, err := ParseRecordingInfo(path)
	if err != nil {
		t.Fatalf("ParseRecordingInfo error: %v", err)
	}
	if info.Title != "Planning meeting" {
		t.Errorf("Title = %q, want \"Planning meeting\"", info.Title)
	}
	if info.Start.IsZero() {
		t.Error("Start should not be zero")
	}
	wantYear, wantMonth, wantDay := 2026, time.February, 24
	if info.Start.Year() != wantYear || info.Start.Month() != time.Month(wantMonth) || info.Start.Day() != wantDay {
		t.Errorf("Start = %v, want 2026-02-24", info.Start)
	}
	if info.Duration != 58*time.Minute {
		t.Errorf("Duration = %v, want 58m", info.Duration)
	}
	if info.Body == "" {
		t.Error("Body should not be empty")
	}
}

func TestParseRecordingInfo_NoFrontMatter(t *testing.T) {
	path := writeTempSummary(t, "Just a plain body.\n")

	info, err := ParseRecordingInfo(path)
	if err != nil {
		t.Fatalf("ParseRecordingInfo error: %v", err)
	}
	if !info.Start.IsZero() {
		t.Errorf("Start should be zero for file without front matter, got %v", info.Start)
	}
	if info.Body == "" {
		t.Error("Body should contain the file text")
	}
}

func TestParseRecordingInfo_MissingFile(t *testing.T) {
	_, err := ParseRecordingInfo("/nonexistent/file.md")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestParseRecordingInfo_End(t *testing.T) {
	start := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	info := RecordingInfo{Start: start, Duration: 30 * time.Minute}
	end := info.End()
	want := start.Add(30 * time.Minute)
	if !end.Equal(want) {
		t.Errorf("End() = %v, want %v", end, want)
	}
}

func TestParseRecordingInfo_EndNoDuration(t *testing.T) {
	start := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	info := RecordingInfo{Start: start}
	if !info.End().Equal(start) {
		t.Errorf("End() with zero duration should equal Start, got %v", info.End())
	}
}

// ---------------------------------------------------------------------------
// Output directory helpers
// ---------------------------------------------------------------------------

func TestCustomerOutputDir(t *testing.T) {
	t.Parallel()
	root := "/output"
	tm := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	got := CustomerOutputDir(root, "Acme", tm)
	want := filepath.Join("/output", "processed", "customers", "Acme", "2026-02")
	if got != want {
		t.Errorf("CustomerOutputDir = %q, want %q", got, want)
	}
}

func TestInternalOutputDir(t *testing.T) {
	t.Parallel()
	root := "/output"
	tm := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	got := InternalOutputDir(root, tm)
	want := filepath.Join("/output", "processed", "internal", "2026-03")
	if got != want {
		t.Errorf("InternalOutputDir = %q, want %q", got, want)
	}
}

func TestUnmatchedOutputDir(t *testing.T) {
	t.Parallel()
	root := "/output"
	tm := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	got := UnmatchedOutputDir(root, tm)
	want := filepath.Join("/output", "processed", "unmatched", "2026-01")
	if got != want {
		t.Errorf("UnmatchedOutputDir = %q, want %q", got, want)
	}
}

func TestDownloadedDir(t *testing.T) {
	t.Parallel()
	got := DownloadedDir("/output")
	want := filepath.Join("/output", "downloaded")
	if got != want {
		t.Errorf("DownloadedDir = %q, want %q", got, want)
	}
}

func TestCustomerOutputDir_YearRollover(t *testing.T) {
	t.Parallel()
	tm := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
	got := CustomerOutputDir("/out", "Globex", tm)
	want := filepath.Join("/out", "processed", "customers", "Globex", "2026-12")
	if got != want {
		t.Errorf("CustomerOutputDir = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// MatchDomain
// ---------------------------------------------------------------------------

func TestMatchDomain_Hit(t *testing.T) {
	reg := &Registry{Customers: []Customer{
		{Name: "Acme", Domains: []string{"acme.com", "acme.co.uk"}},
	}}
	c := reg.MatchDomain("acme.com")
	if c == nil {
		t.Fatal("MatchDomain returned nil, want Acme")
	}
	if c.Name != "Acme" {
		t.Errorf("customer name = %q, want Acme", c.Name)
	}
}

func TestMatchDomain_CaseInsensitive(t *testing.T) {
	reg := &Registry{Customers: []Customer{
		{Name: "Acme", Domains: []string{"ACME.COM"}},
	}}
	c := reg.MatchDomain("acme.com")
	if c == nil {
		t.Fatal("MatchDomain returned nil for case-insensitive match")
	}
}

func TestMatchDomain_Miss(t *testing.T) {
	reg := &Registry{Customers: []Customer{
		{Name: "Acme", Domains: []string{"acme.com"}},
	}}
	c := reg.MatchDomain("other.com")
	if c != nil {
		t.Errorf("MatchDomain returned %q, want nil", c.Name)
	}
}

func TestMatchDomain_Empty(t *testing.T) {
	reg := &Registry{Customers: []Customer{
		{Name: "Acme", Domains: []string{"acme.com"}},
	}}
	c := reg.MatchDomain("")
	if c != nil {
		t.Errorf("MatchDomain(\"\") returned %q, want nil", c.Name)
	}
}
