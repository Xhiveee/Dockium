package db

import (
	"encoding/json"
	"time"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

const (
	StatusStopped  = "stopped"
	StatusStarting = "starting"
	StatusRunning  = "running"
	StatusStopping = "stopping"
	StatusExited   = "exited"
	StatusFailed   = "failed"
)

type User struct {
	ID           int64
	Login        string
	PasswordHash string
	Role         string
	CreatedAt    int64
	LastLoginAt  int64
}

type Session struct {
	ID         string
	UserID     int64
	CreatedAt  int64
	LastSeenAt int64
	ExpiresAt  int64
	IP         string
	UserAgent  string
}

type LoginLog struct {
	ID        int64
	Login     string
	Success   bool
	Reason    string
	IP        string
	UserAgent string
	Browser   string
	OS        string
	CreatedAt int64
}

type Ban struct {
	ID        int64
	IP        string
	Reason    string
	Source    string
	BannedAt  int64
	ExpiresAt int64
}

type ApiRequest struct {
	ID        int64
	Login     string
	Procedure string
	Status    int
	IP        string
	CreatedAt int64
}

type Agent struct {
	ID            int64
	Name          string
	Country       string
	TokenHash     string
	Version       string
	Hostname      string
	Platform      string
	Arch          string
	DockerVersion string
	DockerOK      bool
	DockerError   string
	CPUModel      string
	CPUCores      uint32
	CPUPercent    float64
	MemUsed       uint64
	MemTotal      uint64
	DiskUsed      uint64
	DiskTotal     uint64
	NetInBPS      uint64
	NetOutBPS     uint64
	LatencyMS     uint32
	Online        bool
	LastSeenAt    int64
	ConnectedAt   int64
	CreatedAt     int64
}

type EnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type PortMapping struct {
	HostPort      uint32 `json:"host_port"`
	ContainerPort uint32 `json:"container_port"`
	Protocol      string `json:"protocol"`
}

type VolumeMount struct {
	HostPath      string `json:"host_path"`
	ContainerPath string `json:"container_path"`
	ReadOnly      bool   `json:"read_only"`
}

type Instance struct {
	ID            int64
	Name          string
	AgentID       int64
	AgentName     string
	AgentOnline   bool
	Status        string
	Image         string
	ContainerName string
	ContainerID   string
	Command       []string
	Environment   []EnvVar
	Ports         []PortMapping
	Volumes       []VolumeMount
	NetworkMode   string
	RestartPolicy string
	MemoryMB      uint32
	CPUs          float64
	TTY           bool
	DataMount     string
	Encoding      string
	AutoStart     bool
	AutoRestart   bool
	LastError     string
	ExitCode      int32
	LastStartedAt int64
	CPUPercent    float64
	MemBytes      uint64
	MemLimitBytes uint64
	NetInBPS      uint64
	NetOutBPS     uint64
	CreatedAt     int64
	UpdatedAt     int64
}

type Permission struct {
	Control bool
	Console bool
	Files   bool
}

type Task struct {
	ID              int64
	InstanceID      int64
	InstanceName    string
	Name            string
	Action          string
	Payload         string
	Kind            string
	IntervalMinutes uint32
	TimeOfDay       string
	Enabled         bool
	LastRunAt       int64
	NextRunAt       int64
	LastError       string
	CreatedAt       int64
}

func decodeJSON[T any](input []byte, output *T) error {
	if len(input) == 0 {
		var zero T
		*output = zero
		return nil
	}
	return json.Unmarshal(input, output)
}

func encodeJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		return []byte("[]")
	}
	return data
}

func Now() int64 {
	return time.Now().UTC().UnixMilli()
}
