package daemon

import (
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
	cpuPct, _ := cpu.Percent(0, false)
	memStats, _ := mem.VirtualMemory()

	m := &Metrics{
		CPU:        cpuPct[0],
		MemPercent: memStats.UsedPercent,
		MemTotal:   memStats.Total,
		MemUsed:    memStats.Used,
	}
	return m, nil
}
