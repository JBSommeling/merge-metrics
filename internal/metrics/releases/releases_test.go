package releases

import (
	"testing"
	"time"

	"github.com/jbsommeling/merge-metrics/internal/github"
)

// fixedNow is a stable reference point for all tests.
var fixedNow = time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

func TestAnalyze_NoReleases(t *testing.T) {
	result := analyzeAt(nil, fixedNow)

	if result.ReleasesLastMonth != 0 {
		t.Errorf("ReleasesLastMonth: got %d, want 0", result.ReleasesLastMonth)
	}
	if result.ReleasesLastWeek != 0 {
		t.Errorf("ReleasesLastWeek: got %d, want 0", result.ReleasesLastWeek)
	}
	if result.AvgTimeBetweenReleases != 0 {
		t.Errorf("AvgTimeBetweenReleases: got %v, want 0", result.AvgTimeBetweenReleases)
	}
	if result.TotalReleases != 0 {
		t.Errorf("TotalReleases: got %d, want 0", result.TotalReleases)
	}
	if result.DeployFrequencyScore != 0 {
		t.Errorf("DeployFrequencyScore: got %d, want 0", result.DeployFrequencyScore)
	}
}

func TestAnalyze_DailyReleases(t *testing.T) {
	// 10 releases, one per day, all within last 30 days.
	releases := make([]github.Release, 10)
	for i := 0; i < 10; i++ {
		releases[i] = github.Release{
			TagName:     "v1." + string(rune('0'+i)),
			PublishedAt: fixedNow.AddDate(0, 0, -i),
		}
	}

	result := analyzeAt(releases, fixedNow)

	if result.DeployFrequencyScore != 100 {
		t.Errorf("DeployFrequencyScore: got %d, want 100", result.DeployFrequencyScore)
	}
	if result.ReleasesLastMonth != 10 {
		t.Errorf("ReleasesLastMonth: got %d, want 10", result.ReleasesLastMonth)
	}
	if result.TotalReleases != 10 {
		t.Errorf("TotalReleases: got %d, want 10", result.TotalReleases)
	}

	// All 10 releases are in last 30 days; prior 30 days has 0 → trend is "increasing".
	if result.Trend != "increasing" {
		t.Errorf("Trend: got %q, want \"increasing\"", result.Trend)
	}
}

func TestAnalyze_WeeklyReleases(t *testing.T) {
	// Releases every 7 days over the past 70 days (11 releases).
	releases := make([]github.Release, 11)
	for i := 0; i < 11; i++ {
		releases[i] = github.Release{
			TagName:     "v1." + string(rune('0'+i)),
			PublishedAt: fixedNow.AddDate(0, 0, -7*i),
		}
	}

	result := analyzeAt(releases, fixedNow)

	if result.DeployFrequencyScore != 80 {
		t.Errorf("DeployFrequencyScore: got %d, want 80", result.DeployFrequencyScore)
	}

	// avg delta = 7 days exactly → score 80.
	expected := 7 * 24 * time.Hour
	if result.AvgTimeBetweenReleases != expected {
		t.Errorf("AvgTimeBetweenReleases: got %v, want %v", result.AvgTimeBetweenReleases, expected)
	}
}

func TestAnalyze_MonthlyReleases(t *testing.T) {
	// Releases every 15 days over 150 days (11 releases), avg = 15 days → score 50.
	releases := make([]github.Release, 11)
	for i := 0; i < 11; i++ {
		releases[i] = github.Release{
			TagName:     "v1." + string(rune('0'+i)),
			PublishedAt: fixedNow.AddDate(0, 0, -15*i),
		}
	}

	result := analyzeAt(releases, fixedNow)

	if result.DeployFrequencyScore != 50 {
		t.Errorf("DeployFrequencyScore: got %d, want 50", result.DeployFrequencyScore)
	}

	expected := 15 * 24 * time.Hour
	if result.AvgTimeBetweenReleases != expected {
		t.Errorf("AvgTimeBetweenReleases: got %v, want %v", result.AvgTimeBetweenReleases, expected)
	}
}

func TestAnalyze_FiltersPrereleases(t *testing.T) {
	releases := []github.Release{
		{TagName: "v1.0.0", PublishedAt: fixedNow.AddDate(0, 0, -1), Prerelease: false},
		{TagName: "v1.1.0-rc1", PublishedAt: fixedNow.AddDate(0, 0, -2), Prerelease: true},
		{TagName: "v1.1.0-rc2", PublishedAt: fixedNow.AddDate(0, 0, -3), Prerelease: true},
		{TagName: "v2.0.0", PublishedAt: fixedNow.AddDate(0, 0, -4), Prerelease: false},
	}

	result := analyzeAt(releases, fixedNow)

	// Only 2 real releases, pre-releases excluded.
	if result.TotalReleases != 2 {
		t.Errorf("TotalReleases: got %d, want 2", result.TotalReleases)
	}
	if result.ReleasesLastMonth != 2 {
		t.Errorf("ReleasesLastMonth: got %d, want 2", result.ReleasesLastMonth)
	}
	if result.ReleasesLastWeek != 2 {
		t.Errorf("ReleasesLastWeek: got %d, want 2", result.ReleasesLastWeek)
	}
}
