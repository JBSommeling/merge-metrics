package config

import (
	"fmt"
	"math"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds all MergeMetrics configuration options.
type Config struct {
	PRLookbackDays     int             `yaml:"pr_lookback_days"`
	CommitLookbackDays int             `yaml:"commit_lookback_days"`
	DashboardPath      string          `yaml:"dashboard_path"`
	PagesURL             string          `yaml:"pages_url"`
	UpdateReadme         bool            `yaml:"update_readme"`
	Weights              WeightConfig    `yaml:"weights"`
	Thresholds           ThresholdConfig `yaml:"thresholds"`
}

// WeightConfig holds relative weights for each scoring dimension.
// Weights should sum to 1.0.
type WeightConfig struct {
	ReviewSpeed          float64 `yaml:"review_speed"`
	BusFactor            float64 `yaml:"bus_factor"`
	PRHygiene            float64 `yaml:"pr_hygiene"`
	PRSize               float64 `yaml:"pr_size"`
	IssueHealth          float64 `yaml:"issue_health"`
	DeployFrequency      float64 `yaml:"deploy_frequency"`
	ContributorDiversity float64 `yaml:"contributor_diversity"`
}

// ThresholdConfig holds threshold values used when computing scores.
type ThresholdConfig struct {
	StalePRDays              int `yaml:"stale_pr_days"`
	ReviewTimeExcellentHours int `yaml:"review_time_excellent_hours"`
	ReviewTimePoorDays       int `yaml:"review_time_poor_days"`
	BusFactorGood            int `yaml:"bus_factor_good"`
	PRSizeGood               int `yaml:"pr_size_good"`
	PRSizePoor               int `yaml:"pr_size_poor"`
}

// Default returns a Config with all fields set to their default values.
func Default() *Config {
	return &Config{
		PRLookbackDays:     90,
		CommitLookbackDays: 180,
		DashboardPath:      "docs",
		PagesURL:             "",
		UpdateReadme:         true,
		Weights: WeightConfig{
			ReviewSpeed:          0.20,
			BusFactor:            0.20,
			PRHygiene:            0.15,
			PRSize:               0.15,
			IssueHealth:          0.10,
			DeployFrequency:      0.10,
			ContributorDiversity: 0.10,
		},
		Thresholds: ThresholdConfig{
			StalePRDays:              7,
			ReviewTimeExcellentHours: 4,
			ReviewTimePoorDays:       7,
			BusFactorGood:            3,
			PRSizeGood:               200,
			PRSizePoor:               1000,
		},
	}
}

// Load reads a YAML config file from path and returns a Config with defaults
// applied for any fields not present in the file. If the file does not exist,
// all defaults are returned without error.
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("config: reading %s: %w", path, err)
	}

	// Unmarshal into a temporary struct so that zero-value fields in the YAML
	// do not overwrite defaults for fields that were simply omitted.
	type partialWeights struct {
		ReviewSpeed          *float64 `yaml:"review_speed"`
		BusFactor            *float64 `yaml:"bus_factor"`
		PRHygiene            *float64 `yaml:"pr_hygiene"`
		PRSize               *float64 `yaml:"pr_size"`
		IssueHealth          *float64 `yaml:"issue_health"`
		DeployFrequency      *float64 `yaml:"deploy_frequency"`
		ContributorDiversity *float64 `yaml:"contributor_diversity"`
	}
	type partialThresholds struct {
		StalePRDays              *int `yaml:"stale_pr_days"`
		ReviewTimeExcellentHours *int `yaml:"review_time_excellent_hours"`
		ReviewTimePoorDays       *int `yaml:"review_time_poor_days"`
		BusFactorGood            *int `yaml:"bus_factor_good"`
		PRSizeGood               *int `yaml:"pr_size_good"`
		PRSizePoor               *int `yaml:"pr_size_poor"`
	}
	type partialConfig struct {
		PRLookbackDays     *int               `yaml:"pr_lookback_days"`
		CommitLookbackDays *int               `yaml:"commit_lookback_days"`
		DashboardPath      *string            `yaml:"dashboard_path"`
		PagesURL             *string            `yaml:"pages_url"`
		UpdateReadme         *bool              `yaml:"update_readme"`
		Weights              *partialWeights    `yaml:"weights"`
		Thresholds           *partialThresholds `yaml:"thresholds"`
	}

	var partial partialConfig
	if err := yaml.Unmarshal(data, &partial); err != nil {
		return nil, fmt.Errorf("config: parsing %s: %w", path, err)
	}

	// Apply top-level fields.
	if partial.PRLookbackDays != nil {
		cfg.PRLookbackDays = *partial.PRLookbackDays
	}
	if partial.CommitLookbackDays != nil {
		cfg.CommitLookbackDays = *partial.CommitLookbackDays
	}
	if partial.DashboardPath != nil {
		cfg.DashboardPath = *partial.DashboardPath
	}
	if partial.PagesURL != nil {
		cfg.PagesURL = *partial.PagesURL
	}
	if partial.UpdateReadme != nil {
		cfg.UpdateReadme = *partial.UpdateReadme
	}

	// Apply weight fields.
	if w := partial.Weights; w != nil {
		if w.ReviewSpeed != nil {
			cfg.Weights.ReviewSpeed = *w.ReviewSpeed
		}
		if w.BusFactor != nil {
			cfg.Weights.BusFactor = *w.BusFactor
		}
		if w.PRHygiene != nil {
			cfg.Weights.PRHygiene = *w.PRHygiene
		}
		if w.PRSize != nil {
			cfg.Weights.PRSize = *w.PRSize
		}
		if w.IssueHealth != nil {
			cfg.Weights.IssueHealth = *w.IssueHealth
		}
		if w.DeployFrequency != nil {
			cfg.Weights.DeployFrequency = *w.DeployFrequency
		}
		if w.ContributorDiversity != nil {
			cfg.Weights.ContributorDiversity = *w.ContributorDiversity
		}
	}

	// Apply threshold fields.
	if t := partial.Thresholds; t != nil {
		if t.StalePRDays != nil {
			cfg.Thresholds.StalePRDays = *t.StalePRDays
		}
		if t.ReviewTimeExcellentHours != nil {
			cfg.Thresholds.ReviewTimeExcellentHours = *t.ReviewTimeExcellentHours
		}
		if t.ReviewTimePoorDays != nil {
			cfg.Thresholds.ReviewTimePoorDays = *t.ReviewTimePoorDays
		}
		if t.BusFactorGood != nil {
			cfg.Thresholds.BusFactorGood = *t.BusFactorGood
		}
		if t.PRSizeGood != nil {
			cfg.Thresholds.PRSizeGood = *t.PRSizeGood
		}
		if t.PRSizePoor != nil {
			cfg.Thresholds.PRSizePoor = *t.PRSizePoor
		}
	}

	return cfg, nil
}

