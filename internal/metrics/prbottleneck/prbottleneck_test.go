package prbottleneck

import (
	"testing"
	"time"

	"github.com/jbsommeling/merge-metrics/internal/github"
)

var now = time.Now()

func makePR(number int, daysAgo float64) github.PullRequest {
	created := now.Add(-time.Duration(daysAgo*24) * time.Hour)
	return github.PullRequest{
		Number:    number,
		Title:     "PR title",
		User:      "author",
		CreatedAt: created,
		UpdatedAt: created, // same as created by default (no recent activity)
	}
}

func makeReview(user, state string, daysAgo float64) github.Review {
	return github.Review{
		User:        user,
		State:       state,
		SubmittedAt: now.Add(-time.Duration(daysAgo*24) * time.Hour),
	}
}

// TestAnalyze_NoPRs verifies that empty input returns a zero Result.
func TestAnalyze_NoPRs(t *testing.T) {
	result := Analyze(nil, nil, nil, 7)
	if result.OpenPRCount != 0 {
		t.Errorf("expected OpenPRCount=0, got %d", result.OpenPRCount)
	}
	if result.AveragePRAgeDays != 0 {
		t.Errorf("expected AveragePRAgeDays=0, got %f", result.AveragePRAgeDays)
	}
	if len(result.StalePRs) != 0 {
		t.Errorf("expected no StalePRs, got %v", result.StalePRs)
	}
	if result.WaitingForReview != 0 {
		t.Errorf("expected WaitingForReview=0, got %d", result.WaitingForReview)
	}
	if result.WaitingForAuthor != 0 {
		t.Errorf("expected WaitingForAuthor=0, got %d", result.WaitingForAuthor)
	}
	if len(result.ReviewerWorkload) != 0 {
		t.Errorf("expected no ReviewerWorkload, got %v", result.ReviewerWorkload)
	}
	if len(result.Bottlenecks) != 0 {
		t.Errorf("expected no Bottlenecks, got %v", result.Bottlenecks)
	}
}

// TestAnalyze_StalePRDetection verifies that a PR open 10 days with no recent activity
// is flagged as stale when the threshold is 7 days.
func TestAnalyze_StalePRDetection(t *testing.T) {
	pr := makePR(1, 10) // open 10 days ago, UpdatedAt = CreatedAt (no recent activity)
	prs := []github.PullRequest{pr}
	reviewsMap := map[int][]github.Review{}
	requestedReviewersMap := map[int][]github.ReviewRequest{}

	result := Analyze(prs, reviewsMap, requestedReviewersMap, 7)

	if len(result.StalePRs) != 1 {
		t.Fatalf("expected 1 stale PR, got %d", len(result.StalePRs))
	}
	stale := result.StalePRs[0]
	if stale.Number != 1 {
		t.Errorf("expected stale PR number 1, got %d", stale.Number)
	}
	if stale.OpenDays < 10 {
		t.Errorf("expected OpenDays >= 10, got %d", stale.OpenDays)
	}
}

// TestAnalyze_StalePRDetection_RecentActivityExcludes verifies that a PR with a recent
// review is NOT flagged as stale even if it is old.
func TestAnalyze_StalePRDetection_RecentActivityExcludes(t *testing.T) {
	pr := makePR(1, 10)
	prs := []github.PullRequest{pr}
	// Review submitted 1 day ago = recent activity.
	reviewsMap := map[int][]github.Review{
		1: {makeReview("reviewer", "COMMENTED", 1)},
	}
	requestedReviewersMap := map[int][]github.ReviewRequest{}

	result := Analyze(prs, reviewsMap, requestedReviewersMap, 7)

	if len(result.StalePRs) != 0 {
		t.Errorf("expected no stale PRs when recent review exists, got %v", result.StalePRs)
	}
}

// TestAnalyze_WaitingForReview verifies that a PR with requested reviewers but no
// reviews is counted as waiting_for_review.
func TestAnalyze_WaitingForReview(t *testing.T) {
	pr := makePR(1, 2)
	prs := []github.PullRequest{pr}
	reviewsMap := map[int][]github.Review{}
	requestedReviewersMap := map[int][]github.ReviewRequest{
		1: {{Login: "alice"}},
	}

	result := Analyze(prs, reviewsMap, requestedReviewersMap, 7)

	if result.WaitingForReview != 1 {
		t.Errorf("expected WaitingForReview=1, got %d", result.WaitingForReview)
	}
	if result.WaitingForAuthor != 0 {
		t.Errorf("expected WaitingForAuthor=0, got %d", result.WaitingForAuthor)
	}
}

// TestAnalyze_WaitingForAuthor verifies that a PR whose latest review is
// CHANGES_REQUESTED is counted as waiting_for_author.
func TestAnalyze_WaitingForAuthor(t *testing.T) {
	pr := makePR(1, 5)
	prs := []github.PullRequest{pr}
	reviewsMap := map[int][]github.Review{
		1: {makeReview("alice", "CHANGES_REQUESTED", 2)},
	}
	requestedReviewersMap := map[int][]github.ReviewRequest{}

	result := Analyze(prs, reviewsMap, requestedReviewersMap, 7)

	if result.WaitingForAuthor != 1 {
		t.Errorf("expected WaitingForAuthor=1, got %d", result.WaitingForAuthor)
	}
	if result.WaitingForReview != 0 {
		t.Errorf("expected WaitingForReview=0, got %d", result.WaitingForReview)
	}
}

// TestAnalyze_ReviewerBottleneck verifies that a reviewer with 5 pending reviews
// is flagged as a bottleneck when other reviewers have only 1 pending each
// (median=1, 5 > 2*1).
func TestAnalyze_ReviewerBottleneck(t *testing.T) {
	// alice has 5 pending PRs; bob, carol, dave each have 1.
	prs := []github.PullRequest{
		makePR(1, 2),
		makePR(2, 2),
		makePR(3, 2),
		makePR(4, 2),
		makePR(5, 2),
		makePR(6, 2),
		makePR(7, 2),
	}
	reviewsMap := map[int][]github.Review{}
	requestedReviewersMap := map[int][]github.ReviewRequest{
		1: {{Login: "alice"}},
		2: {{Login: "alice"}},
		3: {{Login: "alice"}},
		4: {{Login: "alice"}},
		5: {{Login: "alice"}},
		6: {{Login: "bob"}},
		7: {{Login: "carol"}},
	}

	result := Analyze(prs, reviewsMap, requestedReviewersMap, 30)

	// Find alice's load.
	var aliceLoad *ReviewerLoad
	for i := range result.ReviewerWorkload {
		if result.ReviewerWorkload[i].Login == "alice" {
			aliceLoad = &result.ReviewerWorkload[i]
		}
	}
	if aliceLoad == nil {
		t.Fatal("alice not found in ReviewerWorkload")
	}
	if aliceLoad.PendingCount != 5 {
		t.Errorf("expected alice PendingCount=5, got %d", aliceLoad.PendingCount)
	}
	if !aliceLoad.IsBottleneck {
		t.Errorf("expected alice to be flagged as bottleneck")
	}
	if len(result.Bottlenecks) == 0 {
		t.Error("expected at least one bottleneck description")
	}

	// bob and carol should not be bottlenecks.
	for _, load := range result.ReviewerWorkload {
		if load.Login == "bob" || load.Login == "carol" {
			if load.IsBottleneck {
				t.Errorf("expected %s not to be a bottleneck", load.Login)
			}
		}
	}
}
