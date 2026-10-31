package agent

type HostSample struct {
	CPUPercent    float64
	MemoryUsed    uint64
	MemoryTotal   uint64
	DiskUsed      uint64
	DiskTotal     uint64
	NetworkInBPS  uint64
	NetworkOutBPS uint64
	CPUModel      string
	CPUCores      uint32
}

func CollectHost(dataDirectory string) HostSample {
	return collectHost(dataDirectory)
}
