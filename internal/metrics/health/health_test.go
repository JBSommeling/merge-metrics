package health

import (
	"testing"
	"time"

	"github.com/jbsommeling/merge-metrics/internal/config"
	"github.com/jbsommeling/merge-metrics/internal/metrics/busfactor"
	"github.com/jbsommeling/merge-metrics/internal/metrics/prbottleneck"
	"github.com/jbsommeling/merge-metrics/internal/metrics/releases"
	"github.com/jbsommeling/merge-metrics/internal/metrics/reviews"
)

// defaultWeights returns the standard weight configuration used across tests.
func defaultWeights() config.WeightConfig {
	return config.WeightConfig{
		ReviewSpeed:          0.20,
		BusFactor:            0.20,
		PRHygiene:            0.15,
		PRSize:               0.15,
		IssueHealth:          0.10,
		DeployFrequency:      0.10,
		ContributorDiversity: 0.10,
	}
}

// defaultThresholds returns the standard threshold configuration.
func defaultThresholds() config.ThresholdConfig {
	return config.ThresholdConfig{
		StalePRDays:              7,
		ReviewTimeExcellentHours: 4,
		ReviewTimePoorDays:       7,
		BusFactorGood:            3,
		PRSizeGood:               200,
		PRSizePoor:               1000,
	}
}

// perfectInput returns an Input where every metric is ideal.
func perfectInput() *Input {
	return &Input{
		PRBottleneck: &prbottleneck.Result{
			StalePRs: nil,
		},
		BusFactor: &busfactor.Result{
			BusFactor: 6,
			HHI:       0.05, // well below 0.15 excellent threshold
		},
		Reviews: &reviews.Result{
			MedianReviewTime: 2 * time.Hour, // below 4h excellent threshold
		},
		Releases: &releases.Result{
			DeployFrequencyScore: 100,
		},
		OpenIssues:   5,  // below 10 excellent threshold
		MedianPRSize: 50, // well below 200 good threshold
	}
}

// poorInput returns an Input where every metric is as bad as possible.
func poorInput() *Input {
	return &Input{
		PRBottleneck: &prbottleneck.Result{
			StalePRs: make([]prbottleneck.StalePR, 15), // 15 stale PRs → score 0
		},
		BusFactor: &busfactor.Result{
			BusFactor: 0,
			HHI:       1.0, // maximum concentration
		},
		Reviews: &reviews.Result{
			MedianReviewTime: 10 * 24 * time.Hour, // 10 days, well above 7-day poor threshold
		},
		Releases: &releases.Result{
			DeployFrequencyScore: 0,
		},
		OpenIssues:   200, // well above 100 poor threshold
		MedianPRSize: 2000, // well above 1000 poor threshold
	}
}

func TestCalculate_PerfectScore(t *testing.T) {
	result := Calculate(perfectInput(), defaultWeights(), defaultThresholds())

	if result.TotalScore < 90 || result.TotalScore > 100 {
		t.Errorf("expected TotalScore in [90,100], got %d", result.TotalScore)
	}
	if result.Band != "Excellent" {
		t.Errorf("expected band %q, got %q", "Excellent", result.Band)
	}
	if len(result.Categories) != 7 {
		t.Errorf("expected 7 categories, got %d", len(result.Categories))
	}
}

func TestCalculate_PoorScore(t *testing.T) {
	result := Calculate(poorInput(), defaultWeights(), defaultThresholds())

	if result.TotalScore > 49 {
		t.Errorf("expected TotalScore <= 49, got %d", result.TotalScore)
	}
	if result.Band != "High Risk" {
		t.Errorf("expected band %q, got %q", "High Risk", result.Band)
	}
}

func TestCalculate_MixedScore(t *testing.T) {
	// Good bus factor and deploy frequency, poor review speed and PR hygiene.
	input := &Input{
		PRBottleneck: &prbottleneck.Result{
			StalePRs: make([]prbottleneck.StalePR, 4), // 4 stale PRs → score 60
		},
		BusFactor: &busfactor.Result{
			BusFactor: 5,
			HHI:       0.10,
		},
		Reviews: &reviews.Result{
			MedianReviewTime: 48 * time.Hour, // 2 days — mid-range
		},
		Releases: &releases.Result{
			DeployFrequencyScore: 80,
		},
		OpenIssues:   20,
		MedianPRSize: 300,
	}

	result := Calculate(input, defaultWeights(), defaultThresholds())

	if result.Band != "Healthy" && result.Band != "Needs Attention" {
		t.Errorf("expected band %q or %q, got %q", "Healthy", "Needs Attention", result.Band)
	}
}

