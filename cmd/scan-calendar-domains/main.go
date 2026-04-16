// scan-calendar-domains — one-time helper that queries Google Calendar for
// events over the past 12 months, collects every attendee email domain, and
// reports which domains appear alongside each customer in customers.yaml.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

type Customer struct {
	Name    string   `yaml:"name"`
	Aliases []string `yaml:"aliases"`
}

type Registry struct {
	Customers []Customer `yaml:"customers"`
}

type googleEventDateTime struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
}

type googleAttendee struct {
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

type googleEvent struct {
	ID        string              `json:"id"`
	Summary   string              `json:"summary"`
	Start     googleEventDateTime `json:"start"`
	End       googleEventDateTime `json:"end"`
	Attendees []googleAttendee    `json:"attendees"`
}

type googleEventsResponse struct {
	Items         []googleEvent `json:"items"`
	NextPageToken string        `json:"nextPageToken"`
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: scan-calendar-domains <access-token> <customers.yaml>")
		os.Exit(1)
	}
	token := os.Args[1]
	customersFile := os.Args[2]

	// Load customer registry.
	raw, err := os.ReadFile(customersFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read customers file: %v\n", err)
		os.Exit(1)
	}
	var registry Registry
	if err := yaml.Unmarshal(raw, &registry); err != nil {
		fmt.Fprintf(os.Stderr, "parse customers file: %v\n", err)
		os.Exit(1)
	}

	// For each customer, build a lower-cased set of names+aliases for matching.
	type CustomerEntry struct {
		Name    string
		Aliases []string
		Domains map[string]int // domain → event count
	}
	customers := make([]*CustomerEntry, len(registry.Customers))
	for i, c := range registry.Customers {
		aliases := make([]string, 0, len(c.Aliases)+1)
		aliases = append(aliases, strings.ToLower(c.Name))
		for _, a := range c.Aliases {
			aliases = append(aliases, strings.ToLower(a))
		}
		customers[i] = &CustomerEntry{
			Name:    c.Name,
			Aliases: aliases,
			Domains: make(map[string]int),
		}
	}

	// Discover calendars so the user can see what's available.
	calendarID, err := findCalendarID(context.Background(), token, "GitHub Calendar")
	if err != nil {
		fmt.Fprintf(os.Stderr, "list calendars: %v\n", err)
		os.Exit(1)
	}
	if calendarID == "" {
		fmt.Fprintln(os.Stderr, "No calendar named \"GitHub Calendar\" found. Available calendars:")
		printCalendars(context.Background(), token)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Using calendar: GitHub Calendar (id=%s)\n", calendarID)

	// Fetch events since 2026-01-01.
	now := time.Now().UTC()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	events, err := listAllEventsFromCalendar(context.Background(), token, calendarID, from, now)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch calendar events: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Fetched %d events\n", len(events))

	// Own domains — skip these (Microsoft, GitHub, etc.)
	skipDomains := map[string]bool{
		"microsoft.com":  true,
		"github.com":     true,
		"gmail.com":      true,
		"googlemail.com": true,
		"outlook.com":    true,
		"hotmail.com":    true,
		"live.com":       true,
		"yahoo.com":      true,
	}

	// normalizeTitle replaces typographic variants so aliases match reliably.
	normalizeTitle := func(s string) string {
		// Replace non-breaking hyphens, en-dashes, em-dashes with regular hyphen.
		s = strings.ReplaceAll(s, "\u2011", "-") // non-breaking hyphen
		s = strings.ReplaceAll(s, "\u2013", "-") // en dash
		s = strings.ReplaceAll(s, "\u2014", "-") // em dash
		// Replace multiplication sign × with space (used as separator).
		s = strings.ReplaceAll(s, "\u00d7", " ")
		// Collapse multiple spaces.
		for strings.Contains(s, "  ") {
			s = strings.ReplaceAll(s, "  ", " ")
		}
		return strings.ToLower(strings.TrimSpace(s))
	}

	// For each event, check if its title contains a customer name.
	// If yes, collect all external attendee domains.
	var unmatchedSample []string
	for _, ev := range events {
		titleLower := normalizeTitle(ev.Summary)
		anyMatch := false

		for _, c := range customers {
			matched := false
			for _, alias := range c.Aliases {
				if containsWord(titleLower, alias) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			anyMatch = true
			extDomains := []string{}
			for _, att := range ev.Attendees {
				if att.Email == "" {
					continue
				}
				parts := strings.SplitN(att.Email, "@", 2)
				if len(parts) != 2 {
					continue
				}
				domain := strings.ToLower(strings.TrimSpace(parts[1]))
				if skipDomains[domain] {
					continue
				}
				extDomains = append(extDomains, domain)
				c.Domains[domain]++
			}
			fmt.Fprintf(os.Stderr, "  MATCH %-20s | %q | ext-attendees: %v\n",
				c.Name, ev.Summary, extDomains)
		}
		if !anyMatch {
			unmatchedSample = append(unmatchedSample, ev.Summary)
		}
	}

	// Print a sample of unmatched titles to help tune aliases.
	if len(unmatchedSample) > 0 {
		fmt.Fprintf(os.Stderr, "\nAll unmatched event titles (%d total):\n", len(unmatchedSample))
		for _, t := range unmatchedSample {
			fmt.Fprintf(os.Stderr, "  %s\n", t)
		}
	}

	// Print results.
	fmt.Println()
	for _, c := range customers {
		if len(c.Domains) == 0 {
			fmt.Printf("%-20s  (no external domains found in calendar events)\n", c.Name)
			continue
		}

		// Sort domains by frequency descending.
		type kv struct {
			Domain string
			Count  int
		}
		var sorted []kv
		for d, n := range c.Domains {
			sorted = append(sorted, kv{d, n})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Count > sorted[j].Count })

		fmt.Printf("%-20s\n", c.Name)
		for _, kv := range sorted {
			fmt.Printf("    %-40s  (%d event(s))\n", kv.Domain, kv.Count)
		}
	}
}

// containsWord returns true if text contains term bounded by non-alphanumeric
// characters (or start/end of string). Both text and term should be lower-cased.
func containsWord(text, term string) bool {
	if term == "" {
		return false
	}
	termRunes := []rune(term)
	textRunes := []rune(text)
	termLen := len(termRunes)
	textLen := len(textRunes)

	for i := 0; i <= textLen-termLen; i++ {
		if textRunes[i] != termRunes[0] {
			continue
		}
		if string(textRunes[i:i+termLen]) != term {
			continue
		}
		if i > 0 && isWordChar(textRunes[i-1]) {
			continue
		}
		end := i + termLen
		if end < textLen && isWordChar(textRunes[end]) {
			continue
		}
		return true
	}
	return false
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// findCalendarID returns the calendarId for the first calendar whose summary
// matches name (case-insensitive), or "" if not found.
func findCalendarID(ctx context.Context, token, name string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://www.googleapis.com/calendar/v3/users/me/calendarList", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("calendarList API returned %s", resp.Status)
	}

	var result struct {
		Items []struct {
			ID      string `json:"id"`
			Summary string `json:"summary"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	nameLower := strings.ToLower(name)
	for _, item := range result.Items {
		if strings.ToLower(item.Summary) == nameLower {
			return item.ID, nil
		}
	}
	return "", nil
}

// printCalendars prints all available calendar names to stderr.
func printCalendars(ctx context.Context, token string) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://www.googleapis.com/calendar/v3/users/me/calendarList", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var result struct {
		Items []struct {
			Summary string `json:"summary"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return
	}
	for _, item := range result.Items {
		fmt.Fprintf(os.Stderr, "  - %s\n", item.Summary)
	}
}

// listAllEventsFromCalendar fetches all events from a specific calendar between
// from and to, following pagination.
func listAllEventsFromCalendar(ctx context.Context, token, calendarID string, from, to time.Time) ([]googleEvent, error) {
	base := "https://www.googleapis.com/calendar/v3/calendars/" + url.PathEscape(calendarID) + "/events"
	var all []googleEvent
	pageToken := ""

	for {
		params := url.Values{}
		params.Set("timeMin", from.Format(time.RFC3339))
		params.Set("timeMax", to.Format(time.RFC3339))
		params.Set("singleEvents", "true")
		params.Set("orderBy", "startTime")
		params.Set("maxResults", "2500")
		if pageToken != "" {
			params.Set("pageToken", pageToken)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("calendar API returned %s", resp.Status)
		}

		var page googleEventsResponse
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			return nil, err
		}

		all = append(all, page.Items...)

		if page.NextPageToken == "" {
			break
		}
		pageToken = page.NextPageToken
	}

	return all, nil
}
