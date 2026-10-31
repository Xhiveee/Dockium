//go:build linux

package agent

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

var hostState struct {
	sync.Mutex
	cpuTotal   uint64
	cpuIdle    uint64
	cpuReady   bool
	netIn      uint64
	netOut     uint64
	netAt      time.Time
	netReady   bool
	cpuModel   string
	cpuCores   uint32
	modelReady bool
}

func collectHost(dataDirectory string) HostSample {
	var sample HostSample
	hostState.Lock()
	defer hostState.Unlock()

	if data, err := os.ReadFile("/proc/stat"); err == nil {
		lines := strings.Split(string(data), "\n")
		if len(lines) > 0 {
			fields := strings.Fields(lines[0])
			if len(fields) >= 8 {
				var values []uint64
				for _, field := range fields[1:] {
					value, _ := strconv.ParseUint(field, 10, 64)
					values = append(values, value)
				}
				total := uint64(0)
				for _, value := range values {
					total += value
				}
				idle := values[3]
				if len(values) > 4 {
					idle += values[4]
				}
				if hostState.cpuReady && total > hostState.cpuTotal {
					totalDelta := total - hostState.cpuTotal
					idleDelta := idle - hostState.cpuIdle
					if totalDelta > 0 {
						sample.CPUPercent = float64(totalDelta-idleDelta) / float64(totalDelta) * 100
					}
				}
				hostState.cpuTotal = total
				hostState.cpuIdle = idle
				hostState.cpuReady = true
			}
		}
	}
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		var total, available uint64
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			value, _ := strconv.ParseUint(fields[1], 10, 64)
			switch fields[0] {
			case "MemTotal:":
				total = value * 1024
			case "MemAvailable:":
				available = value * 1024
			}
		}
		if total > 0 {
			sample.MemoryTotal = total
			sample.MemoryUsed = total - min(total, available)
		}
	}
	if data, err := os.ReadFile("/proc/net/dev"); err == nil {
		var totalIn, totalOut uint64
		for _, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(line, ":") {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if strings.TrimSpace(parts[0]) == "lo" {
				continue
			}
			fields := strings.Fields(parts[1])
			if len(fields) < 9 {
				continue
			}
			rx, _ := strconv.ParseUint(fields[0], 10, 64)
			tx, _ := strconv.ParseUint(fields[8], 10, 64)
			totalIn += rx
			totalOut += tx
		}
		now := time.Now()
		if hostState.netReady && now.After(hostState.netAt) {
			seconds := now.Sub(hostState.netAt).Seconds()
			if seconds > 0 {
				if totalIn >= hostState.netIn {
					sample.NetworkInBPS = uint64(float64(totalIn-hostState.netIn) / seconds)
				}
				if totalOut >= hostState.netOut {
					sample.NetworkOutBPS = uint64(float64(totalOut-hostState.netOut) / seconds)
				}
			}
		}
		hostState.netIn = totalIn
		hostState.netOut = totalOut
		hostState.netAt = now
		hostState.netReady = true
	}
	if dataDirectory != "" {
		var stat unix.Statfs_t
		if err := unix.Statfs(dataDirectory, &stat); err == nil {
			total := stat.Blocks * uint64(stat.Bsize)
			available := stat.Bavail * uint64(stat.Bsize)
			sample.DiskTotal = total
			if total > available {
				sample.DiskUsed = total - available
			}
		}
	}
	if !hostState.modelReady {
		hostState.cpuCores = uint32(runtime.NumCPU())
		if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "model name") {
					if _, value, found := strings.Cut(line, ":"); found {
						hostState.cpuModel = strings.TrimSpace(value)
					}
					break
				}
			}
		}
		hostState.modelReady = true
	}
	sample.CPUModel = hostState.cpuModel
	sample.CPUCores = hostState.cpuCores
	return sample
}
