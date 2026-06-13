package health

import (
	"fmt"
	"math"
	"time"

	"github.com/jbsommeling/merge-metrics/internal/config"
	"github.com/jbsommeling/merge-metrics/internal/metrics/busfactor"
	"github.com/jbsommeling/merge-metrics/internal/metrics/prbottleneck"
	"github.com/jbsommeling/merge-metrics/internal/metrics/releases"
	"github.com/jbsommeling/merge-metrics/internal/metrics/reviews"
)

// ScoreCategory holds the score and metadata for a single health dimension.
type ScoreCategory struct {
	Name         string  `json:"name"`
	Score        int     `json:"score"`        // 0-100
	Weight       float64 `json:"weight"`
	Contribution float64 `json:"contribution"` // score * weight
	Detail       string  `json:"detail"`       // human explanation
}

// Result is the output of the health score engine.
type Result struct {
	TotalScore      int             `json:"total_score"` // 0-100
	Band            string          `json:"band"`        // "Excellent", "Healthy", "Needs Attention", "High Risk"
	Categories      []ScoreCategory `json:"categories"`
	Recommendations []string        `json:"recommendations"`
}

// Input aggregates the results from each metric engine plus repo-level counters.
type Input struct {
	PRBottleneck *prbottleneck.Result
	BusFactor    *busfactor.Result
	Reviews      *reviews.Result
	Releases     *releases.Result
	OpenIssues   int
	MedianPRSize int // lines changed
}

// Calculate computes an overall repository health score from the provided inputs.
func Calculate(input *Input, weights config.WeightConfig, thresholds config.ThresholdConfig) *Result {
	categories := make([]ScoreCategory, 0, 7)

	// 1. Review Speed
	reviewSpeedScore := reviewSpeedSubScore(input, thresholds)
	categories = append(categories, ScoreCategory{
		Name:         "Review Speed",
		Score:        reviewSpeedScore,
		Weight:       weights.ReviewSpeed,
		Contribution: float64(reviewSpeedScore) * weights.ReviewSpeed,
		Detail:       fmt.Sprintf("Median review time: %s", formatDuration(input.Reviews.MedianReviewTime)),
	})

	// 2. Bus Factor
	bfScore := busFactorSubScore(input)
	categories = append(categories, ScoreCategory{
		Name:         "Bus Factor",
		Score:        bfScore,
		Weight:       weights.BusFactor,
		Contribution: float64(bfScore) * weights.BusFactor,
		Detail:       fmt.Sprintf("Bus factor: %d", input.BusFactor.BusFactor),
	})

	// 3. PR Hygiene
	stalePRCount := len(input.PRBottleneck.StalePRs)
	prHygieneScore := max(0, 100-stalePRCount*10)
	categories = append(categories, ScoreCategory{
		Name:         "PR Hygiene",
		Score:        prHygieneScore,
		Weight:       weights.PRHygiene,
		Contribution: float64(prHygieneScore) * weights.PRHygiene,
		Detail:       fmt.Sprintf("%d stale PRs", stalePRCount),
	})

	// 4. PR Size
	prSizeScore := linearDecay(float64(input.MedianPRSize), float64(thresholds.PRSizeGood), float64(thresholds.PRSizePoor))
	categories = append(categories, ScoreCategory{
		Name:         "PR Size",
		Score:        prSizeScore,
		Weight:       weights.PRSize,
		Contribution: float64(prSizeScore) * weights.PRSize,
		Detail:       fmt.Sprintf("Median PR size: %d lines", input.MedianPRSize),
	})

	// 5. Issue Health
	issueHealthScore := linearDecay(float64(input.OpenIssues), 10, 100)
	categories = append(categories, ScoreCategory{
		Name:         "Issue Health",
		Score:        issueHealthScore,
		Weight:       weights.IssueHealth,
		Contribution: float64(issueHealthScore) * weights.IssueHealth,
		Detail:       fmt.Sprintf("%d open issues", input.OpenIssues),
	})

	// 6. Deploy Frequency
	deployScore := input.Releases.DeployFrequencyScore
	categories = append(categories, ScoreCategory{
		Name:         "Deploy Frequency",
		Score:        deployScore,
		Weight:       weights.DeployFrequency,
		Contribution: float64(deployScore) * weights.DeployFrequency,
		Detail:       fmt.Sprintf("Deploy frequency score: %d", deployScore),
	})

	// 7. Contributor Diversity
	diversityScore := linearDecay(input.BusFactor.HHI, 0.15, 1.0)
	categories = append(categories, ScoreCategory{
		Name:         "Contributor Diversity",
		Score:        diversityScore,
		Weight:       weights.ContributorDiversity,
		Contribution: float64(diversityScore) * weights.ContributorDiversity,
		Detail:       fmt.Sprintf("HHI: %.2f", input.BusFactor.HHI),
	})

	// Compute total score.
	var total float64
	for _, c := range categories {
		total += c.Contribution
	}
	totalScore := int(math.Round(total))

	// Clamp to 0-100.
	if totalScore < 0 {
		totalScore = 0
	} else if totalScore > 100 {
		totalScore = 100
	}

	band := scoreBand(totalScore)

	// Build recommendations for low-scoring categories.
	recommendations := buildRecommendations(categories)

	return &Result{
		TotalScore:      totalScore,
		Band:            band,
		Categories:      categories,
		Recommendations: recommendations,
	}
}

