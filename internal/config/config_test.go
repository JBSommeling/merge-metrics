package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTemp writes content to a temporary file and returns its path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".mergemetrics.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTemp: %v", err)
	}
	return path
}

func TestLoad_MissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load("/nonexistent/path/.mergemetrics.yml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}

	def := Default()
	if cfg.PRLookbackDays != def.PRLookbackDays {
		t.Errorf("PRLookbackDays: got %d, want %d", cfg.PRLookbackDays, def.PRLookbackDays)
	}
	if cfg.CommitLookbackDays != def.CommitLookbackDays {
		t.Errorf("CommitLookbackDays: got %d, want %d", cfg.CommitLookbackDays, def.CommitLookbackDays)
	}
	if cfg.StalePRThresholdDays != def.StalePRThresholdDays {
		t.Errorf("StalePRThresholdDays: got %d, want %d", cfg.StalePRThresholdDays, def.StalePRThresholdDays)
	}
	if cfg.DashboardPath != def.DashboardPath {
		t.Errorf("DashboardPath: got %q, want %q", cfg.DashboardPath, def.DashboardPath)
	}
	if cfg.UpdateReadme != def.UpdateReadme {
		t.Errorf("UpdateReadme: got %v, want %v", cfg.UpdateReadme, def.UpdateReadme)
	}
	if cfg.Weights != def.Weights {
		t.Errorf("Weights: got %+v, want %+v", cfg.Weights, def.Weights)
	}
	if cfg.Thresholds != def.Thresholds {
		t.Errorf("Thresholds: got %+v, want %+v", cfg.Thresholds, def.Thresholds)
	}
}

func TestLoad_PartialFileAppliesDefaults(t *testing.T) {
	yaml := `
pr_lookback_days: 30
weights:
  review_speed: 0.30
`
	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Overridden value.
	if cfg.PRLookbackDays != 30 {
		t.Errorf("PRLookbackDays: got %d, want 30", cfg.PRLookbackDays)
	}
	// Overridden weight.
	if cfg.Weights.ReviewSpeed != 0.30 {
		t.Errorf("ReviewSpeed: got %f, want 0.30", cfg.Weights.ReviewSpeed)
	}

	// Remaining fields must carry defaults.
	def := Default()
	if cfg.CommitLookbackDays != def.CommitLookbackDays {
		t.Errorf("CommitLookbackDays: got %d, want %d", cfg.CommitLookbackDays, def.CommitLookbackDays)
	}
	if cfg.DashboardPath != def.DashboardPath {
		t.Errorf("DashboardPath: got %q, want %q", cfg.DashboardPath, def.DashboardPath)
	}
	if cfg.Weights.BusFactor != def.Weights.BusFactor {
		t.Errorf("BusFactor: got %f, want %f", cfg.Weights.BusFactor, def.Weights.BusFactor)
	}
	if cfg.Thresholds != def.Thresholds {
		t.Errorf("Thresholds: got %+v, want %+v", cfg.Thresholds, def.Thresholds)
	}
}

func TestLoad_FullFileUsesProvidedValues(t *testing.T) {
	yaml := `
pr_lookback_days: 60
commit_lookback_days: 120
stale_pr_threshold_days: 14
dashboard_path: "site"
pages_url: "https://example.com"
update_readme: false
weights:
  review_speed: 0.10
  bus_factor: 0.10
  pr_hygiene: 0.20
  pr_size: 0.20
  issue_health: 0.15
  deploy_frequency: 0.15
  contributor_diversity: 0.10
thresholds:
  stale_pr_days: 10
  review_time_excellent_hours: 2
  review_time_poor_days: 5
  bus_factor_good: 5
  pr_size_good: 100
  pr_size_poor: 500
`
	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.PRLookbackDays != 60 {
		t.Errorf("PRLookbackDays: got %d, want 60", cfg.PRLookbackDays)
	}
	if cfg.CommitLookbackDays != 120 {
		t.Errorf("CommitLookbackDays: got %d, want 120", cfg.CommitLookbackDays)
	}
	if cfg.StalePRThresholdDays != 14 {
		t.Errorf("StalePRThresholdDays: got %d, want 14", cfg.StalePRThresholdDays)
	}
	if cfg.DashboardPath != "site" {
		t.Errorf("DashboardPath: got %q, want site", cfg.DashboardPath)
	}
	if cfg.PagesURL != "https://example.com" {
		t.Errorf("PagesURL: got %q, want https://example.com", cfg.PagesURL)
	}
	if cfg.UpdateReadme != false {
		t.Errorf("UpdateReadme: got true, want false")
	}
	if cfg.Weights.ReviewSpeed != 0.10 {
		t.Errorf("ReviewSpeed: got %f, want 0.10", cfg.Weights.ReviewSpeed)
	}
	if cfg.Weights.PRHygiene != 0.20 {
		t.Errorf("PRHygiene: got %f, want 0.20", cfg.Weights.PRHygiene)
	}
	if cfg.Thresholds.StalePRDays != 10 {
		t.Errorf("StalePRDays: got %d, want 10", cfg.Thresholds.StalePRDays)
	}
	if cfg.Thresholds.PRSizeGood != 100 {
		t.Errorf("PRSizeGood: got %d, want 100", cfg.Thresholds.PRSizeGood)
	}
	if cfg.Thresholds.PRSizePoor != 500 {
		t.Errorf("PRSizePoor: got %d, want 500", cfg.Thresholds.PRSizePoor)
	}
}

func TestValidate_FailsWhenWeightsDontSumToOne(t *testing.T) {
	cfg := Default()
	cfg.Weights.ReviewSpeed = 0.50 // sum will be well over 1.0
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error when weights don't sum to 1.0, got nil")
	}
}

func TestValidate_PassesWithDefaultWeights(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no validation error for default weights, got: %v", err)
	}
}

func TestValidate_FailsOnNegativeWeights(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"review_speed", func(c *Config) { c.Weights.ReviewSpeed = -0.1 }},
		{"bus_factor", func(c *Config) { c.Weights.BusFactor = -0.1 }},
		{"pr_hygiene", func(c *Config) { c.Weights.PRHygiene = -0.1 }},
		{"pr_size", func(c *Config) { c.Weights.PRSize = -0.1 }},
		{"issue_health", func(c *Config) { c.Weights.IssueHealth = -0.1 }},
		{"deploy_frequency", func(c *Config) { c.Weights.DeployFrequency = -0.1 }},
		{"contributor_diversity", func(c *Config) { c.Weights.ContributorDiversity = -0.1 }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			tc.mutate(cfg)
			if err := cfg.Validate(); err == nil {
				t.Errorf("expected validation error for negative weight %q, got nil", tc.name)
			}
		})
	}
}

func TestValidate_FailsOnNegativeLookbackDays(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"pr_lookback_days=0", func(c *Config) { c.PRLookbackDays = 0 }},
		{"pr_lookback_days=-1", func(c *Config) { c.PRLookbackDays = -1 }},
		{"commit_lookback_days=0", func(c *Config) { c.CommitLookbackDays = 0 }},
		{"commit_lookback_days=-1", func(c *Config) { c.CommitLookbackDays = -1 }},
		{"stale_pr_threshold_days=0", func(c *Config) { c.StalePRThresholdDays = 0 }},
		{"stale_pr_threshold_days=-1", func(c *Config) { c.StalePRThresholdDays = -1 }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			tc.mutate(cfg)
			if err := cfg.Validate(); err == nil {
				t.Errorf("expected validation error for %q, got nil", tc.name)
			}
		})
	}
}
