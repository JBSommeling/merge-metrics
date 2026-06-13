package dashboard

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"time"

	"github.com/jbsommeling/merge-metrics/internal/metrics/busfactor"
	"github.com/jbsommeling/merge-metrics/internal/metrics/health"
	"github.com/jbsommeling/merge-metrics/internal/metrics/prbottleneck"
	"github.com/jbsommeling/merge-metrics/internal/metrics/releases"
	"github.com/jbsommeling/merge-metrics/internal/metrics/reviews"
)

// DashboardData holds all data needed to render the dashboard.
type DashboardData struct {
	RepoName     string               `json:"repo_name"`
	RepoFullName string               `json:"repo_full_name"`
	GeneratedAt  time.Time            `json:"generated_at"`
	HealthScore  *health.Result       `json:"health_score"`
	PRBottleneck *prbottleneck.Result `json:"pr_bottleneck"`
	BusFactor    *busfactor.Result    `json:"bus_factor"`
	Reviews      *reviews.Result      `json:"reviews"`
	Releases     *releases.Result     `json:"releases"`
}

// metricsJSON is the flattened metrics-only representation for metrics.json.
type metricsJSON struct {
	HealthScore            int       `json:"health_score"`
	HealthBand             string    `json:"health_band"`
	OpenPRs                int       `json:"open_prs"`
	StalePRs               int       `json:"stale_prs"`
	AvgPRAgeDays           float64   `json:"avg_pr_age_days"`
	BusFactor              int       `json:"bus_factor"`
	HHI                    float64   `json:"hhi"`
	AvgReviewTimeHours     float64   `json:"avg_review_time_hours"`
	MedianReviewTimeHours  float64   `json:"median_review_time_hours"`
	P90ReviewTimeHours     float64   `json:"p90_review_time_hours"`
	ReleasesLastMonth      int       `json:"releases_last_month"`
	DeployFrequencyScore   int       `json:"deploy_frequency_score"`
	GeneratedAt            time.Time `json:"generated_at"`
}

// Generate creates outputDir/assets, writes data.json, metrics.json, assets/style.css, and index.html.
func Generate(data *DashboardData, outputDir string) error {
	assetsDir := filepath.Join(outputDir, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return fmt.Errorf("create output directories: %w", err)
	}

	if err := writeDataJSON(data, outputDir); err != nil {
		return err
	}
	if err := writeMetricsJSON(data, outputDir); err != nil {
		return err
	}
	if err := writeCSS(assetsDir); err != nil {
		return err
	}
	if err := writeHTML(data, outputDir); err != nil {
		return err
	}
	return nil
}

// writeDataJSON serialises the full DashboardData to data.json.
func writeDataJSON(data *DashboardData, outputDir string) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal data.json: %w", err)
	}
	return os.WriteFile(filepath.Join(outputDir, "data.json"), b, 0644)
}

// writeMetricsJSON writes a flattened metrics-only JSON to metrics.json.
func writeMetricsJSON(data *DashboardData, outputDir string) error {
	m := metricsJSON{GeneratedAt: data.GeneratedAt}

	if data.HealthScore != nil {
		m.HealthScore = data.HealthScore.TotalScore
		m.HealthBand = data.HealthScore.Band
	}
	if data.PRBottleneck != nil {
		m.OpenPRs = data.PRBottleneck.OpenPRCount
		m.StalePRs = len(data.PRBottleneck.StalePRs)
		m.AvgPRAgeDays = data.PRBottleneck.AveragePRAgeDays
	}
	if data.BusFactor != nil {
		m.BusFactor = data.BusFactor.BusFactor
		m.HHI = data.BusFactor.HHI
	}
	if data.Reviews != nil {
		m.AvgReviewTimeHours = data.Reviews.AverageReviewTime.Hours()
		m.MedianReviewTimeHours = data.Reviews.MedianReviewTime.Hours()
		m.P90ReviewTimeHours = data.Reviews.P90ReviewTime.Hours()
	}
	if data.Releases != nil {
		m.ReleasesLastMonth = data.Releases.ReleasesLastMonth
		m.DeployFrequencyScore = data.Releases.DeployFrequencyScore
	}

	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metrics.json: %w", err)
	}
	return os.WriteFile(filepath.Join(outputDir, "metrics.json"), b, 0644)
}

