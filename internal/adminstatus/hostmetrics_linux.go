//go:build linux

package adminstatus

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

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

type hostSampler struct {
	mu        sync.Mutex
	procRoot  string
	prevCPU   map[string]cpuSample
	prevNet   netSample
	prevNetAt time.Time
}

type cpuSample struct {
	total uint64
	idle  uint64
}

type netSample struct {
	rx uint64
	tx uint64
}

func newHostSampler() *hostSampler {
	root := strings.TrimSpace(os.Getenv("VSRV_PROC_ROOT"))
	if root == "" {
		root = "/proc"
	}
	return &hostSampler{
		procRoot: root,
		prevCPU:  make(map[string]cpuSample),
	}
}

func (c *Checker) HostMetrics() HostSnapshot {
	if c.host == nil {
		c.host = newHostSampler()
	}
	return c.host.snapshot()
}

func (s *hostSampler) snapshot() HostSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	out := HostSnapshot{
		CPUs:    s.readCPUs(),
		Memory:  s.readMemory(),
		Storage: s.readStorage(),
		Network: s.readNetwork(now),
		Updated: now,
	}
	if len(out.CPUs) == 0 {
		n := runtime.NumCPU()
		if n <= 0 {
			n = 1
		}
		load := s.readLoadAvg()
		pct := load / float64(n) * 100
		if pct > 100 {
			pct = 100
		}
		for i := 0; i < n; i++ {
			out.CPUs = append(out.CPUs, CPUStat{
				ID:       fmt.Sprintf("CPU%d", i),
				UsagePct: pct,
			})
		}
	}
	return out
}

func (s *hostSampler) procPath(parts ...string) string {
	return filepath.Join(append([]string{s.procRoot}, parts...)...)
}

func (s *hostSampler) readCPUs() []CPUStat {
	f, err := os.Open(s.procPath("stat"))
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []CPUStat
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "cpu ") {
			continue
		}
		if !strings.HasPrefix(line, "cpu") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		id := fields[0]
		var vals []uint64
		for _, part := range fields[1:] {
			n, err := strconv.ParseUint(part, 10, 64)
			if err != nil {
				vals = nil
				break
			}
			vals = append(vals, n)
		}
		if len(vals) < 4 {
			continue
		}
		var total, idle uint64
		for _, v := range vals {
			total += v
		}
		idle = vals[3]
		if len(vals) > 4 {
			idle += vals[4]
		}
		cur := cpuSample{total: total, idle: idle}
		pct := 0.0
		if prev, ok := s.prevCPU[id]; ok && total > prev.total {
			dTotal := float64(total - prev.total)
			dIdle := float64(idle - prev.idle)
			pct = (1 - dIdle/dTotal) * 100
			if pct < 0 {
				pct = 0
			}
			if pct > 100 {
				pct = 100
			}
		}
		s.prevCPU[id] = cur
		out = append(out, CPUStat{ID: strings.ToUpper(id), UsagePct: pct})
	}
	return out
}

func (s *hostSampler) readLoadAvg() float64 {
	b, err := os.ReadFile(s.procPath("loadavg"))
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(fields[0], 64)
	return v
}

func (s *hostSampler) readMemory() MemoryStat {
	f, err := os.Open(s.procPath("meminfo"))
	if err != nil {
		return MemoryStat{}
	}
	defer f.Close()

	var total, available uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total = parseKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			available = parseKB(line)
		}
	}
	if total == 0 {
		return MemoryStat{}
	}
	used := total - available
	pct := float64(used) / float64(total) * 100
	return MemoryStat{
		UsedPct:    pct,
		UsedBytes:  used * 1024,
		TotalBytes: total * 1024,
	}
}

func parseKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	n, _ := strconv.ParseUint(fields[1], 10, 64)
	return n
}

func (s *hostSampler) readStorage() StorageStat {
	mount := os.Getenv("VSRV_STORAGE_PATH")
	if mount == "" {
		mount = "/"
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(mount, &st); err != nil {
		return StorageStat{Mount: mount}
	}
	total := st.Blocks * uint64(st.Bsize)
	free := st.Bfree * uint64(st.Bsize)
	if total == 0 {
		return StorageStat{Mount: mount}
	}
	used := total - free
	return StorageStat{
		UsedPct:    float64(used) / float64(total) * 100,
		UsedBytes:  used,
		TotalBytes: total,
		Mount:      mount,
	}
}

func (s *hostSampler) readNetwork(now time.Time) NetworkStat {
	f, err := os.Open(s.procPath("net", "dev"))
	if err != nil {
		return NetworkStat{}
	}
	defer f.Close()

	var rx, tx uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.Contains(line, "|") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 10 {
			continue
		}
		iface := strings.TrimSuffix(parts[0], ":")
		if iface == "lo" {
			continue
		}
		r, _ := strconv.ParseUint(parts[1], 10, 64)
		t, _ := strconv.ParseUint(parts[9], 10, 64)
		rx += r
		tx += t
	}

	out := NetworkStat{}
	if !s.prevNetAt.IsZero() {
		secs := now.Sub(s.prevNetAt).Seconds()
		if secs > 0 {
			if rx >= s.prevNet.rx {
				out.RxBytesPerSec = uint64(float64(rx-s.prevNet.rx) / secs)
			}
			if tx >= s.prevNet.tx {
				out.TxBytesPerSec = uint64(float64(tx-s.prevNet.tx) / secs)
			}
		}
	}
	s.prevNet = netSample{rx: rx, tx: tx}
	s.prevNetAt = now

	// Map throughput to gauge: 100 Mbps reference (~12.5 MB/s combined).
	const refBytesPerSec = 12_500_000.0
	combined := float64(out.RxBytesPerSec + out.TxBytesPerSec)
	out.ThroughputPct = combined / refBytesPerSec * 100
	if out.ThroughputPct > 100 {
		out.ThroughputPct = 100
	}
	return out
}
