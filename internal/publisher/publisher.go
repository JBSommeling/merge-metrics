package publisher

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	sentinelStart = "<!-- mergemetrics-start -->"
	sentinelEnd   = "<!-- mergemetrics-end -->"
)

// ReadmeUpdate holds the data needed to generate the MergeMetrics README section.
type ReadmeUpdate struct {
	HealthScore int
	Band        string
	PagesURL    string // e.g. "https://owner.github.io/repo/"
	UpdatedAt   time.Time
}

// badgeColor returns the shields.io color string for a given health band.
func badgeColor(band string) string {
	switch band {
	case "Excellent", "Healthy":
		return "green"
	case "Needs Attention":
		return "yellow"
	default:
		return "red"
	}
}

// generateSection builds the MergeMetrics markdown block including sentinels.
func generateSection(update *ReadmeUpdate) string {
	color := badgeColor(update.Band)
	pagesURL := update.PagesURL
	date := update.UpdatedAt.Format("2006-01-02")
	score := fmt.Sprintf("%d%%2F100", update.HealthScore)

	return fmt.Sprintf(`%s
[![Repository Health](https://img.shields.io/badge/Health-%s-%s)](%s)

📊 [View Repository Dashboard](%s)

Last updated: %s
%s`,
		sentinelStart,
		score,
		color,
		pagesURL,
		pagesURL,
		date,
		sentinelEnd,
	)
}

// UpdateReadme updates a README.md content string with MergeMetrics badge and link.
// Uses sentinel comments to safely replace only the MergeMetrics section.
// If sentinels don't exist, appends them at the end.
// Returns the updated content and whether changes were made.
func UpdateReadme(content string, update *ReadmeUpdate) (string, bool) {
	section := generateSection(update)

	startIdx := strings.Index(content, sentinelStart)
	endIdx := strings.Index(content, sentinelEnd)

	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		// Replace content between (and including) sentinels.
		before := content[:startIdx]
		after := content[endIdx+len(sentinelEnd):]

		newContent := before + section + after

		// Preserve trailing newline: if original ended with newline, ensure result does too.
		if strings.HasSuffix(content, "\n") && !strings.HasSuffix(newContent, "\n") {
			newContent += "\n"
		}

		if newContent == content {
			return content, false
		}
		return newContent, true
	}

	// No sentinels found — append at end.
	newContent := content
	if newContent != "" && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	if newContent != "" {
		newContent += "\n"
	}
	newContent += section + "\n"

	return newContent, true
}

// UpdateReadmeFile reads a README.md, applies the update, and writes it back.
// If the file doesn't exist, creates it with just the MergeMetrics section.
func UpdateReadmeFile(path string, update *ReadmeUpdate) error {
	var content string

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read readme: %w", err)
		}
		// File doesn't exist; start with empty content.
		content = ""
	} else {
		content = string(data)
	}

	updated, _ := UpdateReadme(content, update)

	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		return fmt.Errorf("write readme: %w", err)
	}
	return nil
}