func TestCalculate_Recommendations(t *testing.T) {
	// Force all categories to be low-scoring.
	result := Calculate(poorInput(), defaultWeights(), defaultThresholds())

	expected := []string{
		"Reduce review response time - consider adding more reviewers",
		"Increase bus factor - encourage broader code ownership",
		"Address stale pull requests to unblock development",
		"Break large PRs into smaller, reviewable chunks",
		"Triage and close stale issues",
		"Increase deployment frequency for faster feedback",
		"Encourage contributions from more team members",
	}

	if len(result.Recommendations) != len(expected) {
		t.Errorf("expected %d recommendations, got %d: %v", len(expected), len(result.Recommendations), result.Recommendations)
	}

	// Build a set for order-independent lookup.
	recSet := make(map[string]bool, len(result.Recommendations))
	for _, r := range result.Recommendations {
		recSet[r] = true
	}
	for _, e := range expected {
		if !recSet[e] {
			t.Errorf("missing recommendation: %q", e)
		}
	}
}

func TestCalculate_BandBoundaries(t *testing.T) {
	thresholds := defaultThresholds()

	tests := []struct {
		score    int
		wantBand string
	}{
		{49, "High Risk"},
		{50, "Needs Attention"},
		{74, "Needs Attention"},
		{75, "Healthy"},
		{89, "Healthy"},
		{90, "Excellent"},
		{100, "Excellent"},
	}

	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			// Construct an input that drives the total score to the target value
			// by using only PR Hygiene (stale PRs) and Deploy Frequency with equal
			// weights, then verify the band from scoreBand directly.
			band := scoreBand(tc.score)
			if band != tc.wantBand {
				t.Errorf("scoreBand(%d) = %q, want %q", tc.score, band, tc.wantBand)
			}

			// Also verify via Calculate using a crafted input.
			// We use a weight set where all weight is on DeployFrequency so we can
			// set the score precisely.
			w := config.WeightConfig{
				ReviewSpeed:          0,
				BusFactor:            0,
				PRHygiene:            0,
				PRSize:               0,
				IssueHealth:          0,
				DeployFrequency:      1.0,
				ContributorDiversity: 0,
			}
			inp := &Input{
				PRBottleneck: &prbottleneck.Result{},
				BusFactor:    &busfactor.Result{BusFactor: 5, HHI: 0.05},
				Reviews:      &reviews.Result{MedianReviewTime: 1 * time.Hour},
				Releases:     &releases.Result{DeployFrequencyScore: tc.score},
				OpenIssues:   0,
				MedianPRSize: 0,
			}
			r := Calculate(inp, w, thresholds)
			if r.TotalScore != tc.score {
				t.Errorf("Calculate total score = %d, want %d", r.TotalScore, tc.score)
			}
			if r.Band != tc.wantBand {
				t.Errorf("Calculate band = %q, want %q for score %d", r.Band, tc.wantBand, tc.score)
			}
		})
	}
}

func TestLinearDecay(t *testing.T) {
	tests := []struct {
		value         float64
		goodThreshold float64
		poorThreshold float64
		want          int
	}{
		// At or below good threshold → 100.
		{0, 10, 100, 100},
		{10, 10, 100, 100},
		// At or above poor threshold → 0.
		{100, 10, 100, 0},
		{200, 10, 100, 0},
		// Midpoint → 50.
		{55, 10, 100, 50},
		// Quarter way between good and poor → 75.
		{32.5, 10, 100, 75},
		// Three-quarters way → 25.
		{77.5, 10, 100, 25},
	}

	for _, tc := range tests {
		got := linearDecay(tc.value, tc.goodThreshold, tc.poorThreshold)
		if got != tc.want {
			t.Errorf("linearDecay(%.1f, %.1f, %.1f) = %d, want %d",
				tc.value, tc.goodThreshold, tc.poorThreshold, got, tc.want)
		}
	}
}
