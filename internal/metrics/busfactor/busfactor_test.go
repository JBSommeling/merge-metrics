package busfactor

import (
	"testing"

	"github.com/jbsommeling/merge-metrics/internal/github"
)

func makeCommits(authorCounts map[string]int) []github.Commit {
	var commits []github.Commit
	for author, n := range authorCounts {
		for i := 0; i < n; i++ {
			commits = append(commits, github.Commit{Author: author})
		}
	}
	return commits
}

func TestAnalyze_NoCommits(t *testing.T) {
	result := Analyze([]github.Commit{}, 30)
	if result.BusFactor != 0 {
		t.Errorf("expected BusFactor 0, got %d", result.BusFactor)
	}
	if result.TotalCommits != 0 {
		t.Errorf("expected TotalCommits 0, got %d", result.TotalCommits)
	}
	if result.HHI != 0 {
		t.Errorf("expected HHI 0, got %f", result.HHI)
	}
	if len(result.Contributors) != 0 {
		t.Errorf("expected no contributors, got %d", len(result.Contributors))
	}
}

func TestAnalyze_SingleContributor(t *testing.T) {
	commits := makeCommits(map[string]int{"alice": 10})
	result := Analyze(commits, 30)
	if result.BusFactor != 1 {
		t.Errorf("expected BusFactor 1, got %d", result.BusFactor)
	}
	if result.HHI != 1.0 {
		t.Errorf("expected HHI 1.0, got %f", result.HHI)
	}
	if result.TotalCommits != 10 {
		t.Errorf("expected TotalCommits 10, got %d", result.TotalCommits)
	}
}

func TestAnalyze_EvenSplit(t *testing.T) {
	// 4 contributors with 25 commits each → 25% each
	// cumulative to reach 50%: 25% + 25% = 50% → bus factor 2
	// HHI = 4 * (0.25)^2 = 4 * 0.0625 = 0.25
	commits := makeCommits(map[string]int{
		"alice": 25,
		"bob":   25,
		"carol": 25,
		"dave":  25,
	})
	result := Analyze(commits, 30)
	if result.BusFactor != 2 {
		t.Errorf("expected BusFactor 2, got %d", result.BusFactor)
	}
	if result.TotalCommits != 100 {
		t.Errorf("expected TotalCommits 100, got %d", result.TotalCommits)
	}
	const wantHHI = 0.25
	if result.HHI < wantHHI-0.0001 || result.HHI > wantHHI+0.0001 {
		t.Errorf("expected HHI ~0.25, got %f", result.HHI)
	}
}

func TestAnalyze_SkewedDistribution(t *testing.T) {
	// alice: 80 commits (80%), bob: 10 (10%), carol: 10 (10%)
	// alice alone covers 80% >= 50% → bus factor 1
	commits := makeCommits(map[string]int{
		"alice": 80,
		"bob":   10,
		"carol": 10,
	})
	result := Analyze(commits, 30)
	if result.BusFactor != 1 {
		t.Errorf("expected BusFactor 1, got %d", result.BusFactor)
	}
	if result.TotalCommits != 100 {
		t.Errorf("expected TotalCommits 100, got %d", result.TotalCommits)
	}
}

func TestAnalyze_HealthyDistribution(t *testing.T) {
	// 5 contributors with 20% each
	// cumulative: 20%, 40%, 60% → need 3 to reach >= 50% (crosses at 3rd)
	// HHI = 5 * (0.2)^2 = 5 * 0.04 = 0.2
	commits := makeCommits(map[string]int{
		"alice": 20,
		"bob":   20,
		"carol": 20,
		"dave":  20,
		"eve":   20,
	})
	result := Analyze(commits, 30)
	if result.BusFactor != 3 {
		t.Errorf("expected BusFactor 3, got %d", result.BusFactor)
	}
	if result.TotalCommits != 100 {
		t.Errorf("expected TotalCommits 100, got %d", result.TotalCommits)
	}
	const wantHHI = 0.2
	if result.HHI < wantHHI-0.0001 || result.HHI > wantHHI+0.0001 {
		t.Errorf("expected HHI ~0.2, got %f", result.HHI)
	}
}