// writeCSS writes the stylesheet to assets/style.css.
func writeCSS(assetsDir string) error {
	return os.WriteFile(filepath.Join(assetsDir, "style.css"), []byte(cssContent), 0644)
}

// writeHTML renders index.html using html/template.
func writeHTML(data *DashboardData, outputDir string) error {
	funcMap := template.FuncMap{
		"formatDuration": formatDuration,
		"formatFloat":    formatFloat,
		"scoreColor":     scoreColor,
		"formatPercent":  formatPercent,
		"barWidth":       barWidth,
	}
	tmpl := template.Must(template.New("dashboard").Funcs(funcMap).Parse(htmlTemplate))
	f, err := os.Create(filepath.Join(outputDir, "index.html"))
	if err != nil {
		return fmt.Errorf("create index.html: %w", err)
	}
	defer f.Close()
	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("render index.html: %w", err)
	}
	return nil
}

// ── template helpers ──────────────────────────────────────────────────────────

// formatDuration returns a human-readable duration string.
func formatDuration(d time.Duration) string {
	h := d.Hours()
	if h < 1 {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if h < 24 {
		return fmt.Sprintf("%d hours", int(h))
	}
	return fmt.Sprintf("%d days", int(h/24))
}

// formatFloat formats a float64 to 1 decimal place.
func formatFloat(f float64) string {
	return fmt.Sprintf("%.1f", f)
}

// scoreColor returns a CSS class name for a health band string.
func scoreColor(band string) string {
	switch band {
	case "Excellent":
		return "score-excellent"
	case "Healthy":
		return "score-healthy"
	case "Needs Attention":
		return "score-attention"
	default:
		return "score-risk"
	}
}

// formatPercent formats a float64 as a percentage string with 1 decimal place.
func formatPercent(f float64) string {
	return fmt.Sprintf("%.1f%%", f)
}

// barWidth returns a CSS percentage string for a bar chart, capping at 100%.
func barWidth(count int, maxCount int) string {
	if maxCount <= 0 {
		return "0%"
	}
	pct := float64(count) / float64(maxCount) * 100
	if pct > 100 {
		pct = 100
	}
	return fmt.Sprintf("%.0f%%", pct)
}

// ── embedded assets ───────────────────────────────────────────────────────────

const cssContent = `:root {
  --color-bg: #f8fafc;
  --color-surface: #ffffff;
  --color-border: #e2e8f0;
  --color-text: #1e293b;
  --color-text-muted: #64748b;
  --color-green: #22c55e;
  --color-yellow: #eab308;
  --color-orange: #f97316;
  --color-red: #ef4444;
  --shadow: 0 1px 3px rgba(0,0,0,.08), 0 1px 2px rgba(0,0,0,.06);
  --radius: 12px;
  --font: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
}

@media (prefers-color-scheme: dark) {
  :root {
    --color-bg: #0f172a;
    --color-surface: #1e293b;
    --color-border: #334155;
    --color-text: #f1f5f9;
    --color-text-muted: #94a3b8;
    --shadow: 0 1px 3px rgba(0,0,0,.4);
  }
}

*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

body {
  font-family: var(--font);
  background: var(--color-bg);
  color: var(--color-text);
  line-height: 1.6;
  padding: 24px 16px;
}

header {
  max-width: 1200px;
  margin: 0 auto 32px;
}

header h1 { font-size: 1.75rem; font-weight: 700; }
header p  { color: var(--color-text-muted); font-size: 0.9rem; margin-top: 4px; }

.grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 20px;
  max-width: 1200px;
  margin: 0 auto;
}

@media (max-width: 768px) {
  .grid { grid-template-columns: 1fr; }
}

.card {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  box-shadow: var(--shadow);
  padding: 24px;
}

.card-full { grid-column: 1 / -1; }

.card h2 {
  font-size: 1rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: .05em;
  color: var(--color-text-muted);
  margin-bottom: 16px;
}

/* Health circle */
.health-circle {
  width: 120px;
  height: 120px;
  border-radius: 50%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  margin: 0 auto 16px;
  border: 6px solid currentColor;
}

.health-circle .score-num  { font-size: 2.5rem; font-weight: 800; line-height: 1; }
.health-circle .score-label { font-size: 0.65rem; text-transform: uppercase; letter-spacing: .08em; }

.score-excellent { color: var(--color-green); }
.score-healthy   { color: var(--color-yellow); }
.score-attention { color: var(--color-orange); }
.score-risk      { color: var(--color-red); }

/* Tables */
table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.875rem;
}

th, td {
  text-align: left;
  padding: 8px 10px;
  border-bottom: 1px solid var(--color-border);
}

th { font-weight: 600; color: var(--color-text-muted); }

tr:last-child td { border-bottom: none; }

/* Bar chart */
.bar-row { margin-bottom: 10px; }
.bar-label { display: flex; justify-content: space-between; font-size: 0.8rem; margin-bottom: 3px; }
.bar-track { background: var(--color-border); border-radius: 4px; height: 10px; }
.bar-fill  { height: 10px; border-radius: 4px; background: var(--color-green); min-width: 2px; }
.bar-fill.bottleneck { background: var(--color-red); }

/* Warnings */
.warning {
  background: #fff7ed;
  border-left: 4px solid var(--color-orange);
  border-radius: 4px;
  padding: 10px 14px;
  font-size: 0.875rem;
  margin-top: 12px;
  color: #9a3412;
}

@media (prefers-color-scheme: dark) {
  .warning { background: #431407; color: #fdba74; }
}

/* Recommendations */
.rec-list { list-style: none; }
.rec-list li {
  padding: 10px 0;
  border-bottom: 1px solid var(--color-border);
  font-size: 0.9rem;
  padding-left: 20px;
  position: relative;
}
.rec-list li::before {
  content: "→";
  position: absolute;
  left: 0;
  color: var(--color-text-muted);
}
.rec-list li:last-child { border-bottom: none; }

/* Trend badge */
.trend { display: inline-block; padding: 2px 8px; border-radius: 20px; font-size: 0.75rem; font-weight: 600; }
.trend-increasing { background: #dcfce7; color: #166534; }
.trend-decreasing { background: #fee2e2; color: #991b1b; }
.trend-stable     { background: #f1f5f9; color: #475569; }

@media (prefers-color-scheme: dark) {
  .trend-increasing { background: #14532d; color: #86efac; }
  .trend-decreasing { background: #450a0a; color: #fca5a5; }
  .trend-stable     { background: #1e293b; color: #94a3b8; }
}

.stat-row { display: flex; gap: 24px; flex-wrap: wrap; margin-bottom: 16px; }
.stat { flex: 1; min-width: 100px; }
.stat-value { font-size: 1.5rem; font-weight: 700; }
.stat-desc  { font-size: 0.75rem; color: var(--color-text-muted); }
`

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>MergeMetrics — {{.RepoName}}</title>
  <link rel="stylesheet" href="assets/style.css">
</head>
<body>
<header>
  <h1>MergeMetrics</h1>
  <p>{{.RepoFullName}} &bull; Last updated {{.GeneratedAt.Format "2006-01-02 15:04 UTC"}}</p>
</header>

<div class="grid">

  {{/* ── Health Score ──────────────────────────────────────────── */}}
  <div class="card">
    <h2>Health Score</h2>
    {{if .HealthScore}}
    <div class="health-circle {{scoreColor .HealthScore.Band}}">
      <span class="score-num">{{.HealthScore.TotalScore}}</span>
      <span class="score-label">/ 100</span>
    </div>
    <p style="text-align:center;font-weight:600;margin-bottom:16px;" class="{{scoreColor .HealthScore.Band}}">{{.HealthScore.Band}}</p>
    <table>
      <thead><tr><th>Category</th><th>Score</th><th>Detail</th></tr></thead>
      <tbody>
      {{range .HealthScore.Categories}}
        <tr>
          <td>{{.Name}}</td>
          <td>{{.Score}}</td>
          <td style="color:var(--color-text-muted);font-size:0.8rem">{{.Detail}}</td>
        </tr>
      {{end}}
      </tbody>
    </table>
    {{else}}
    <p style="color:var(--color-text-muted)">No data available.</p>
    {{end}}
  </div>

  {{/* ── PR Bottlenecks ────────────────────────────────────────── */}}
  <div class="card">
    <h2>PR Bottlenecks</h2>
    {{if .PRBottleneck}}
    <div class="stat-row">
      <div class="stat">
        <div class="stat-value">{{.PRBottleneck.OpenPRCount}}</div>
        <div class="stat-desc">Open PRs</div>
      </div>
      <div class="stat">
        <div class="stat-value">{{len .PRBottleneck.StalePRs}}</div>
        <div class="stat-desc">Stale PRs</div>
      </div>
      <div class="stat">
        <div class="stat-value">{{formatFloat .PRBottleneck.AveragePRAgeDays}}</div>
        <div class="stat-desc">Avg age (days)</div>
      </div>
    </div>

    {{if .PRBottleneck.ReviewerWorkload}}
    <h3 style="font-size:0.85rem;margin-bottom:10px;color:var(--color-text-muted)">Reviewer Workload</h3>
    {{range .PRBottleneck.ReviewerWorkload}}
    <div class="bar-row">
      <div class="bar-label">
        <span>{{.Login}}{{if .IsBottleneck}} ⚠{{end}}</span>
        <span>{{.PendingCount}} pending</span>
      </div>
      <div class="bar-track">
        <div class="bar-fill{{if .IsBottleneck}} bottleneck{{end}}" style="width:{{barWidth .PendingCount 10}}"></div>
      </div>
    </div>
    {{end}}
    {{end}}

    {{if .PRBottleneck.StalePRs}}
    <h3 style="font-size:0.85rem;margin:16px 0 8px;color:var(--color-text-muted)">Stale PRs</h3>
    <table>
      <thead><tr><th>#</th><th>Title</th><th>Author</th><th>Days open</th><th>Status</th></tr></thead>
      <tbody>
      {{range .PRBottleneck.StalePRs}}
        <tr>
          <td>{{.Number}}</td>
          <td>{{.Title}}</td>
          <td>{{.Author}}</td>
          <td>{{.OpenDays}}</td>
          <td>{{.Status}}</td>
        </tr>
      {{end}}
      </tbody>
    </table>
    {{end}}

    {{range .PRBottleneck.Bottlenecks}}
    <div class="warning">{{.}}</div>
    {{end}}
    {{else}}
    <p style="color:var(--color-text-muted)">No data available.</p>
    {{end}}
  </div>

  {{/* ── Review Analytics ──────────────────────────────────────── */}}
  <div class="card">
    <h2>Review Analytics</h2>
    {{if .Reviews}}
    <div class="stat-row">
      <div class="stat">
        <div class="stat-value">{{formatDuration .Reviews.AverageReviewTime}}</div>
        <div class="stat-desc">Mean review time</div>
      </div>
      <div class="stat">
        <div class="stat-value">{{formatDuration .Reviews.MedianReviewTime}}</div>
        <div class="stat-desc">Median review time</div>
      </div>
      <div class="stat">
        <div class="stat-value">{{formatDuration .Reviews.P90ReviewTime}}</div>
        <div class="stat-desc">P90 review time</div>
      </div>
    </div>
    <div class="stat-row">
      <div class="stat">
        <div class="stat-value">{{.Reviews.TotalReviews}}</div>
        <div class="stat-desc">Total reviews</div>
      </div>
      <div class="stat">
        <div class="stat-value">{{formatFloat .Reviews.ReviewThroughput}}</div>
        <div class="stat-desc">Reviews / week</div>
      </div>
    </div>
    {{if .Reviews.ReviewerStats}}
    <table>
      <thead><tr><th>Reviewer</th><th>Avg response</th><th>Reviews</th><th>/ week</th></tr></thead>
      <tbody>
      {{range .Reviews.ReviewerStats}}
        <tr>
          <td>{{.Login}}</td>
          <td>{{formatDuration .AvgResponseTime}}</td>
          <td>{{.ReviewCount}}</td>
          <td>{{formatFloat .ReviewsPerWeek}}</td>
        </tr>
      {{end}}
      </tbody>
    </table>
    {{end}}
    {{else}}
    <p style="color:var(--color-text-muted)">No data available.</p>
    {{end}}
  </div>

  {{/* ── Contributors / Bus Factor ─────────────────────────────── */}}
  <div class="card">
    <h2>Contributors</h2>
    {{if .BusFactor}}
    <div class="stat-row">
      <div class="stat">
        <div class="stat-value">{{.BusFactor.BusFactor}}</div>
        <div class="stat-desc">Bus factor</div>
      </div>
      <div class="stat">
        <div class="stat-value">{{formatFloat .BusFactor.HHI}}</div>
        <div class="stat-desc">HHI</div>
      </div>
      <div class="stat">
        <div class="stat-value">{{.BusFactor.TotalCommits}}</div>
        <div class="stat-desc">Total commits</div>
      </div>
    </div>
    {{range .BusFactor.Contributors}}
    <div class="bar-row">
      <div class="bar-label">
        <span>{{.Login}}</span>
        <span>{{formatPercent .SharePct}} ({{.Commits}} commits)</span>
      </div>
      <div class="bar-track">
        <div class="bar-fill" style="width:{{formatPercent .SharePct}}"></div>
      </div>
    </div>
    {{end}}
    {{else}}
    <p style="color:var(--color-text-muted)">No data available.</p>
    {{end}}
  </div>

  {{/* ── Releases ──────────────────────────────────────────────── */}}
  <div class="card">
    <h2>Releases</h2>
    {{if .Releases}}
    <div class="stat-row">
      <div class="stat">
        <div class="stat-value">{{.Releases.ReleasesLastMonth}}</div>
        <div class="stat-desc">Last 30 days</div>
      </div>
      <div class="stat">
        <div class="stat-value">{{.Releases.ReleasesLastWeek}}</div>
        <div class="stat-desc">Last 7 days</div>
      </div>
      <div class="stat">
        <div class="stat-value">{{.Releases.DeployFrequencyScore}}</div>
        <div class="stat-desc">Deploy score</div>
      </div>
    </div>
    <p>
      Trend:
      <span class="trend trend-{{.Releases.Trend}}">{{.Releases.Trend}}</span>
    </p>
    {{if .Releases.AvgTimeBetweenReleases}}
    <p style="margin-top:10px;font-size:0.875rem;color:var(--color-text-muted)">
      Avg time between releases: {{formatDuration .Releases.AvgTimeBetweenReleases}}
    </p>
    {{end}}
    {{else}}
    <p style="color:var(--color-text-muted)">No data available.</p>
    {{end}}
  </div>

  {{/* ── Recommendations ───────────────────────────────────────── */}}
  {{if .HealthScore}}{{if .HealthScore.Recommendations}}
  <div class="card card-full">
    <h2>Recommendations</h2>
    <ul class="rec-list">
      {{range .HealthScore.Recommendations}}
      <li>{{.}}</li>
      {{end}}
    </ul>
  </div>
  {{end}}{{end}}

</div><!-- .grid -->
</body>
</html>
`
