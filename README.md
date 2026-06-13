# MergeMetrics

Lightweight GitHub repository health dashboard — no SaaS, no accounts, just GitHub.

## What is MergeMetrics?

MergeMetrics is a Go CLI tool and GitHub Action that analyzes your repository's engineering health and generates a static dashboard published via GitHub Pages. It fetches data directly from the GitHub API, runs five metric engines, and produces a self-contained HTML dashboard written into the `docs/` folder of your repository. There is no external service, no account registration, and no data leaves your GitHub environment. Everything lives inside your own repo.

## Features

1. **PR Bottleneck Detection** — identifies stale pull requests, flags reviewers whose pending load is more than 2x the team median, and classifies each open PR as waiting for review, waiting for author, or idle.
2. **Bus Factor Analysis** — measures contribution concentration using commit share data, computes the Herfindahl-Hirschman Index (HHI), and calculates the bus factor as the minimum number of contributors whose combined commits account for at least 50% of activity.
3. **Review Analytics** — computes mean, median, and P90 first-review response times across closed PRs, ranks reviewers by speed and throughput, and reports overall review volume per week.
4. **Release Insights** — measures deployment frequency against DORA benchmarks, tracks release counts over rolling 7-day and 30-day windows, detects trend direction (increasing / stable / decreasing), and scores frequency on a 0-100 scale.
5. **Health Score** — composite 0-100 score across 7 weighted dimensions with qualitative band labels and actionable recommendations for any dimension scoring below 60.

## Quick Start

### As a GitHub Action (Recommended)

Copy this workflow into `.github/workflows/mergemetrics.yml` in your repository:

```yaml
name: MergeMetrics Dashboard
on:
  schedule:
    - cron: '0 6 * * *'
  workflow_dispatch: {}
permissions:
  contents: write
jobs:
  update-dashboard:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: jbsommeling/merge-metrics@main
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
      - uses: stefanzweifel/git-auto-commit-action@v5
        with:
          commit_message: "chore: update MergeMetrics dashboard"
          file_pattern: "docs/* README.md"
```

Then enable GitHub Pages for the `docs/` folder in your repository settings (Settings > Pages > Source: Deploy from a branch, Branch: main, Folder: /docs). Your dashboard will be live at `https://<owner>.github.io/<repo>/`.

The workflow runs daily at 06:00 UTC and can also be triggered manually from the Actions tab.

### As a CLI

```bash
go install github.com/jbsommeling/merge-metrics/cmd/mergemetrics@latest
mergemetrics generate --token $GITHUB_TOKEN
```

Available flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--owner` | auto-detected | Repository owner (GitHub username or org). Auto-detected from `git remote origin` if omitted. |
| `--repo` | auto-detected | Repository name. Auto-detected from `git remote origin` if omitted. |
| `--token` | `$GITHUB_TOKEN` | GitHub personal access token. Falls back to the `GITHUB_TOKEN` environment variable. |
| `--config` | `.mergemetrics.yml` | Path to the configuration file. |
| `--output-dir` | `docs` | Directory to write dashboard files into. |
| `--dry-run` | `false` | Print metrics to stdout without writing any files. |

## How It Works

1. Authenticates with GitHub using a personal access token (PAT) supplied via `--token` or the `GITHUB_TOKEN` environment variable.
2. Fetches repository metadata, open and recently-closed pull requests, per-PR reviews and reviewer assignments, commits within the configured lookback window, and all published releases.
3. Runs all four metric engines (PR bottleneck, bus factor, review analytics, release insights) as independent analyses on the fetched data — no further API calls are made at this stage.
4. Computes the composite health score from the four engine outputs plus open issue count and median PR size.
5. Generates a static HTML dashboard and structured JSON data files.
6. Writes all output to the `docs/` folder and, unless disabled, updates the repository README between sentinel comment markers with the current health badge.
7. In GitHub Action mode, the `stefanzweifel/git-auto-commit-action` step commits the updated files back to the repository automatically.

## Architecture

```
cmd/mergemetrics/       CLI entrypoint and orchestration
internal/
├── auth/               GitHub App JWT and PAT authentication helpers
├── config/             YAML configuration loading with partial-override and validation
├── github/             GitHub REST API client with pagination and rate-limit handling
├── metrics/
│   ├── prbottleneck/   PR bottleneck detection (stale PRs, reviewer load, status classification)
│   ├── busfactor/      Bus factor analysis (commit shares, HHI, cumulative coverage)
│   ├── reviews/        Review time analytics (mean, median, P90, per-reviewer stats)
│   ├── releases/       Release frequency analysis (DORA scoring, trend detection)
│   └── health/         Composite health score engine (7 dimensions, weighted sum)
├── dashboard/          Static HTML, CSS, and JSON file generator
└── publisher/          README badge integration via sentinel comment markers
```

- **cmd/mergemetrics** — parses flags, coordinates data fetching with bounded concurrency, calls all engines, and drives the dashboard and README update steps.
- **internal/config** — loads `.mergemetrics.yml` using pointer-based partial unmarshalling so omitted fields keep their defaults; validates weight sums and threshold signs.
- **internal/github** — wraps the GitHub REST API with automatic pagination and exposes typed structs for PRs, reviews, commits, and releases.
- **internal/metrics/prbottleneck** — pure function analysis of open PR state; classifies stale PRs and identifies reviewer bottlenecks.
- **internal/metrics/busfactor** — aggregates commit counts by author, sorts by share, and cumulates to the 50% threshold to derive the bus factor score.
- **internal/metrics/reviews** — analyses closed PRs to compute first-response times, excluding self-reviews, and ranks reviewers by average response time.
- **internal/metrics/releases** — filters pre-releases, computes rolling window counts and average inter-release interval, and maps to a DORA-aligned score.
- **internal/metrics/health** — applies configurable weights to the per-dimension scores and produces a total score, band label, and recommendations.
- **internal/dashboard** — renders `index.html` via `html/template`, serialises `data.json` and `metrics.json`, and writes the embedded stylesheet.
- **internal/publisher** — locates sentinel comment markers in the README and replaces only the content between them, leaving all other README content untouched.

## Key Components

### Health Score

The health score is a weighted sum of seven dimension scores, each 0-100, rounded to the nearest integer and clamped to [0, 100].

| Category | Weight | Scoring Logic |
|----------|--------|---------------|
| Review Speed | 20% | 100 if median review time < 4 hours; linear decay to 0 at 7 days |
| Bus Factor | 20% | 100 if bus factor >= 5; 75 if >= 3; 50 if = 2; 20 if = 1; 0 otherwise |
| PR Hygiene | 15% | 100 minus 10 per stale PR (minimum 0) |
| PR Size | 15% | 100 if median PR < 200 lines changed; linear decay to 0 at 1000+ lines |
| Issue Health | 10% | 100 if fewer than 10 open issues; linear decay to 0 at 100 open issues |
| Deploy Frequency | 10% | Daily releases = 100; weekly = 80; monthly = 50; less frequent = 20 |
| Contributor Diversity | 10% | 100 if HHI <= 0.15; linear decay to 0 at HHI = 1.0 |

Score bands:

| Range | Label |
|-------|-------|
| 90-100 | Excellent |
| 75-89 | Healthy |
| 50-74 | Needs Attention |
| 0-49 | High Risk |

Any dimension scoring below 60 generates an actionable recommendation in the dashboard and CLI output.

### Bus Factor Algorithm

Contributors are sorted in descending order by their share of total commits over the configured lookback period. The bus factor is the minimum number of contributors whose cumulative share reaches or exceeds 50%. A bus factor of 1 means a single contributor accounts for the majority of codebase activity.

The HHI (Herfindahl-Hirschman Index) is the sum of each contributor's squared fractional share, on a 0-1 scale. An HHI above 0.25 is conventionally considered concentrated. The contributor diversity dimension uses HHI directly: low HHI scores well, high HHI scores poorly.

### PR Bottleneck Detection

A PR is classified as **stale** when it has been open longer than `stale_pr_days` (default: 7) and has had no activity — no PR update and no new review submission — in the past 3 days.

Each open PR is assigned a status based on its review state:
- `waiting_for_author` — the most recent review requested changes.
- `waiting_for_review` — there are pending review requests with no changes-requested outcome.
- `idle` — no pending review requests and no changes-requested outcome.

Reviewers are flagged as bottlenecks when their pending review request count exceeds twice the median pending count across all reviewers on open PRs.

## Configuration

All fields are optional. Create `.mergemetrics.yml` in your repository root to override defaults:

```yaml
# How far back to look when fetching closed PRs (days).
pr_lookback_days: 90

