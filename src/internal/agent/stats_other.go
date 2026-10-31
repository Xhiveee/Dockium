//go:build !linux && !windows

package agent

import (
	"runtime"
	"syscall"
)

func collectHost(dataDirectory string) HostSample {
	var sample HostSample
	sample.CPUCores = uint32(runtime.NumCPU())
	if dataDirectory != "" {
		var info syscall.Statfs_t
		if err := syscall.Statfs(dataDirectory, &info); err == nil {
			sample.DiskTotal = info.Blocks * uint64(info.Bsize)
			sample.DiskUsed = (info.Blocks - info.Bavail) * uint64(info.Bsize)
		}
	}
	return sample
}
