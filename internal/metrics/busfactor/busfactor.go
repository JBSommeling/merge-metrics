package busfactor

import (
	"sort"

	"github.com/jbsommeling/merge-metrics/internal/github"
)

// ContributorShare holds a contributor's commit count and percentage share.
type ContributorShare struct {
	Login    string
	Commits  int
	SharePct float64 // 0-100
}

// Result holds the bus factor analysis output.
type Result struct {
	Contributors       []ContributorShare // sorted descending by share
	BusFactor          int
	HHI                float64 // Herfindahl-Hirschman Index, 0-1
	TotalCommits       int
	AnalysisPeriodDays int
}

// Analyze computes bus factor metrics from a slice of commits.
// It is a pure function — no API calls are made.
func Analyze(commits []github.Commit, analysisPeriodDays int) *Result {
	if len(commits) == 0 {
		return &Result{
			Contributors:       []ContributorShare{},
			BusFactor:          0,
			HHI:                0,
			TotalCommits:       0,
			AnalysisPeriodDays: analysisPeriodDays,
		}
	}

	// Step 1: group commits by author.
	counts := make(map[string]int)
	for _, c := range commits {
		counts[c.Author]++
	}

	total := len(commits)

	// Step 2: compute shares.
	contributors := make([]ContributorShare, 0, len(counts))
	for login, n := range counts {
		contributors = append(contributors, ContributorShare{
			Login:    login,
			Commits:  n,
			SharePct: float64(n) / float64(total) * 100,
		})
	}

	// Step 3: sort descending by share.
	sort.Slice(contributors, func(i, j int) bool {
		if contributors[i].SharePct != contributors[j].SharePct {
			return contributors[i].SharePct > contributors[j].SharePct
		}
		return contributors[i].Login < contributors[j].Login
	})

	// Step 4: bus factor = minimum authors whose cumulative share >= 50%.
	busFactor := 0
	cumulative := 0.0
	for _, c := range contributors {
		cumulative += c.SharePct
		busFactor++
		if cumulative >= 50.0 {
			break
		}
	}

	// Step 5: HHI = sum of (share/100)^2.
	hhi := 0.0
	for _, c := range contributors {
		frac := c.SharePct / 100.0
		hhi += frac * frac
	}

	return &Result{
		Contributors:       contributors,
		BusFactor:          busFactor,
		HHI:                hhi,
		TotalCommits:       total,
		AnalysisPeriodDays: analysisPeriodDays,
	}
}
