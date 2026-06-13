package reviews

import (
	"math"
	"sort"
	"time"

	"github.com/jbsommeling/merge-metrics/internal/github"
)

// ReviewerStat holds aggregated review statistics for a single reviewer.
type ReviewerStat struct {
	Login           string
	AvgResponseTime time.Duration
	ReviewCount     int
	ReviewsPerWeek  float64
}

// Result holds the aggregated review analytics for a set of pull requests.
type Result struct {
	AverageReviewTime time.Duration
	MedianReviewTime  time.Duration
	P90ReviewTime     time.Duration
	ReviewerStats     []ReviewerStat // sorted by AvgResponseTime ascending (fastest first)
	TotalReviews      int
	ReviewThroughput  float64 // total reviews per week
}

// Analyze computes review analytics from a slice of pull requests and their reviews.
// Only closed PRs are considered. Self-reviews (where review.User == pr.User) are skipped.
// Only the first non-author review per PR contributes to review time calculations.
func Analyze(prs []github.PullRequest, reviewsMap map[int][]github.Review, analysisPeriodDays int) *Result {
	weeksInPeriod := float64(analysisPeriodDays) / 7.0

	// reviewerTotals accumulates data per reviewer across all PRs.
	type reviewerData struct {
		totalResponseTime time.Duration
		count             int
	}
	reviewerMap := make(map[string]*reviewerData)

	var reviewTimes []time.Duration

	for _, pr := range prs {
		if pr.State != "closed" {
			continue
		}

		revs := reviewsMap[pr.Number]

		// Sort reviews chronologically so we pick the earliest non-author review.
		sort.Slice(revs, func(i, j int) bool {
			return revs[i].SubmittedAt.Before(revs[j].SubmittedAt)
		})

		// Find the first non-author review for this PR.
		for _, rev := range revs {
			if rev.User == pr.User {
				continue
			}
			// This is the first non-author review.
			responseTime := rev.SubmittedAt.Sub(pr.CreatedAt)
			reviewTimes = append(reviewTimes, responseTime)

			if _, ok := reviewerMap[rev.User]; !ok {
				reviewerMap[rev.User] = &reviewerData{}
			}
			reviewerMap[rev.User].totalResponseTime += responseTime
			reviewerMap[rev.User].count++

			break
		}
	}

	result := &Result{}

	totalReviews := len(reviewTimes)
	result.TotalReviews = totalReviews

	if weeksInPeriod > 0 {
		result.ReviewThroughput = float64(totalReviews) / weeksInPeriod
	}

	if totalReviews > 0 {
		// Compute mean.
		var sum time.Duration
		for _, d := range reviewTimes {
			sum += d
		}
		result.AverageReviewTime = sum / time.Duration(totalReviews)

		// Compute median and P90 (helpers require sorted input).
		sorted := make([]time.Duration, len(reviewTimes))
		copy(sorted, reviewTimes)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

		result.MedianReviewTime = median(sorted)
		result.P90ReviewTime = percentile(sorted, 90)
	}

	// Build ReviewerStats.
	stats := make([]ReviewerStat, 0, len(reviewerMap))
	for login, data := range reviewerMap {
		var avg time.Duration
		if data.count > 0 {
			avg = data.totalResponseTime / time.Duration(data.count)
		}
		var rpw float64
		if weeksInPeriod > 0 {
			rpw = float64(data.count) / weeksInPeriod
		}
		stats = append(stats, ReviewerStat{
			Login:           login,
			AvgResponseTime: avg,
			ReviewCount:     data.count,
			ReviewsPerWeek:  rpw,
		})
	}

	// Sort by AvgResponseTime ascending (fastest first); break ties by login for stability.
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].AvgResponseTime != stats[j].AvgResponseTime {
			return stats[i].AvgResponseTime < stats[j].AvgResponseTime
		}
		return stats[i].Login < stats[j].Login
	})

	result.ReviewerStats = stats
	return result
}

// median returns the median value of a pre-sorted slice of durations.
// Returns 0 if the slice is empty.
func median(durations []time.Duration) time.Duration {
	n := len(durations)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return durations[n/2]
	}
	return (durations[n/2-1] + durations[n/2]) / 2
}

// percentile returns the p-th percentile of a pre-sorted slice of durations
// using the nearest-rank method. p should be in the range (0, 100].
// Returns 0 if the slice is empty.
func percentile(durations []time.Duration, p float64) time.Duration {
	n := len(durations)
	if n == 0 {
		return 0
	}
	// Nearest-rank: ceil(p/100 * n), 1-indexed.
	rank := int(math.Ceil(p / 100.0 * float64(n)))
	if rank < 1 {
		rank = 1
	}
	if rank > n {
		rank = n
	}
	return durations[rank-1]
}