// linearDecay returns 100 when value <= goodThreshold, 0 when value >= poorThreshold,
// and linearly interpolates between the two.
func linearDecay(value, goodThreshold, poorThreshold float64) int {
	if value <= goodThreshold {
		return 100
	}
	if value >= poorThreshold {
		return 0
	}
	ratio := (value - goodThreshold) / (poorThreshold - goodThreshold)
	return int(math.Round(100 * (1 - ratio)))
}

// reviewSpeedSubScore computes the 0-100 review speed score.
func reviewSpeedSubScore(input *Input, thresholds config.ThresholdConfig) int {
	medianHours := input.Reviews.MedianReviewTime.Hours()
	excellentHours := float64(thresholds.ReviewTimeExcellentHours)
	poorHours := float64(thresholds.ReviewTimePoorDays) * 24
	return linearDecay(medianHours, excellentHours, poorHours)
}

// busFactorSubScore computes the 0-100 bus factor score using the step table.
func busFactorSubScore(input *Input) int {
	switch {
	case input.BusFactor.BusFactor >= 5:
		return 100
	case input.BusFactor.BusFactor >= 3:
		return 75
	case input.BusFactor.BusFactor == 2:
		return 50
	case input.BusFactor.BusFactor == 1:
		return 20
	default:
		return 0
	}
}

// scoreBand maps a total score to its qualitative band label.
func scoreBand(score int) string {
	switch {
	case score >= 90:
		return "Excellent"
	case score >= 75:
		return "Healthy"
	case score >= 50:
		return "Needs Attention"
	default:
		return "High Risk"
	}
}

// buildRecommendations returns a slice of recommendation strings for all
// categories whose score is below 60.
func buildRecommendations(categories []ScoreCategory) []string {
	var recs []string
	for _, c := range categories {
		if c.Score >= 60 {
			continue
		}
		switch c.Name {
		case "Review Speed":
			recs = append(recs, "Reduce review response time - consider adding more reviewers")
		case "Bus Factor":
			recs = append(recs, "Increase bus factor - encourage broader code ownership")
		case "PR Hygiene":
			recs = append(recs, "Address stale pull requests to unblock development")
		case "PR Size":
			recs = append(recs, "Break large PRs into smaller, reviewable chunks")
		case "Issue Health":
			recs = append(recs, "Triage and close stale issues")
		case "Deploy Frequency":
			recs = append(recs, "Increase deployment frequency for faster feedback")
		case "Contributor Diversity":
			recs = append(recs, "Encourage contributions from more team members")
		}
	}
	return recs
}

// formatDuration returns a human-readable summary of a duration.
func formatDuration(d time.Duration) string {
	hours := d.Hours()
	if hours < 1 {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if hours < 24 {
		return fmt.Sprintf("%.1f hours", hours)
	}
	return fmt.Sprintf("%.1f days", hours/24)
}

// max returns the larger of a and b (integer).
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
