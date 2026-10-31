//go:build windows

package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var windowsState struct {
	sync.Mutex
	prevIdle, prevKernel, prevUser uint64
	cpuReady                       bool
	prevNetworkIn                  uint64
	prevNetworkOut                 uint64
	prevNetworkAt                  time.Time
	networkReady                   bool
	cpuModel                       string
	cpuModelReady                  bool
	cpuCores                       uint32
	cpuCoresReady                  bool
}

var kernel32 = syscall.NewLazyDLL("kernel32.dll")

var (
	procGlobalMemory      = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetSystemTimes    = kernel32.NewProc("GetSystemTimes")
	procLogicalProcessors = kernel32.NewProc("GetLogicalProcessorInformation")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

type fileTime struct {
	LowDateTime  uint32
	HighDateTime uint32
}

func (t fileTime) value() uint64 {
	return uint64(t.HighDateTime)<<32 | uint64(t.LowDateTime)
}

type logicalProcessorInformation struct {
	ProcessorMask uintptr
	Relationship  uint32
	_             [16]byte
}

func collectHost(dataDirectory string) HostSample {
	var sample HostSample
	windowsState.Lock()
	defer windowsState.Unlock()
	var memory memoryStatusEx
	memory.Length = uint32(unsafe.Sizeof(memory))
	if ret, _, _ := procGlobalMemory.Call(uintptr(unsafe.Pointer(&memory))); ret != 0 {
		sample.MemoryTotal = memory.TotalPhys
		sample.MemoryUsed = memory.TotalPhys - memory.AvailPhys
	}
	var idle, kernel, user fileTime
	if ret, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	); ret != 0 {
		idleNow, kernelNow, userNow := idle.value(), kernel.value(), user.value()
		if windowsState.cpuReady {
			idleDelta := idleNow - windowsState.prevIdle
			totalDelta := (kernelNow - windowsState.prevKernel) + (userNow - windowsState.prevUser)
			if totalDelta > 0 {
				sample.CPUPercent = float64(totalDelta-idleDelta) / float64(totalDelta) * 100
			}
		}
		windowsState.prevIdle = idleNow
		windowsState.prevKernel = kernelNow
		windowsState.prevUser = userNow
		windowsState.cpuReady = true
	}
	sample.DiskUsed, sample.DiskTotal = windowsDiskUsage(dataDirectory)
	sample.NetworkInBPS, sample.NetworkOutBPS = windowsNetworkRates()
	if !windowsState.cpuModelReady {
		windowsState.cpuModel = windowsCPUModel()
		windowsState.cpuModelReady = true
	}
	if !windowsState.cpuCoresReady {
		windowsState.cpuCores = windowsPhysicalCores()
		windowsState.cpuCoresReady = true
	}
	sample.CPUModel = windowsState.cpuModel
	sample.CPUCores = windowsState.cpuCores
	return sample
}

func windowsNetworkRates() (uint64, uint64) {
	var table *windows.MibIfTable2
	if err := windows.GetIfTable2Ex(windows.MibIfTableNormal, &table); err != nil || table == nil {
		return 0, 0
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))
	rows := unsafe.Slice(&table.Table[0], table.NumEntries)
	var totalIn, totalOut uint64
	for _, row := range rows {
		if row.OperStatus != windows.IfOperStatusUp {
			continue
		}
		totalIn += row.InOctets
		totalOut += row.OutOctets
	}
	now := time.Now()
	var inRate, outRate uint64
	if windowsState.networkReady && now.After(windowsState.prevNetworkAt) {
		seconds := now.Sub(windowsState.prevNetworkAt).Seconds()
		if seconds > 0 {
			if totalIn >= windowsState.prevNetworkIn {
				inRate = uint64(float64(totalIn-windowsState.prevNetworkIn) / seconds)
			}
			if totalOut >= windowsState.prevNetworkOut {
				outRate = uint64(float64(totalOut-windowsState.prevNetworkOut) / seconds)
			}
		}
	}
	windowsState.prevNetworkIn = totalIn
	windowsState.prevNetworkOut = totalOut
	windowsState.prevNetworkAt = now
	windowsState.networkReady = true
	return inRate, outRate
}

func windowsCPUModel() string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	value, _, err := key.GetStringValue("ProcessorNameString")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func windowsPhysicalCores() uint32 {
	var length uint32
	procLogicalProcessors.Call(0, uintptr(unsafe.Pointer(&length)))
	if length == 0 {
		return uint32(runtime.NumCPU())
	}
	buffer := make([]byte, length)
	ret, _, _ := procLogicalProcessors.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&length)))
	if ret == 0 {
		return uint32(runtime.NumCPU())
	}
	size := unsafe.Sizeof(logicalProcessorInformation{})
	count := uint32(0)
	for offset := uintptr(0); offset+size <= uintptr(len(buffer)); offset += size {
		info := (*logicalProcessorInformation)(unsafe.Pointer(&buffer[offset]))
		if info.Relationship == 0 {
			count++
		}
	}
	if count == 0 {
		return uint32(runtime.NumCPU())
	}
	return count
}

func windowsDiskUsage(dataDirectory string) (uint64, uint64) {
	path := dataDirectory
	if path == "" {
		path, _ = os.Getwd()
	}
	if volume := filepath.VolumeName(path); volume != "" {
		path = volume + `\`
	} else {
		path = "C:\\"
	}
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0
	}
	var available, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(pointer, &available, &total, &free); err != nil {
		return 0, 0
	}
	return total - free, total
}
