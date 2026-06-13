package reviews

import (
	"testing"
	"time"

	"github.com/jbsommeling/merge-metrics/internal/github"
)

// base is a fixed reference time used across tests.
var base = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func hrs(h float64) time.Duration {
	return time.Duration(float64(time.Hour) * h)
}

// TestAnalyze_NoPRs verifies that an empty input produces zero-value results.
func TestAnalyze_NoPRs(t *testing.T) {
	result := Analyze(nil, nil, 30)

	if result.TotalReviews != 0 {
		t.Errorf("TotalReviews: got %d, want 0", result.TotalReviews)
	}
	if result.AverageReviewTime != 0 {
		t.Errorf("AverageReviewTime: got %v, want 0", result.AverageReviewTime)
	}
	if result.MedianReviewTime != 0 {
		t.Errorf("MedianReviewTime: got %v, want 0", result.MedianReviewTime)
	}
	if result.P90ReviewTime != 0 {
		t.Errorf("P90ReviewTime: got %v, want 0", result.P90ReviewTime)
	}
	if result.ReviewThroughput != 0 {
		t.Errorf("ReviewThroughput: got %v, want 0", result.ReviewThroughput)
	}
	if len(result.ReviewerStats) != 0 {
		t.Errorf("ReviewerStats: got %d entries, want 0", len(result.ReviewerStats))
	}
}

// TestAnalyze_BasicReviewTimes uses 3 PRs with response times of 2h, 4h, and 6h.
// mean = 4h, median = 4h, P90 = 6h (nearest-rank: ceil(90/100*3) = ceil(2.7) = 3 → index 2 → 6h).
func TestAnalyze_BasicReviewTimes(t *testing.T) {
	prs := []github.PullRequest{
		{Number: 1, State: "closed", User: "alice", CreatedAt: base},
		{Number: 2, State: "closed", User: "alice", CreatedAt: base},
		{Number: 3, State: "closed", User: "alice", CreatedAt: base},
	}
	reviewsMap := map[int][]github.Review{
		1: {{User: "bob", SubmittedAt: base.Add(hrs(2))}},
		2: {{User: "bob", SubmittedAt: base.Add(hrs(4))}},
		3: {{User: "bob", SubmittedAt: base.Add(hrs(6))}},
	}

	result := Analyze(prs, reviewsMap, 28) // 28 days = 4 weeks

	if result.TotalReviews != 3 {
		t.Errorf("TotalReviews: got %d, want 3", result.TotalReviews)
	}

	wantAvg := hrs(4)
	if result.AverageReviewTime != wantAvg {
		t.Errorf("AverageReviewTime: got %v, want %v", result.AverageReviewTime, wantAvg)
	}

	wantMedian := hrs(4)
	if result.MedianReviewTime != wantMedian {
		t.Errorf("MedianReviewTime: got %v, want %v", result.MedianReviewTime, wantMedian)
	}

	wantP90 := hrs(6)
	if result.P90ReviewTime != wantP90 {
		t.Errorf("P90ReviewTime: got %v, want %v", result.P90ReviewTime, wantP90)
	}

	// 3 reviews / 4 weeks = 0.75
	wantThroughput := 0.75
	if result.ReviewThroughput != wantThroughput {
		t.Errorf("ReviewThroughput: got %v, want %v", result.ReviewThroughput, wantThroughput)
	}
}

// TestAnalyze_SkipsSelfReviews verifies that a review where review.User == pr.User
// is not counted; only the subsequent review by a different user is used.
func TestAnalyze_SkipsSelfReviews(t *testing.T) {
	prs := []github.PullRequest{
		{Number: 1, State: "closed", User: "alice", CreatedAt: base},
	}
	reviewsMap := map[int][]github.Review{
		// First review is a self-review; second is by a different user.
		1: {
			{User: "alice", SubmittedAt: base.Add(hrs(1))}, // self-review — skip
			{User: "bob", SubmittedAt: base.Add(hrs(3))},   // first non-author review
		},
	}

	result := Analyze(prs, reviewsMap, 7) // 1 week

	if result.TotalReviews != 1 {
		t.Errorf("TotalReviews: got %d, want 1", result.TotalReviews)
	}

	wantAvg := hrs(3)
	if result.AverageReviewTime != wantAvg {
		t.Errorf("AverageReviewTime: got %v, want %v", result.AverageReviewTime, wantAvg)
	}

	// alice must not appear in reviewer stats.
	for _, rs := range result.ReviewerStats {
		if rs.Login == "alice" {
			t.Errorf("self-reviewer alice should not appear in ReviewerStats")
		}
	}

	if len(result.ReviewerStats) != 1 || result.ReviewerStats[0].Login != "bob" {
		t.Errorf("expected exactly one reviewer (bob), got %+v", result.ReviewerStats)
	}
}

// TestAnalyze_ReviewerRanking verifies that ReviewerStats are sorted by AvgResponseTime
// ascending (fastest reviewer first).
func TestAnalyze_ReviewerRanking(t *testing.T) {
	prs := []github.PullRequest{
		{Number: 1, State: "closed", User: "alice", CreatedAt: base},
		{Number: 2, State: "closed", User: "alice", CreatedAt: base},
		{Number: 3, State: "closed", User: "alice", CreatedAt: base},
	}
	// charlie reviews PR1 in 1h, bob reviews PR2 in 5h, dave reviews PR3 in 3h.
	reviewsMap := map[int][]github.Review{
		1: {{User: "charlie", SubmittedAt: base.Add(hrs(1))}},
		2: {{User: "bob", SubmittedAt: base.Add(hrs(5))}},
		3: {{User: "dave", SubmittedAt: base.Add(hrs(3))}},
	}

	result := Analyze(prs, reviewsMap, 7)

	if len(result.ReviewerStats) != 3 {
		t.Fatalf("expected 3 reviewer stats, got %d", len(result.ReviewerStats))
	}

	// Expected order: charlie (1h), dave (3h), bob (5h).
	want := []string{"charlie", "dave", "bob"}
	for i, stat := range result.ReviewerStats {
		if stat.Login != want[i] {
			t.Errorf("ReviewerStats[%d].Login: got %q, want %q", i, stat.Login, want[i])
		}
	}
}

// TestAnalyze_PRsWithNoReviews verifies that PRs without any reviews do not
// affect the review time calculations.
func TestAnalyze_PRsWithNoReviews(t *testing.T) {
	prs := []github.PullRequest{
		{Number: 1, State: "closed", User: "alice", CreatedAt: base},
		{Number: 2, State: "closed", User: "alice", CreatedAt: base}, // no reviews
	}
	reviewsMap := map[int][]github.Review{
		1: {{User: "bob", SubmittedAt: base.Add(hrs(4))}},
		// PR 2 has no entry in the map.
	}

	result := Analyze(prs, reviewsMap, 7)

	if result.TotalReviews != 1 {
		t.Errorf("TotalReviews: got %d, want 1 (PR without reviews should be ignored)", result.TotalReviews)
	}

	wantAvg := hrs(4)
	if result.AverageReviewTime != wantAvg {
		t.Errorf("AverageReviewTime: got %v, want %v", result.AverageReviewTime, wantAvg)
	}

	if len(result.ReviewerStats) != 1 {
		t.Errorf("ReviewerStats: got %d entries, want 1", len(result.ReviewerStats))
	}
}
