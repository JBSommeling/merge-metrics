package publisher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testTime = time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)

var baseUpdate = &ReadmeUpdate{
	HealthScore: 82,
	Band:        "Healthy",
	PagesURL:    "https://owner.github.io/repo/",
	UpdatedAt:   testTime,
}

// TestUpdateReadme_NoSentinels verifies that when no sentinels exist, the section
// is appended at the end with a blank line separator.
func TestUpdateReadme_NoSentinels(t *testing.T) {
	content := "# My Repo\n\nSome existing content.\n"
	result, changed := UpdateReadme(content, baseUpdate)

	if !changed {
		t.Fatal("expected changed=true")
	}
	if !strings.HasPrefix(result, content) {
		t.Errorf("expected result to start with original content\ngot:\n%s", result)
	}
	if !strings.Contains(result, sentinelStart) {
		t.Error("expected sentinel start in result")
	}
	if !strings.Contains(result, sentinelEnd) {
		t.Error("expected sentinel end in result")
	}
	if !strings.HasSuffix(result, "\n") {
		t.Error("expected result to end with newline")
	}
}

// TestUpdateReadme_ExistingSentinels verifies that when sentinels already exist,
// the content between them is replaced.
func TestUpdateReadme_ExistingSentinels(t *testing.T) {
	content := "# My Repo\n\n<!-- mergemetrics-start -->\nold content\n<!-- mergemetrics-end -->\n"
	result, changed := UpdateReadme(content, baseUpdate)

	if !changed {
		t.Fatal("expected changed=true")
	}
	if strings.Contains(result, "old content") {
		t.Error("expected old content to be replaced")
	}
	if !strings.Contains(result, "82%2F100") {
		t.Error("expected new badge score in result")
	}
	if !strings.HasSuffix(result, "\n") {
		t.Error("expected result to end with newline")
	}
}

// TestUpdateReadme_PreservesUserContent verifies content before and after sentinels
// is not modified.
func TestUpdateReadme_PreservesUserContent(t *testing.T) {
	before := "# My Repo\n\nThis is important user content.\n\n"
	after := "\n\nMore content after the badge.\n"
	content := before + sentinelStart + "\nold\n" + sentinelEnd + after

	result, changed := UpdateReadme(content, baseUpdate)

	if !changed {
		t.Fatal("expected changed=true")
	}
	if !strings.HasPrefix(result, before) {
		t.Errorf("content before sentinels was modified\ngot prefix:\n%q", result[:len(before)+10])
	}
	if !strings.HasSuffix(result, after) {
		t.Errorf("content after sentinels was modified\ngot suffix:\n%q", result[len(result)-len(after)-5:])
	}
}

// TestUpdateReadme_EmptyReadme verifies that an empty string is handled gracefully.
func TestUpdateReadme_EmptyReadme(t *testing.T) {
	result, changed := UpdateReadme("", baseUpdate)

	if !changed {
		t.Fatal("expected changed=true")
	}
	if !strings.Contains(result, sentinelStart) {
		t.Error("expected sentinel start in result")
	}
	if !strings.Contains(result, sentinelEnd) {
		t.Error("expected sentinel end in result")
	}
	if !strings.HasSuffix(result, "\n") {
		t.Error("expected result to end with newline")
	}
}

// TestUpdateReadme_BadgeColorByBand verifies the correct shield color for each band.
func TestUpdateReadme_BadgeColorByBand(t *testing.T) {
	cases := []struct {
		band          string
		expectedColor string
	}{
		{"Excellent", "green"},
		{"Healthy", "green"},
		{"Needs Attention", "yellow"},
		{"High Risk", "red"},
		{"Unknown", "red"},
	}

	for _, tc := range cases {
		t.Run(tc.band, func(t *testing.T) {
			u := &ReadmeUpdate{
				HealthScore: 50,
				Band:        tc.band,
				PagesURL:    "https://example.com/",
				UpdatedAt:   testTime,
			}
			result, _ := UpdateReadme("", u)
			// Badge URL is nested: [![alt](badge-url)](link-url), so color appears as "-color)]("
			if !strings.Contains(result, "-"+tc.expectedColor+")](") {
				t.Errorf("band %q: expected color %q in badge URL\ngot:\n%s", tc.band, tc.expectedColor, result)
			}
		})
	}
}

// TestUpdateReadmeFile_CreatesFile verifies that a README is created if it doesn't exist.
func TestUpdateReadmeFile_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")

	if err := UpdateReadmeFile(path, baseUpdate); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, sentinelStart) {
		t.Error("expected sentinel start in created file")
	}
	if !strings.Contains(content, "82%2F100") {
		t.Error("expected badge score in created file")
	}
}

// TestUpdateReadmeFile_UpdatesExistingFile verifies that an existing README is modified correctly.
func TestUpdateReadmeFile_UpdatesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")

	initial := "# Existing Repo\n\nSome content.\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if err := UpdateReadmeFile(path, baseUpdate); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read file: %v", err)
	}
	content := string(data)

	if !strings.HasPrefix(content, initial) {
		t.Error("existing content was not preserved at the top")
	}
	if !strings.Contains(content, sentinelStart) {
		t.Error("expected sentinel start in updated file")
	}
	if !strings.Contains(content, "82%2F100") {
		t.Error("expected badge score in updated file")
	}
}
