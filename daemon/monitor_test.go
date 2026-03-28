package daemon

import (
	"testing"
)

func TestCollectMetrics(t *testing.T) {
	metrics, err := CollectMetrics()
	if err != nil {
		t.Fatalf("CollectMetrics() error = %v", err)
	}

	if metrics.CPU < 0 || metrics.CPU > 100 {
		t.Errorf("Expected CPU to be between 0 and 100, got %f", metrics.CPU)
	}

	if metrics.MemPercent < 0 || metrics.MemPercent > 100 {
		t.Errorf("Expected MemPercent to be between 0 and 100, got %f", metrics.MemPercent)
	}

	if metrics.MemTotal == 0 {
		t.Error("Expected MemTotal to be non-zero")
	}
}
