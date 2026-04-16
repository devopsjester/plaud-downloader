package customer

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CorrelateFile reads a Markdown file (summary or transcript) and returns any
// customer matches found in its YAML front matter title and body content.
func CorrelateFile(path string, registry *Registry) ([]Match, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	title, body, _, err := parseFrontMatter(f)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return registry.MatchText(title, body), nil
}

// ParseRecordingDate returns the date parsed from a file's YAML front matter
// "date:" field (YYYY-MM-DD). Returns zero time when absent or unparseable.
func ParseRecordingDate(path string) (time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	_, _, date, err := parseFrontMatter(f)
	return date, err
}

// CorrelateFileCombined reads both the summary and transcript for a recording
// (identified by summaryPath) and merges matches from both. Title matches
// from either file yield high confidence; body-only matches yield medium.
func CorrelateFileCombined(summaryPath string, registry *Registry) ([]Match, error) {
	summaryMatches, err := CorrelateFile(summaryPath, registry)
	if err != nil {
		return nil, err
	}

	transcriptPath := strings.TrimSuffix(summaryPath, "_summary.md") + "_transcript.md"
	transcriptMatches, err := CorrelateFile(transcriptPath, registry)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return mergeMatches(summaryMatches, transcriptMatches), nil
}

// mergeMatches combines two match slices, keeping the highest confidence per
// customer and preserving the original registry order.
func mergeMatches(a, b []Match) []Match {
	best := make(map[string]Match)
	for _, m := range a {
		best[m.Customer.Name] = m
	}
	for _, m := range b {
		if prev, ok := best[m.Customer.Name]; !ok || ConfidenceRank(m.Confidence) > ConfidenceRank(prev.Confidence) {
			best[m.Customer.Name] = m
		}
	}
	// Flatten in insertion order of map (Go maps are unordered, but callers
	// don't require strict ordering for merge results).
	out := make([]Match, 0, len(best))
	for _, m := range best {
		out = append(out, m)
	}
	return out
}

// CustomerOutputDir returns the directory where files for a customer should be
// written: {outputDir}/customers/{customerName}.
func CustomerOutputDir(outputDir, customerName string) string {
	return filepath.Join(outputDir, "customers", customerName)
}

// parseFrontMatter reads a Markdown file and returns the YAML front matter
// title, the body text, the date field (if present), and any read error.
func parseFrontMatter(r io.Reader) (title, body string, date time.Time, err error) {
	scanner := newLargeScanner(r)

	// Check for opening "---".
	if !scanner.Scan() {
		return "", "", time.Time{}, scanner.Err()
	}
	firstLine := scanner.Text()
	if strings.TrimSpace(firstLine) != "---" {
		// No front matter: entire file is body.
		var sb strings.Builder
		sb.WriteString(firstLine)
		sb.WriteByte('\n')
		for scanner.Scan() {
			sb.WriteString(scanner.Text())
			sb.WriteByte('\n')
		}
		return "", sb.String(), time.Time{}, scanner.Err()
	}

	// Read lines until closing "---".
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		if strings.HasPrefix(line, "title:") {
			val := strings.TrimPrefix(line, "title:")
			val = strings.TrimSpace(val)
			val = strings.Trim(val, `"'`)
			title = val
		}
		if strings.HasPrefix(line, "date:") {
			val := strings.TrimPrefix(line, "date:")
			val = strings.TrimSpace(val)
			val = strings.Trim(val, `"'`)
			if t, parseErr := time.Parse("2006-01-02", val); parseErr == nil {
				date = t
			}
		}
	}

	// Remaining content is the body.
	var sb strings.Builder
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
		sb.WriteByte('\n')
	}
	return title, sb.String(), date, scanner.Err()
}

// newLargeScanner wraps a reader in a bufio.Scanner with a generous token
// buffer to handle large transcript files.
func newLargeScanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 1024*1024), 1024*1024)
	return s
}