# How far back to look when fetching commits for bus factor analysis (days).
commit_lookback_days: 180

# Directory to write dashboard files into.
dashboard_path: docs

# URL of your GitHub Pages site. If set, the README badge links to this URL.
# Example: https://myorg.github.io/my-repo/
pages_url: ""

# Whether to update the README.md with the health badge. Set to false to disable.
update_readme: true

# Relative weights for each health score dimension. Must sum to 1.0.
weights:
  review_speed: 0.20
  bus_factor: 0.20
  pr_hygiene: 0.15
  pr_size: 0.15
  issue_health: 0.10
  deploy_frequency: 0.10
  contributor_diversity: 0.10

# Threshold values used when computing scores.
thresholds:
  # PRs open longer than this with no recent activity are considered stale.
  stale_pr_days: 7
  # Review times at or below this threshold score 100 on review speed (hours).
  review_time_excellent_hours: 4
  # Review times at or above this threshold score 0 on review speed (days).
  review_time_poor_days: 7
  # Bus factors at or above this threshold score the maximum step (75).
  bus_factor_good: 3
  # Median PR sizes at or below this line count score 100 on PR size.
  pr_size_good: 200
  # Median PR sizes at or above this line count score 0 on PR size.
  pr_size_poor: 1000
```

## Dashboard

MergeMetrics writes the following files to the output directory (default: `docs/`):

- `docs/index.html` — the main dashboard; responsive layout, dark mode support via `prefers-color-scheme`.
- `docs/data.json` — full structured data including all metric engine outputs; consumed by the dashboard.
- `docs/metrics.json` — flattened key-value metrics for programmatic use (CI gates, status pages, scripts).
- `docs/assets/style.css` — the dashboard stylesheet.

## README Badge

When `update_readme` is enabled (the default), MergeMetrics updates your README between a pair of HTML comment sentinels:

```markdown
<!-- mergemetrics-start -->
[![Repository Health](https://img.shields.io/badge/Health-82%2F100-green)](https://owner.github.io/repo/)

View Repository Dashboard: https://owner.github.io/repo/

Last updated: 2026-06-13
<!-- mergemetrics-end -->
```

Only the content between the sentinel markers is ever modified. Everything outside the markers is left exactly as-is. If the sentinels are not present, the block is appended to the end of the file.

To disable README updates, set `update_readme: false` in `.mergemetrics.yml`.

## Docker

```bash
docker build -t mergemetrics .
docker run -e GITHUB_TOKEN=$GITHUB_TOKEN mergemetrics generate --owner <owner> --repo <repo>
```

## Development

```bash
git clone https://github.com/jbsommeling/merge-metrics.git
cd merge-metrics
make build    # builds binary to bin/mergemetrics
make test     # runs all tests
make lint     # runs go vet
```

## License

MIT