// Validate checks that the configuration is coherent. It returns an error if:
//   - weights do not sum to approximately 1.0 (within ±0.01)
//   - any weight value is negative
//   - PRLookbackDays or CommitLookbackDays are not > 0
//   - any threshold value is not > 0
func (c *Config) Validate() error {
	// Check for negative weights.
	weights := map[string]float64{
		"review_speed":          c.Weights.ReviewSpeed,
		"bus_factor":            c.Weights.BusFactor,
		"pr_hygiene":            c.Weights.PRHygiene,
		"pr_size":               c.Weights.PRSize,
		"issue_health":          c.Weights.IssueHealth,
		"deploy_frequency":      c.Weights.DeployFrequency,
		"contributor_diversity": c.Weights.ContributorDiversity,
	}
	for name, v := range weights {
		if v < 0 {
			return fmt.Errorf("config: weight %q must be >= 0.0 (got %f)", name, v)
		}
	}

	// Check weights sum to 1.0.
	sum := c.Weights.ReviewSpeed +
		c.Weights.BusFactor +
		c.Weights.PRHygiene +
		c.Weights.PRSize +
		c.Weights.IssueHealth +
		c.Weights.DeployFrequency +
		c.Weights.ContributorDiversity

	if math.Abs(sum-1.0) > 0.01 {
		return fmt.Errorf("config: weights must sum to 1.0 (got %.4f)", sum)
	}

	// Check lookback days are > 0.
	if c.PRLookbackDays <= 0 {
		return fmt.Errorf("config: pr_lookback_days must be > 0 (got %d)", c.PRLookbackDays)
	}
	if c.CommitLookbackDays <= 0 {
		return fmt.Errorf("config: commit_lookback_days must be > 0 (got %d)", c.CommitLookbackDays)
	}

	// Check threshold values are > 0.
	if c.Thresholds.StalePRDays <= 0 {
		return fmt.Errorf("config: thresholds.stale_pr_days must be > 0 (got %d)", c.Thresholds.StalePRDays)
	}
	if c.Thresholds.ReviewTimeExcellentHours <= 0 {
		return fmt.Errorf("config: thresholds.review_time_excellent_hours must be > 0 (got %d)", c.Thresholds.ReviewTimeExcellentHours)
	}
	if c.Thresholds.ReviewTimePoorDays <= 0 {
		return fmt.Errorf("config: thresholds.review_time_poor_days must be > 0 (got %d)", c.Thresholds.ReviewTimePoorDays)
	}
	if c.Thresholds.BusFactorGood <= 0 {
		return fmt.Errorf("config: thresholds.bus_factor_good must be > 0 (got %d)", c.Thresholds.BusFactorGood)
	}
	if c.Thresholds.PRSizeGood <= 0 {
		return fmt.Errorf("config: thresholds.pr_size_good must be > 0 (got %d)", c.Thresholds.PRSizeGood)
	}
	if c.Thresholds.PRSizePoor <= 0 {
		return fmt.Errorf("config: thresholds.pr_size_poor must be > 0 (got %d)", c.Thresholds.PRSizePoor)
	}

	return nil
}
