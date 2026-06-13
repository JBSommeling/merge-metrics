package dashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jbsommeling/merge-metrics/internal/metrics/busfactor"
	"github.com/jbsommeling/merge-metrics/internal/metrics/health"
	"github.com/jbsommeling/merge-metrics/internal/metrics/prbottleneck"
	"github.com/jbsommeling/merge-metrics/internal/metrics/releases"
	"github.com/jbsommeling/merge-metrics/internal/metrics/reviews"
)

func sampleData() *DashboardData {
	return &DashboardData{
		RepoName:     "merge-metrics",
		RepoFullName: "jbsommeling/merge-metrics",
		GeneratedAt:  time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC),
		HealthScore: &health.Result{
			TotalScore: 82,
			Band:       "Healthy",
			Categories: []health.ScoreCategory{
				{Name: "Review Speed", Score: 90, Weight: 0.2, Contribution: 18, Detail: "Median review time: 2 hours"},
				{Name: "Bus Factor", Score: 75, Weight: 0.15, Contribution: 11.25, Detail: "Bus factor: 2"},
			},
			Recommendations: []string{"Increase bus factor - encourage broader code ownership"},
		},
		PRBottleneck: &prbottleneck.Result{
			OpenPRCount:      5,
			AveragePRAgeDays: 3.5,
			StalePRs: []prbottleneck.StalePR{
				{Number: 42, Title: "Add feature X", Author: "alice", OpenDays: 10, Status: "waiting_for_review"},
			},
			WaitingForReview: 3,
			WaitingForAuthor: 1,
			ReviewerWorkload: []prbottleneck.ReviewerLoad{
				{Login: "alice", PendingCount: 3, IsBottleneck: false},
				{Login: "bob", PendingCount: 8, IsBottleneck: true},
			},
			Bottlenecks: []string{"bob has 8 pending reviews (median: 3)"},
		},
		BusFactor: &busfactor.Result{
			Contributors: []busfactor.ContributorShare{
				{Login: "alice", Commits: 60, SharePct: 60.0},
				{Login: "bob", Commits: 40, SharePct: 40.0},
			},
			BusFactor:          2,
			HHI:                0.52,
			TotalCommits:       100,
			AnalysisPeriodDays: 90,
		},
		Reviews: &reviews.Result{
			AverageReviewTime: 6 * time.Hour,
			MedianReviewTime:  4*time.Hour + 30*time.Minute,
			P90ReviewTime:     24 * time.Hour,
			TotalReviews:      20,
			ReviewThroughput:  3.5,
			ReviewerStats: []reviews.ReviewerStat{
				{Login: "alice", AvgResponseTime: 4 * time.Hour, ReviewCount: 12, ReviewsPerWeek: 2.0},
				{Login: "bob", AvgResponseTime: 9 * time.Hour, ReviewCount: 8, ReviewsPerWeek: 1.5},
			},
		},
		Releases: &releases.Result{
			ReleasesLastMonth:      8,
			ReleasesLastWeek:       2,
			AvgTimeBetweenReleases: 4 * 24 * time.Hour,
			Trend:                  "stable",
			DeployFrequencyScore:   80,
			TotalReleases:          24,
		},
	}
}

// TestGenerate_CreatesFiles verifies all four expected files are created.
func TestGenerate_CreatesFiles(t *testing.T) {
	dir, err := os.MkdirTemp("", "dashboard-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	if err := Generate(sampleData(), dir); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	expected := []string{
		filepath.Join(dir, "data.json"),
		filepath.Join(dir, "metrics.json"),
		filepath.Join(dir, "assets", "style.css"),
		filepath.Join(dir, "index.html"),
	}
	for _, path := range expected {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file not found: %s", path)
		}
	}
}

