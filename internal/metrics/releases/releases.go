package releases

import (
	"sort"
	"time"

	"github.com/jbsommeling/merge-metrics/internal/github"
)

// Result holds computed release metrics.
type Result struct {
	ReleasesLastMonth      int
	ReleasesLastWeek       int
	AvgTimeBetweenReleases time.Duration
	Trend                  string // "increasing", "stable", "decreasing"
	DeployFrequencyScore   int    // 0-100
	TotalReleases          int
}

// Analyze computes release metrics from the given releases using the current time.
func Analyze(releases []github.Release) *Result {
	return analyzeAt(releases, time.Now())
}

func analyzeAt(releases []github.Release, now time.Time) *Result {
	// Filter out pre-releases.
	filtered := make([]github.Release, 0, len(releases))
	for _, r := range releases {
		if !r.Prerelease {
			filtered = append(filtered, r)
		}
	}

	// Sort by PublishedAt descending (most recent first).
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].PublishedAt.After(filtered[j].PublishedAt)
	})

	result := &Result{
		TotalReleases: len(filtered),
	}

	if len(filtered) == 0 {
		result.Trend = "stable"
		result.DeployFrequencyScore = 0
		return result
	}

	// Window boundaries.
	last30Start := now.AddDate(0, 0, -30)
	last7Start := now.AddDate(0, 0, -7)
	prior30Start := now.AddDate(0, 0, -60)

	var countLast30, countLast7, countPrior30 int
	for _, r := range filtered {
		pub := r.PublishedAt
		if pub.After(last30Start) {
			countLast30++
		}
		if pub.After(last7Start) {
			countLast7++
		}
		if pub.After(prior30Start) && !pub.After(last30Start) {
			countPrior30++
		}
	}

	result.ReleasesLastMonth = countLast30
	result.ReleasesLastWeek = countLast7

	// AvgTimeBetweenReleases — mean of deltas between consecutive releases.
	if len(filtered) >= 2 {
		var totalDelta time.Duration
		for i := 0; i < len(filtered)-1; i++ {
			// filtered is descending, so filtered[i] is newer than filtered[i+1].
			delta := filtered[i].PublishedAt.Sub(filtered[i+1].PublishedAt)
			totalDelta += delta
		}
		result.AvgTimeBetweenReleases = totalDelta / time.Duration(len(filtered)-1)
	}

	// Trend.
	switch {
	case countLast30 > countPrior30:
		result.Trend = "increasing"
	case countLast30 < countPrior30:
		result.Trend = "decreasing"
	default:
		result.Trend = "stable"
	}

	// DeployFrequencyScore.
	const day = 24 * time.Hour
	switch {
	case len(filtered) < 2:
		// Single release: no interval to measure; treat as low frequency.
		result.DeployFrequencyScore = 20
	case result.AvgTimeBetweenReleases <= day:
		result.DeployFrequencyScore = 100
	case result.AvgTimeBetweenReleases <= 7*day:
		result.DeployFrequencyScore = 80
	case result.AvgTimeBetweenReleases <= 30*day:
		result.DeployFrequencyScore = 50
	default:
		result.DeployFrequencyScore = 20
	}

	return result
}
