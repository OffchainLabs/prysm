package evaluators

import (
	"strings"
	"testing"
)

func TestMetricCheckComparison(t *testing.T) {
	tests := []struct {
		name        string
		pageContent string
		topic1      string
		topic2      string
		comparison  float64
		wantErr     bool
	}{
		{
			name: "valid metrics with ratio below threshold",
			pageContent: `# HELP committee_cache_miss Test metric
# TYPE committee_cache_miss counter
committee_cache_miss 10
# HELP committee_cache_hit Test metric
# TYPE committee_cache_hit counter
committee_cache_hit 1000
`,
			topic1:     "committee_cache_miss",
			topic2:     "committee_cache_hit",
			comparison: 0.05,
			wantErr:    false,
		},
		{
			name: "valid metrics with ratio exceeding threshold",
			pageContent: `# HELP committee_cache_miss Test metric
# TYPE committee_cache_miss counter
committee_cache_miss 40
# HELP committee_cache_hit Test metric
# TYPE committee_cache_hit counter
committee_cache_hit 2946
`,
			topic1:     "committee_cache_miss",
			topic2:     "committee_cache_hit",
			comparison: 0.01,
			wantErr:    true,
		},
		{
			name: "topic2 not found should skip test",
			pageContent: `# HELP committee_cache_miss Test metric
# TYPE committee_cache_miss counter
committee_cache_miss 10
`,
			topic1:     "committee_cache_miss",
			topic2:     "missing_cache_hit",
			comparison: 0.01,
			wantErr:    false,
		},
		{
			name: "topic1 not found should skip test",
			pageContent: `# HELP committee_cache_hit Test metric
# TYPE committee_cache_hit counter
committee_cache_hit 1000
`,
			topic1:     "missing_cache_miss",
			topic2:     "committee_cache_hit",
			comparison: 0.01,
			wantErr:    false,
		},
		{
			name: "both topics missing should skip test",
			pageContent: `# HELP some_other_metric Test metric
# TYPE some_other_metric counter
some_other_metric 100
`,
			topic1:     "missing_metric_1",
			topic2:     "missing_metric_2",
			comparison: 0.01,
			wantErr:    false,
		},
		{
			name: "hot state cache miss ratio above threshold",
			pageContent: `# HELP hot_state_cache_miss Test metric
# TYPE hot_state_cache_miss counter
hot_state_cache_miss 1
# HELP hot_state_cache_hit Test metric
# TYPE hot_state_cache_hit counter
hot_state_cache_hit 41
`,
			topic1:     "hot_state_cache_miss",
			topic2:     "hot_state_cache_hit",
			comparison: 0.01,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := metricCheckComparison(tt.pageContent, tt.topic1, tt.topic2, tt.comparison)
			if (err != nil) != tt.wantErr {
				t.Errorf("metricCheckComparison() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValueOfTopic(t *testing.T) {
	tests := []struct {
		name        string
		pageContent string
		topic       string
		want        int
		wantErr     bool
	}{
		{
			name: "finds single metric value",
			pageContent: `# HELP test_metric Test metric help
# TYPE test_metric gauge
test_metric 42
`,
			topic:   "test_metric",
			want:    42,
			wantErr: false,
		},
		{
			name: "metric not found returns error",
			pageContent: `# HELP some_metric Some metric help
# TYPE some_metric gauge
some_metric 10
`,
			topic:   "missing_metric",
			want:    -1,
			wantErr: true,
		},
		{
			name: "finds float metric value",
			pageContent: `# HELP cache_hit_rate Cache hit rate help
# TYPE cache_hit_rate gauge
cache_hit_rate 0.95
`,
			topic:   "cache_hit_rate",
			want:    0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := valueOfTopic(tt.pageContent, tt.topic)
			if (err != nil) != tt.wantErr {
				t.Errorf("valueOfTopic() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("valueOfTopic() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMetricCheckComparisonFix(t *testing.T) {
	t.Run("fix verification: comparison actually happens when metrics found", func(t *testing.T) {
		pageContent := `# HELP committee_cache_miss Test metric
# TYPE committee_cache_miss counter
committee_cache_miss 40
# HELP committee_cache_hit Test metric
# TYPE committee_cache_hit counter
committee_cache_hit 2946
`
		err := metricCheckComparison(pageContent, "committee_cache_miss", "committee_cache_hit", 0.01)
		if err == nil {
			t.Error("Expected error when ratio 40/2946=0.0135 exceeds threshold 0.01, but got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "unexpected result") {
			t.Errorf("Expected error about comparison, got: %v", err)
		}
	})

	t.Run("fix verification: test passes when ratio below threshold", func(t *testing.T) {
		pageContent := `# HELP committee_cache_miss Test metric
# TYPE committee_cache_miss counter
committee_cache_miss 5
# HELP committee_cache_hit Test metric
# TYPE committee_cache_hit counter
committee_cache_hit 2946
`
		err := metricCheckComparison(pageContent, "committee_cache_miss", "committee_cache_hit", 0.01)
		if err != nil {
			t.Errorf("Expected no error when ratio 5/2946=0.0017 is below threshold 0.01, got: %v", err)
		}
	})
}
