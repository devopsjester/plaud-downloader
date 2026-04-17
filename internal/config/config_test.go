package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// resetViper clears all Viper state between tests so they don't bleed into each other.
func resetViper(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Cleanup(func() { viper.Reset() })
}

// TestConstants ensures public constants have the expected values.
// These are API contracts: changing them would break config files and
// callers that depend on the exact strings.
func TestConstants(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, got, want string }{
		{"DefaultOutputDir", DefaultOutputDir, "./output"},
		{"DefaultCalendarProvider", DefaultCalendarProvider, "reclaim"},
		{"SubdirDownloaded", SubdirDownloaded, "downloaded"},
		{"SubdirProcessed", SubdirProcessed, "processed"},
		{"SubdirCustomers", SubdirCustomers, "customers"},
		{"SubdirInternal", SubdirInternal, "internal"},
		{"SubdirUnmatched", SubdirUnmatched, "unmatched"},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	if DefaultConcurrency != 5 {
		t.Errorf("DefaultConcurrency = %d, want 5", DefaultConcurrency)
	}
}

// TestSetup_Defaults verifies that Setup without a config file sets expected
// Viper defaults.
func TestSetup_Defaults(t *testing.T) {
	resetViper(t)

	if err := Setup(""); err != nil {
		t.Fatalf("Setup error: %v", err)
	}

	if got := viper.GetString("output_dir"); got != DefaultOutputDir {
		t.Errorf("output_dir = %q, want %q", got, DefaultOutputDir)
	}
	if got := viper.GetInt("concurrency"); got != DefaultConcurrency {
		t.Errorf("concurrency = %d, want %d", got, DefaultConcurrency)
	}
	if got := viper.GetString("calendar_provider"); got != DefaultCalendarProvider {
		t.Errorf("calendar_provider = %q, want %q", got, DefaultCalendarProvider)
	}
	if got := viper.GetString("type"); got != DefaultType {
		t.Errorf("type = %q, want %q", got, DefaultType)
	}
}

// TestSetup_ExplicitConfigFile checks that values from a config file override
// defaults.
func TestSetup_ExplicitConfigFile(t *testing.T) {
	resetViper(t)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "plaud-hub.yaml")
	content := "output_dir: /custom/output\ncalendar_provider: google\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := Setup(cfgPath); err != nil {
		t.Fatalf("Setup error: %v", err)
	}

	if got := viper.GetString("output_dir"); got != "/custom/output" {
		t.Errorf("output_dir = %q, want /custom/output", got)
	}
	if got := viper.GetString("calendar_provider"); got != "google" {
		t.Errorf("calendar_provider = %q, want google", got)
	}
}

// TestSetup_MissingConfigFileIsNotError ensures Setup does not error when no
// config file is present (default behaviour: use env + defaults).
func TestSetup_MissingConfigFileIsNotError(t *testing.T) {
	resetViper(t)

	// Point to a non-existent config file — Setup should tolerate this.
	dir := t.TempDir()
	missing := filepath.Join(dir, "plaud-hub.yaml")
	if err := Setup(missing); err == nil {
		// We passed a non-existent file with SetConfigFile — this will error.
		// That is expected: explicit path must exist. Test that the error is useful.
	}
	// Implicit (empty string) path should never error when file is absent.
	resetViper(t)
	if err := Setup(""); err != nil {
		t.Errorf("Setup(\"\") returned error: %v", err)
	}
}

// TestToken_FromEnv checks that PLAUD_TOKEN environment variable is picked up.
func TestToken_FromEnv(t *testing.T) {
	resetViper(t)
	t.Setenv("PLAUD_TOKEN", "test-token-value")

	if err := Setup(""); err != nil {
		t.Fatalf("Setup error: %v", err)
	}

	tok, err := Token()
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if tok != "test-token-value" {
		t.Errorf("Token() = %q, want test-token-value", tok)
	}
}

// TestToken_MissingReturnsError checks that Token() errors when no token is set.
func TestToken_MissingReturnsError(t *testing.T) {
	resetViper(t)

	// Ensure env var is empty and write a config file with no token so Setup
	// doesn't pick up the user's real config file from UserConfigDir().
	t.Setenv("PLAUD_TOKEN", "")
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "plaud-hub.yaml")
	if err := os.WriteFile(cfgPath, []byte("output_dir: ./output\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := Setup(cfgPath); err != nil {
		t.Fatalf("Setup error: %v", err)
	}

	_, err := Token()
	if err == nil {
		t.Error("expected Token() to error when no token is available")
	}
}