// TestGenerate_ValidJSON verifies data.json and metrics.json parse correctly with expected keys.
func TestGenerate_ValidJSON(t *testing.T) {
	dir, err := os.MkdirTemp("", "dashboard-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	data := sampleData()
	if err := Generate(data, dir); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Verify data.json
	dataBytes, err := os.ReadFile(filepath.Join(dir, "data.json"))
	if err != nil {
		t.Fatalf("read data.json: %v", err)
	}
	var dataMap map[string]interface{}
	if err := json.Unmarshal(dataBytes, &dataMap); err != nil {
		t.Fatalf("parse data.json: %v", err)
	}
	if dataMap["repo_name"] != "merge-metrics" {
		t.Errorf("data.json repo_name = %v, want merge-metrics", dataMap["repo_name"])
	}
	if dataMap["repo_full_name"] != "jbsommeling/merge-metrics" {
		t.Errorf("data.json repo_full_name = %v, want jbsommeling/merge-metrics", dataMap["repo_full_name"])
	}
	if _, ok := dataMap["health_score"]; !ok {
		t.Error("data.json missing health_score key")
	}

	// Verify metrics.json
	metricBytes, err := os.ReadFile(filepath.Join(dir, "metrics.json"))
	if err != nil {
		t.Fatalf("read metrics.json: %v", err)
	}
	var m metricsJSON
	if err := json.Unmarshal(metricBytes, &m); err != nil {
		t.Fatalf("parse metrics.json: %v", err)
	}
	if m.HealthScore != 82 {
		t.Errorf("metrics.json health_score = %d, want 82", m.HealthScore)
	}
	if m.HealthBand != "Healthy" {
		t.Errorf("metrics.json health_band = %s, want Healthy", m.HealthBand)
	}
	if m.OpenPRs != 5 {
		t.Errorf("metrics.json open_prs = %d, want 5", m.OpenPRs)
	}
	if m.StalePRs != 1 {
		t.Errorf("metrics.json stale_prs = %d, want 1", m.StalePRs)
	}
	if m.BusFactor != 2 {
		t.Errorf("metrics.json bus_factor = %d, want 2", m.BusFactor)
	}
	if m.ReleasesLastMonth != 8 {
		t.Errorf("metrics.json releases_last_month = %d, want 8", m.ReleasesLastMonth)
	}
	if m.DeployFrequencyScore != 80 {
		t.Errorf("metrics.json deploy_frequency_score = %d, want 80", m.DeployFrequencyScore)
	}
}

// TestGenerate_ValidHTML verifies index.html contains the key section headings.
func TestGenerate_ValidHTML(t *testing.T) {
	dir, err := os.MkdirTemp("", "dashboard-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	if err := Generate(sampleData(), dir); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	htmlBytes, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(htmlBytes)

	checks := []struct {
		name    string
		contain string
	}{
		{"title", "MergeMetrics"},
		{"repo name", "merge-metrics"},
		{"health score section", "Health Score"},
		{"PR bottlenecks section", "PR Bottlenecks"},
		{"review analytics section", "Review Analytics"},
		{"contributors section", "Contributors"},
		{"releases section", "Releases"},
		{"recommendations section", "Recommendations"},
		{"stylesheet link", "assets/style.css"},
	}
	for _, c := range checks {
		if !strings.Contains(html, c.contain) {
			t.Errorf("index.html missing %s: expected to contain %q", c.name, c.contain)
		}
	}
}

// TestGenerate_EmptyData verifies that nil metric fields do not cause a panic.
func TestGenerate_EmptyData(t *testing.T) {
	dir, err := os.MkdirTemp("", "dashboard-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	empty := &DashboardData{
		RepoName:     "empty-repo",
		RepoFullName: "org/empty-repo",
		GeneratedAt:  time.Now(),
		// All metric fields are nil
	}

	if err := Generate(empty, dir); err != nil {
		t.Fatalf("Generate with empty data returned error: %v", err)
	}

	// Verify files still exist
	for _, name := range []string{"data.json", "metrics.json", "index.html"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file not created for empty data: %s", name)
		}
	}

	// Verify metrics.json has zero values but still parses
	metricBytes, err := os.ReadFile(filepath.Join(dir, "metrics.json"))
	if err != nil {
		t.Fatalf("read metrics.json: %v", err)
	}
	var m metricsJSON
	if err := json.Unmarshal(metricBytes, &m); err != nil {
		t.Fatalf("parse metrics.json for empty data: %v", err)
	}
	if m.HealthScore != 0 {
		t.Errorf("empty data metrics.json health_score = %d, want 0", m.HealthScore)
	}
}
