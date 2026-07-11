//go:build !linux

package adminstatus

import "time"

type HostSnapshot struct {
	CPUs    []CPUStat    `json:"cpus"`
	Memory  MemoryStat   `json:"memory"`
	Storage StorageStat  `json:"storage"`
	Network NetworkStat  `json:"network"`
	Updated time.Time    `json:"updated_at"`
}

type CPUStat struct {
	ID       string  `json:"id"`
	UsagePct float64 `json:"usage_pct"`
}

type MemoryStat struct {
	UsedPct    float64 `json:"used_pct"`
	UsedBytes  uint64  `json:"used_bytes"`
	TotalBytes uint64  `json:"total_bytes"`
}

type StorageStat struct {
	UsedPct    float64 `json:"used_pct"`
	UsedBytes  uint64  `json:"used_bytes"`
	TotalBytes uint64  `json:"total_bytes"`
	Mount      string  `json:"mount"`
}

type NetworkStat struct {
	ThroughputPct float64 `json:"throughput_pct"`
	RxBytesPerSec uint64  `json:"rx_bytes_per_sec"`
	TxBytesPerSec uint64  `json:"tx_bytes_per_sec"`
}

type hostSampler struct{}

func (c *Checker) HostMetrics() HostSnapshot {
	return HostSnapshot{Updated: time.Now().UTC()}
}
