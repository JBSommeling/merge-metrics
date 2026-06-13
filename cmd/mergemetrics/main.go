package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jbsommeling/merge-metrics/internal/config"
	githubclient "github.com/jbsommeling/merge-metrics/internal/github"
	"github.com/jbsommeling/merge-metrics/internal/dashboard"
	"github.com/jbsommeling/merge-metrics/internal/metrics/busfactor"
	"github.com/jbsommeling/merge-metrics/internal/metrics/health"
	"github.com/jbsommeling/merge-metrics/internal/metrics/prbottleneck"
	"github.com/jbsommeling/merge-metrics/internal/metrics/releases"
	"github.com/jbsommeling/merge-metrics/internal/metrics/reviews"
	"github.com/jbsommeling/merge-metrics/internal/publisher"
	"golang.org/x/sync/errgroup"
)

func main() {
	// Determine subcommand.
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "generate" {
		args = args[1:]
	} else if len(args) > 0 && strings.HasPrefix(args[0], "-") {
		// No subcommand, but flags follow — treat as generate.
	} else if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unknown command %q\n\nUsage: mergemetrics generate [flags]\n", args[0])
		os.Exit(1)
	}

	// 1. Parse flags.
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: mergemetrics generate [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
	}

	var (
		owner     = fs.String("owner", "", "Repository owner (default: auto-detect from git remote)")
		repo      = fs.String("repo", "", "Repository name (default: auto-detect from git remote)")
		token     = fs.String("token", "", "GitHub personal access token (default: $GITHUB_TOKEN env var)")
		cfgPath   = fs.String("config", ".mergemetrics.yml", "Path to config file")
		outputDir = fs.String("output-dir", "docs", "Output directory for dashboard")
		dryRun    = fs.Bool("dry-run", false, "Print metrics without writing files")
	)

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	// 2. Auto-detect owner/repo from git remote if not provided.
	if *owner == "" || *repo == "" {
		detectedOwner, detectedRepo, err := detectRepo()
		if err != nil {
			log.Fatalf("auto-detect owner/repo: %v\nUse --owner and --repo to specify them explicitly", err)
		}
		if *owner == "" {
			*owner = detectedOwner
		}
		if *repo == "" {
			*repo = detectedRepo
		}
	}

	if *owner == "" || *repo == "" {
		log.Fatal("owner and repo are required; use --owner and --repo or run from a git repository with a GitHub remote")
	}

	// 3. Get token from flag or GITHUB_TOKEN env var.
	if *token == "" {
		*token = os.Getenv("GITHUB_TOKEN")
	}
	if *token == "" {
		log.Fatal("GitHub token is required; use --token or set the GITHUB_TOKEN environment variable")
	}

	// 4. Load config.
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	// 5. Create GitHub client.
	client := githubclient.NewClient(*token)
	ctx := context.Background()

	// 6. Fetch all data (with progress messages to stderr).
	fmt.Fprintln(os.Stderr, "Fetching repository info...")
	repoInfo, err := client.GetRepository(ctx, *owner, *repo)
	if err != nil {
		log.Fatalf("fetch repository: %v", err)
	}

	fmt.Fprintln(os.Stderr, "Fetching pull requests...")
	openPRs, err := client.ListOpenPullRequests(ctx, *owner, *repo)
	if err != nil {
		log.Fatalf("fetch open pull requests: %v", err)
	}

	prLookbackSince := time.Now().AddDate(0, 0, -cfg.PRLookbackDays)
	closedPRs, err := client.ListClosedPullRequests(ctx, *owner, *repo, prLookbackSince)
	if err != nil {
		log.Fatalf("fetch closed pull requests: %v", err)
	}

	// Fetch reviews and requested reviewers for open PRs.
	reviewsMap := make(map[int][]githubclient.Review)
	requestedReviewersMap := make(map[int][]githubclient.ReviewRequest)
	allPRs := make([]githubclient.PullRequest, 0, len(openPRs)+len(closedPRs))
	allPRs = append(allPRs, openPRs...)
	allPRs = append(allPRs, closedPRs...)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(10)
	var mu sync.Mutex

	for _, pr := range allPRs {
		pr := pr
		g.Go(func() error {
			revs, err := client.ListPullRequestReviews(gctx, *owner, *repo, pr.Number)
			if err != nil {
				return fmt.Errorf("fetching reviews for PR #%d: %w", pr.Number, err)
			}
			mu.Lock()
			reviewsMap[pr.Number] = revs
			mu.Unlock()
			return nil
		})
	}

	for _, pr := range openPRs {
		pr := pr
		g.Go(func() error {
			rr, err := client.ListRequestedReviewers(gctx, *owner, *repo, pr.Number)
			if err != nil {
				return fmt.Errorf("fetching reviewers for PR #%d: %w", pr.Number, err)
			}
			mu.Lock()
			requestedReviewersMap[pr.Number] = rr
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		log.Fatalf("Error fetching PR details: %v", err)
	}

	commitLookbackSince := time.Now().AddDate(0, 0, -cfg.CommitLookbackDays)
	fmt.Fprintf(os.Stderr, "Fetching commits (last %d days)...\n", cfg.CommitLookbackDays)
	commits, err := client.ListCommits(ctx, *owner, *repo, commitLookbackSince)
	if err != nil {
		log.Fatalf("fetch commits: %v", err)
	}

	fmt.Fprintln(os.Stderr, "Fetching releases...")
	releaseList, err := client.ListReleases(ctx, *owner, *repo)
	if err != nil {
		log.Fatalf("fetch releases: %v", err)
	}

	// 7. Run all metric engines.
	fmt.Fprintln(os.Stderr, "Analyzing metrics...")

	prBottleneckResult := prbottleneck.Analyze(openPRs, reviewsMap, requestedReviewersMap, cfg.Thresholds.StalePRDays)
	busFactorResult := busfactor.Analyze(commits, cfg.CommitLookbackDays)
	reviewsResult := reviews.Analyze(allPRs, reviewsMap, cfg.PRLookbackDays)
	releasesResult := releases.Analyze(releaseList)

	// Compute MedianPRSize from open PRs.
	medianPRSize := computeMedianPRSize(openPRs)

	// 8. Calculate health score.
	healthInput := &health.Input{
		PRBottleneck: prBottleneckResult,
		BusFactor:    busFactorResult,
		Reviews:      reviewsResult,
		Releases:     releasesResult,
		OpenIssues:   repoInfo.OpenIssuesCount,
		MedianPRSize: medianPRSize,
	}
	healthResult := health.Calculate(healthInput, cfg.Weights, cfg.Thresholds)

	// 9. Generate dashboard (unless dry-run).
	if !*dryRun {
		fmt.Fprintln(os.Stderr, "Generating dashboard...")
		dashData := &dashboard.DashboardData{
			RepoName:     repoInfo.Name,
			RepoFullName: repoInfo.FullName,
			GeneratedAt:  time.Now().UTC(),
			HealthScore:  healthResult,
			PRBottleneck: prBottleneckResult,
			BusFactor:    busFactorResult,
			Reviews:      reviewsResult,
			Releases:     releasesResult,
		}
		if err := dashboard.Generate(dashData, *outputDir); err != nil {
			log.Fatalf("generate dashboard: %v", err)
		}

		// 10. Update README (unless dry-run or disabled).
		if cfg.UpdateReadme {
			fmt.Fprintln(os.Stderr, "Updating README...")
			readmeUpdate := &publisher.ReadmeUpdate{
				HealthScore: healthResult.TotalScore,
				Band:        healthResult.Band,
				PagesURL:    cfg.PagesURL,
				UpdatedAt:   time.Now().UTC(),
			}
			if err := publisher.UpdateReadmeFile("README.md", readmeUpdate); err != nil {
				log.Fatalf("update README: %v", err)
			}
		}
	}

	// 11. Print summary to stdout.
	printSummary(*owner, *repo, healthResult, prBottleneckResult, busFactorResult, reviewsResult, releasesResult, *outputDir, *dryRun)
}

// detectRepo runs git remote get-url origin and parses the owner and repo name.
// It supports both HTTPS and SSH remote URL formats.
func detectRepo() (owner, repo string, err error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", "", fmt.Errorf("git remote get-url origin: %w", err)
	}

	rawURL := strings.TrimSpace(string(out))

	// HTTPS: https://github.com/owner/repo.git
	httpsRe := regexp.MustCompile(`https://github\.com/([^/]+)/([^/]+?)(?:\.git)?$`)
	if m := httpsRe.FindStringSubmatch(rawURL); m != nil {
		return m[1], m[2], nil
	}

	// SSH: git@github.com:owner/repo.git
	sshRe := regexp.MustCompile(`git@github\.com:([^/]+)/([^/]+?)(?:\.git)?$`)
	if m := sshRe.FindStringSubmatch(rawURL); m != nil {
		return m[1], m[2], nil
	}

	return "", "", fmt.Errorf("unrecognised remote URL format: %q (expected https://github.com/owner/repo or git@github.com:owner/repo)", rawURL)
}

