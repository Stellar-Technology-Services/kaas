package ui

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// HostCapacity compares what the host has against what running sandboxes
// have been promised. Boat has plan caps; a laptop has RAM.
type HostCapacity struct {
	HostCPUs       int
	HostMemoryGiB  int
	AllocCPUs      int
	AllocMemoryGiB int
	RunningCount   int
	MemoryPercent  int  // allocated / host, 0 when host memory is unknown
	Warn           bool // allocation is already at or above the warning threshold
}

// capacityWarnPercent is the allocation share at which the UI warns before
// starting another VM. It is advisory only.
const capacityWarnPercent = 85

func hostCapacity(sandboxes []state.Sandbox) HostCapacity {
	cap := HostCapacity{HostCPUs: runtime.NumCPU(), HostMemoryGiB: hostMemoryGiB()}
	for _, sb := range sandboxes {
		if sb.ObservedState != state.ObservedRunning {
			continue
		}
		cap.RunningCount++
		cap.AllocCPUs += sb.CPUs
		cap.AllocMemoryGiB += sb.MemoryGiB
	}
	if cap.HostMemoryGiB > 0 {
		cap.MemoryPercent = cap.AllocMemoryGiB * 100 / cap.HostMemoryGiB
		cap.Warn = cap.MemoryPercent >= capacityWarnPercent
	}
	return cap
}

// wouldExceed reports whether adding memGiB pushes allocation past the threshold.
func (c HostCapacity) wouldExceed(memGiB int) bool {
	if c.HostMemoryGiB <= 0 {
		return false
	}
	return (c.AllocMemoryGiB+memGiB)*100/c.HostMemoryGiB >= capacityWarnPercent
}

// hostMemoryGiB returns total physical memory, or 0 when it cannot be read.
func hostMemoryGiB() int {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}
		bytes, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		if err != nil {
			return 0
		}
		return int(bytes / (1 << 30))
	case "linux":
		f, err := os.Open("/proc/meminfo")
		if err != nil {
			return 0
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) >= 2 && fields[0] == "MemTotal:" {
				kb, err := strconv.ParseInt(fields[1], 10, 64)
				if err != nil {
					return 0
				}
				return int(kb / (1 << 20))
			}
		}
	}
	return 0
}
