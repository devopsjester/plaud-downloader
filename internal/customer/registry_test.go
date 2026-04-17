package customer

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTempRegistry writes a YAML customers file to a temp dir and returns its path.
func writeTempRegistry(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "customers.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp registry: %v", err)
	}
	return path
}

// ---------------------------------------------------------------------------
// LoadRegistry
// ---------------------------------------------------------------------------

func TestLoadRegistry_Valid(t *testing.T) {
	yaml := `customers:
  - name: Acme
    aliases: [Acme Corp]
    domains: [acme.com]
    gainsight_name: "Acme Inc"
  - name: Globex
`
	path := writeTempRegistry(t, yaml)
	reg, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("LoadRegistry error: %v", err)
	}
	if len(reg.Customers) != 2 {
		t.Fatalf("got %d customers, want 2", len(reg.Customers))
	}
	if reg.Customers[0].Name != "Acme" {
		t.Errorf("customers[0].Name = %q, want Acme", reg.Customers[0].Name)
	}
	if reg.Customers[0].GainsightName != "Acme Inc" {
		t.Errorf("customers[0].GainsightName = %q, want \"Acme Inc\"", reg.Customers[0].GainsightName)
	}
	if len(reg.Customers[0].Aliases) != 1 || reg.Customers[0].Aliases[0] != "Acme Corp" {
		t.Errorf("customers[0].Aliases = %v, want [Acme Corp]", reg.Customers[0].Aliases)
	}
}

func TestLoadRegistry_EmptyList(t *testing.T) {
	path := writeTempRegistry(t, "customers: []\n")
	reg, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("LoadRegistry error: %v", err)
	}
	if len(reg.Customers) != 0 {
		t.Errorf("got %d customers, want 0", len(reg.Customers))
	}
}

func TestLoadRegistry_PathTraversal(t *testing.T) {
	_, err := LoadRegistry("../../../etc/passwd")
	if err == nil {
		t.Error("expected error for path-traversal path, got nil")
	}
}

func TestLoadRegistry_Missing(t *testing.T) {
	_, err := LoadRegistry("/nonexistent/customers.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestLoadRegistry_InvalidYAML(t *testing.T) {
	path := writeTempRegistry(t, "this: is: not: valid: yaml: [\n")
	_, err := LoadRegistry(path)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

// ---------------------------------------------------------------------------
// MatchText
// ---------------------------------------------------------------------------

func makeTestRegistry(t *testing.T) *Registry {
	t.Helper()
	return &Registry{Customers: []Customer{
		{Name: "Acme", Aliases: []string{"Acme Corp"}, Domains: []string{"acme.com"}},
		{Name: "Globex", Aliases: []string{"GlbX"}},
	}}
}

func TestMatchText_TitleMatchHigh(t *testing.T) {
	t.Parallel()
	reg := makeTestRegistry(t)
	matches := reg.MatchText("Acme quarterly review", "some unrelated body")
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	if matches[0].Confidence != ConfidenceHigh {
		t.Errorf("confidence = %q, want high", matches[0].Confidence)
	}
	if matches[0].Customer.Name != "Acme" {
		t.Errorf("customer = %q, want Acme", matches[0].Customer.Name)
	}
}

func TestMatchText_BodyMatchMedium(t *testing.T) {
	t.Parallel()
	reg := makeTestRegistry(t)
	matches := reg.MatchText("Weekly sync", "We discussed Acme's expansion plans.")
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	if matches[0].Confidence != ConfidenceMedium {
		t.Errorf("confidence = %q, want medium", matches[0].Confidence)
	}
}

func TestMatchText_AliasMatchHigh(t *testing.T) {
	t.Parallel()
	reg := makeTestRegistry(t)
	matches := reg.MatchText("Acme Corp kickoff", "")
	if len(matches) == 0 {
		t.Fatal("got no matches, want Acme via alias")
	}
	if matches[0].Confidence != ConfidenceHigh {
		t.Errorf("alias confidence = %q, want high", matches[0].Confidence)
	}
}

func TestMatchText_NoMatch(t *testing.T) {
	t.Parallel()
	reg := makeTestRegistry(t)
	matches := reg.MatchText("Internal planning session", "No external customers here.")
	if len(matches) != 0 {
		t.Errorf("got %d matches, want 0", len(matches))
	}
}

func TestMatchText_MultipleCustomers(t *testing.T) {
	t.Parallel()
	reg := makeTestRegistry(t)
	matches := reg.MatchText("Acme and Globex collaboration", "Joint session notes.")
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2", len(matches))
	}
}

func TestMatchText_TitleBeatBody(t *testing.T) {
	t.Parallel()
	// Acme appears in both title AND body — should be high (not medium).
	reg := makeTestRegistry(t)
	matches := reg.MatchText("Acme review", "Acme had good things to say.")
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	if matches[0].Confidence != ConfidenceHigh {
		t.Errorf("confidence = %q, want high (title beats body)", matches[0].Confidence)
	}
}

func TestMatchText_CaseInsensitive(t *testing.T) {
	t.Parallel()
	reg := makeTestRegistry(t)
	matches := reg.MatchText("ACME planning", "")
	if len(matches) == 0 {
		t.Error("case-insensitive match failed for ACME/Acme")
	}
}

func TestMatchText_EmptyInput(t *testing.T) {
	t.Parallel()
	reg := makeTestRegistry(t)
	matches := reg.MatchText("", "")
	if len(matches) != 0 {
		t.Errorf("got %d matches for empty input, want 0", len(matches))
	}
}

// ---------------------------------------------------------------------------
// allTerms (via MatchText behaviour)
// ---------------------------------------------------------------------------

func TestAllTerms_IncludesAliases(t *testing.T) {
	t.Parallel()
	c := Customer{Name: "McDonald's", Aliases: []string{"McD", ""}}
	terms := c.allTerms()
	// Should include name + non-empty alias, skip empty alias.
	if len(terms) != 2 {
		t.Errorf("allTerms() len = %d, want 2 (name + 1 non-empty alias)", len(terms))
	}
}