// computeMedianPRSize calculates the median of (Additions + Deletions) for the
// provided pull requests. Returns 0 if the slice is empty.
func computeMedianPRSize(prs []githubclient.PullRequest) int {
	if len(prs) == 0 {
		return 0
	}
	sizes := make([]int, len(prs))
	for i, pr := range prs {
		sizes[i] = pr.Additions + pr.Deletions
	}
	sort.Ints(sizes)
	n := len(sizes)
	if n%2 == 1 {
		return sizes[n/2]
	}
	return (sizes[n/2-1] + sizes[n/2]) / 2
}

// printSummary writes the formatted summary report to stdout.
func printSummary(
	owner, repo string,
	healthResult *health.Result,
	prResult *prbottleneck.Result,
	bfResult *busfactor.Result,
	revResult *reviews.Result,
	relResult *releases.Result,
	outputDir string,
	dryRun bool,
) {
	title := fmt.Sprintf("MergeMetrics Report for %s/%s", owner, repo)
	fmt.Println(title)
	fmt.Println(strings.Repeat("=", len(title)))
	fmt.Printf("Health Score: %d/100 (%s)\n", healthResult.TotalScore, healthResult.Band)
	fmt.Println()

	fmt.Println("PR Bottlenecks:")
	fmt.Printf("  Open PRs: %d\n", prResult.OpenPRCount)
	fmt.Printf("  Stale PRs: %d\n", len(prResult.StalePRs))
	fmt.Printf("  Avg PR Age: %.1f days\n", prResult.AveragePRAgeDays)
	fmt.Println()

	fmt.Printf("Bus Factor: %d (HHI: %.2f)\n", bfResult.BusFactor, bfResult.HHI)
	fmt.Println()

	fmt.Println("Review Analytics:")
	fmt.Printf("  Avg Review Time: %s\n", formatDuration(revResult.AverageReviewTime))
	fmt.Printf("  Median Review Time: %s\n", formatDuration(revResult.MedianReviewTime))
	fmt.Printf("  P90 Review Time: %s\n", formatDuration(revResult.P90ReviewTime))
	fmt.Println()

	fmt.Println("Releases:")
	fmt.Printf("  Last Month: %d\n", relResult.ReleasesLastMonth)
	fmt.Printf("  Deploy Score: %d/100\n", relResult.DeployFrequencyScore)
	fmt.Println()

	if !dryRun {
		fmt.Printf("Dashboard written to %s/\n", outputDir)
	}
}

// formatDuration returns a compact human-readable duration string.
func formatDuration(d time.Duration) string {
	h := d.Hours()
	if h < 1 {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if h < 24 {
		return fmt.Sprintf("%dh", int(h))
	}
	return fmt.Sprintf("%dd", int(h/24))
}
