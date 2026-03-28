package daemon

import (
	"fmt"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

type Metrics struct {
	CPU        float64
	MemPercent float64
	MemTotal   uint64
	MemUsed    uint64
}

func CollectMetrics() (*Metrics, error) {
	cpuPct, err := cpu.Percent(0, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU percent: %w", err)
	}

	memStats, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get memory stats: %w", err)
	}

	return &Metrics{
		CPU:        cpuPct[0],
		MemPercent: memStats.UsedPercent,
		MemTotal:   memStats.Total,
		MemUsed:    memStats.Used,
	}, nil
}
