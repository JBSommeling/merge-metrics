package prbottleneck

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/jbsommeling/merge-metrics/internal/github"
)

// StalePR describes a pull request that has been open too long with no recent activity.
type StalePR struct {
	Number   int
	Title    string
	Author   string
	OpenDays int
	Status   string // "waiting_for_review", "waiting_for_author", "idle"
}

// ReviewerLoad describes the pending review load for a single reviewer.
type ReviewerLoad struct {
	Login        string
	PendingCount int
	IsBottleneck bool
}

// Result holds the computed PR bottleneck metrics.
type Result struct {
	OpenPRCount      int
	AveragePRAgeDays float64
	StalePRs         []StalePR
	WaitingForReview int
	WaitingForAuthor int
	ReviewerWorkload []ReviewerLoad
	Bottlenecks      []string // human-readable bottleneck descriptions
}

const recentActivityWindow = 3 * 24 * time.Hour

// Analyze computes PR bottleneck metrics from the supplied data.
// It is a pure function: no API calls are made.
func Analyze(
	prs []github.PullRequest,
	reviewsMap map[int][]github.Review,
	requestedReviewersMap map[int][]github.ReviewRequest,
	staleDaysThreshold int,
) *Result {
	now := time.Now()
	result := &Result{}

	result.OpenPRCount = len(prs)
	if len(prs) == 0 {
		return result
	}

	// Compute average age and collect per-PR data.
	var totalAgeDays float64
	reviewerPending := map[string]int{}

	for _, pr := range prs {
		ageDays := now.Sub(pr.CreatedAt).Hours() / 24
		totalAgeDays += ageDays

		reviews := reviewsMap[pr.Number]
		requestedReviewers := requestedReviewersMap[pr.Number]

		status := prStatus(reviews, requestedReviewers)

		switch status {
		case "waiting_for_review":
			result.WaitingForReview++
		case "waiting_for_author":
			result.WaitingForAuthor++
		}

		// Stale detection: open longer than threshold AND no activity in last 3 days.
		if ageDays > float64(staleDaysThreshold) && !hasRecentActivity(pr, reviews, now) {
			result.StalePRs = append(result.StalePRs, StalePR{
				Number:   pr.Number,
				Title:    pr.Title,
				Author:   pr.User,
				OpenDays: int(math.Floor(ageDays)),
				Status:   status,
			})
		}

		// Count pending review requests per reviewer.
		for _, rr := range requestedReviewers {
			reviewerPending[rr.Login]++
		}
	}

	result.AveragePRAgeDays = totalAgeDays / float64(len(prs))

	// Build ReviewerWorkload slice.
	if len(reviewerPending) > 0 {
		counts := make([]int, 0, len(reviewerPending))
		for _, c := range reviewerPending {
			counts = append(counts, c)
		}
		med := median(counts)

		for login, count := range reviewerPending {
			isBottleneck := med > 0 && count > 2*med
			load := ReviewerLoad{
				Login:        login,
				PendingCount: count,
				IsBottleneck: isBottleneck,
			}
			result.ReviewerWorkload = append(result.ReviewerWorkload, load)
			if isBottleneck {
				result.Bottlenecks = append(result.Bottlenecks,
					fmt.Sprintf("%s has %d pending reviews (median: %d)", login, count, med))
			}
		}

		// Sort for determinism.
		sort.Slice(result.ReviewerWorkload, func(i, j int) bool {
			return result.ReviewerWorkload[i].Login < result.ReviewerWorkload[j].Login
		})
		sort.Strings(result.Bottlenecks)
	}

	return result
}

// prStatus determines the status of a PR based on its reviews and requested reviewers.
func prStatus(reviews []github.Review, requestedReviewers []github.ReviewRequest) string {
	// waiting_for_author takes priority: latest review is CHANGES_REQUESTED.
	if len(reviews) > 0 {
		latest := latestReview(reviews)
		if latest.State == "CHANGES_REQUESTED" {
			return "waiting_for_author"
		}
	}

	// waiting_for_review: has pending review requests (regardless of existing reviews).
	if len(requestedReviewers) > 0 {
		return "waiting_for_review"
	}

	return "idle"
}

// latestReview returns the review with the most recent SubmittedAt time.
func latestReview(reviews []github.Review) github.Review {
	latest := reviews[0]
	for _, r := range reviews[1:] {
		if r.SubmittedAt.After(latest.SubmittedAt) {
			latest = r
		}
	}
	return latest
}

// hasRecentActivity returns true if the PR or any of its reviews had activity
// within the last recentActivityWindow.
func hasRecentActivity(pr github.PullRequest, reviews []github.Review, now time.Time) bool {
	cutoff := now.Add(-recentActivityWindow)
	if pr.UpdatedAt.After(cutoff) {
		return true
	}
	for _, r := range reviews {
		if r.SubmittedAt.After(cutoff) {
			return true
		}
	}
	return false
}

// median returns the median value of a non-empty slice of ints.
func median(values []int) int {
	sorted := make([]int, len(values))
	copy(sorted, values)
	sort.Ints(sorted)
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}
