package agent

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	agentv1 "dockium/gen/agent/v1"
)

type Docker struct {
	binary string
}

func NewDocker() *Docker {
	binary := "docker"
	if value := os.Getenv("DOCKIUM_DOCKER_BIN"); value != "" {
		binary = value
	}
	return &Docker{binary: binary}
}

func (d *Docker) run(ctx context.Context, args ...string) ([]byte, error) {
	if _, err := exec.LookPath(d.binary); err != nil {
		return nil, errors.New("Docker CLI не найден на агенте")
	}
	command := exec.CommandContext(ctx, d.binary, args...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(string(output))
		}
		if message == "" {
			message = err.Error()
		}
		if isDockerUnavailable(message) {
			return nil, errors.New("Docker недоступен: " + shorten(message, 300))
		}
		if ctx.Err() != nil {
			return nil, errors.New("Docker не ответил вовремя")
		}
		return nil, errors.New(shorten(message, 500))
	}
	return output, nil
}

func isDockerUnavailable(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "cannot connect") ||
		strings.Contains(lower, "failed to connect") ||
		strings.Contains(lower, "docker daemon is not running") ||
		strings.Contains(lower, "is the docker daemon running") ||
		strings.Contains(lower, "open //./pipe/docker") ||
		strings.Contains(lower, "permission denied while trying to connect")
}

func (d *Docker) Version(ctx context.Context) (string, error) {
	output, err := d.run(ctx, "version", "--format", "{{.Server.Version}}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (d *Docker) ListImages(ctx context.Context) ([]*agentv1.Image, error) {
	output, err := d.run(ctx, "image", "ls", "--format", "{{.ID}}|{{.Repository}}|{{.Tag}}|{{.Size}}|{{.CreatedAt}}")
	if err != nil {
		return nil, err
	}
	items := make([]*agentv1.Image, 0)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "|")
		if len(parts) < 5 {
			continue
		}
		items = append(items, &agentv1.Image{
			Id:         parts[0],
			Repository: parts[1],
			Tag:        parts[2],
			SizeBytes:  parseHumanSize(parts[3]),
			CreatedAt:  parseDockerTime(parts[4]),
		})
	}
	return items, scanner.Err()
}

type containerRow struct {
	id        string
	name      string
	image     string
	command   string
	status    string
	state     string
	createdAt int64
	labels    map[string]string
}

func (d *Docker) listContainers(ctx context.Context) ([]containerRow, error) {
	output, err := d.run(ctx, "ps", "-a", "--no-trunc", "--format", "{{.ID}}|{{.Names}}|{{.Image}}|{{.Command}}|{{.Status}}|{{.State}}|{{.CreatedAt}}|{{.Labels}}")
	if err != nil {
		return nil, err
	}
	rows := make([]containerRow, 0)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "|", 8)
		if len(parts) < 8 {
			continue
		}
		rows = append(rows, containerRow{
			id:        parts[0],
			name:      parts[1],
			image:     parts[2],
			command:   parts[3],
			status:    parts[4],
			state:     parts[5],
			createdAt: parseDockerTime(parts[6]),
			labels:    parseLabels(parts[7]),
		})
	}
	return rows, scanner.Err()
}

func (d *Docker) ListContainers(ctx context.Context) ([]*agentv1.Container, error) {
	rows, err := d.listContainers(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*agentv1.Container, 0, len(rows))
	for _, row := range rows {
		instanceID, _ := strconv.ParseInt(row.labels["dockium.instance"], 10, 64)
		items = append(items, &agentv1.Container{
			Id:         row.id,
			Name:       row.name,
			Image:      row.image,
			Command:    row.command,
			Status:     row.status,
			State:      row.state,
			CreatedAt:  row.createdAt,
			Managed:    row.labels["dockium.managed"] == "true",
			InstanceId: instanceID,
		})
	}
	return items, nil
}

func (d *Docker) Inspect(ctx context.Context, kind, id string) (string, error) {
	var object string
	switch kind {
	case "image":
		object = "image"
	case "container":
		object = "container"
	default:
		return "", errors.New("неизвестный тип объекта Docker")
	}
	output, err := d.run(ctx, object, "inspect", id)
	if err != nil {
		return "", err
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, output, "", "  "); err != nil {
		return string(output), nil
	}
	return formatted.String(), nil
}

func (d *Docker) RemoveImage(ctx context.Context, image string) error {
	_, err := d.run(ctx, "image", "rm", image)
	return err
}

func (d *Docker) ImageExists(ctx context.Context, image string) bool {
	_, err := d.run(ctx, "image", "inspect", image)
	return err == nil
}

func (d *Docker) Pull(ctx context.Context, image string, onLine func(string)) error {
	if _, err := exec.LookPath(d.binary); err != nil {
		return errors.New("Docker CLI не найден на агенте")
	}
	command := exec.CommandContext(ctx, d.binary, "pull", image)
	return streamCommand(command, onLine)
}

func (d *Docker) Build(ctx context.Context, tag, dockerfile string, onLine func(string)) error {
	if _, err := exec.LookPath(d.binary); err != nil {
		return errors.New("Docker CLI не найден на агенте")
	}
	directory, err := os.MkdirTemp("", "dockium-build-*")
	if err != nil {
		return fmt.Errorf("не удалось создать контекст сборки: %w", err)
	}
	defer os.RemoveAll(directory)
	if err := os.WriteFile(filepath.Join(directory, "Dockerfile"), []byte(dockerfile), 0o600); err != nil {
		return fmt.Errorf("не удалось записать Dockerfile: %w", err)
	}
	command := exec.CommandContext(ctx, d.binary, "build", "--progress=plain", "-t", tag, directory)
	return streamCommand(command, onLine)
}

func streamCommand(command *exec.Cmd, onLine func(string)) error {
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	var mutex sync.Mutex
	scan := func(reader io.Reader) {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			mutex.Lock()
			onLine(scanner.Text())
			mutex.Unlock()
		}
	}
	done := make(chan struct{}, 2)
	go func() { scan(stdout); done <- struct{}{} }()
	go func() { scan(stderr); done <- struct{}{} }()
	<-done
	<-done
	if err := command.Wait(); err != nil {
		return fmt.Errorf("команда завершилась с ошибкой: %w", err)
	}
	return nil
}

type containerState struct {
	ID         string
	Status     string
	ExitCode   int32
	Error      string
	StartedAt  int64
	FinishedAt int64
}

func (d *Docker) ContainerStates(ctx context.Context, names []string) (map[string]containerState, error) {
	result := make(map[string]containerState)
	if len(names) == 0 {
		return result, nil
	}
	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[name] = true
	}
	rows, err := d.listContainers(ctx)
	if err != nil {
		return result, err
	}
	existing := make([]string, 0, len(names))
	for _, row := range rows {
		if !wanted[row.name] {
			continue
		}
		result[row.name] = containerState{ID: row.id, Status: row.state}
		existing = append(existing, row.name)
	}
	if len(existing) == 0 {
		return result, nil
	}
	args := []string{"inspect", "--type", "container", "--format",
		`{{.Name}}|{{.State.Status}}|{{.State.ExitCode}}|{{.State.StartedAt}}|{{.State.FinishedAt}}|{{.State.Error}}|{{.Id}}`}
	args = append(args, existing...)
	output, err := d.run(ctx, args...)
	if err != nil {
		return result, nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "|")
		if len(parts) < 7 {
			continue
		}
		name := strings.TrimPrefix(parts[0], "/")
		exitCode, _ := strconv.ParseInt(parts[2], 10, 32)
		result[name] = containerState{
			ID:         parts[6],
			Status:     parts[1],
			ExitCode:   int32(exitCode),
			StartedAt:  parseRFC3339Millis(parts[3]),
			FinishedAt: parseRFC3339Millis(parts[4]),
			Error:      parts[5],
		}
	}
	return result, nil
}

type containerStats struct {
	CPUPercent float64
	MemUsage   uint64
	MemLimit   uint64
	NetIn      uint64
	NetOut     uint64
}

func (d *Docker) Stats(ctx context.Context) (map[string]containerStats, error) {
	output, err := d.run(ctx, "stats", "--no-stream", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	result := make(map[string]containerStats)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var raw struct {
			Name     string `json:"Name"`
			CPUPerc  string `json:"CPUPerc"`
			MemUsage string `json:"MemUsage"`
			NetIO    string `json:"NetIO"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			continue
		}
		stats := containerStats{}
		stats.CPUPercent = parsePercent(raw.CPUPerc)
		memParts := strings.Split(raw.MemUsage, "/")
		if len(memParts) == 2 {
			stats.MemUsage = parseHumanSizeBytes(strings.TrimSpace(memParts[0]))
			stats.MemLimit = parseHumanSizeBytes(strings.TrimSpace(memParts[1]))
		}
		netParts := strings.Split(raw.NetIO, "/")
		if len(netParts) == 2 {
			stats.NetIn = parseHumanSizeBytes(strings.TrimSpace(netParts[0]))
			stats.NetOut = parseHumanSizeBytes(strings.TrimSpace(netParts[1]))
		}
		result[raw.Name] = stats
	}
	return result, scanner.Err()
}

func (d *Docker) ContainerExists(ctx context.Context, name string) bool {
	_, err := d.run(ctx, "inspect", "--type", "container", name)
	return err == nil
}

func (d *Docker) ContainerLabel(ctx context.Context, name, label string) string {
	output, err := d.run(ctx, "inspect", "--type", "container", "--format", `{{index .Config.Labels "`+label+`"}}`, name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (d *Docker) ContainerNameList(ctx context.Context) ([]string, error) {
	output, err := d.run(ctx, "ps", "-a", "--format", "{{.Names}}")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(string(output), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

func (d *Docker) Create(ctx context.Context, spec *agentv1.InstanceSpec, dataDirectory, hash string) error {
	args := []string{"create", "--name", spec.ContainerName,
		"--label", "dockium.managed=true",
		"--label", "dockium.instance=" + strconv.FormatInt(spec.Id, 10),
		"--label", "dockium.spec=" + hash,
	}
	policy := spec.RestartPolicy
	if policy == "" {
		policy = "no"
	}
	args = append(args, "--restart", policy)
	network := spec.NetworkMode
	if network == "" {
		network = "bridge"
	}
	args = append(args, "--network", network)
	args = append(args, "-i")
	if spec.Tty {
		args = append(args, "-t")
	}
	for _, item := range spec.Environment {
		args = append(args, "-e", item.Key+"="+item.Value)
	}
	for _, port := range spec.Ports {
		if port.HostPort == 0 || port.ContainerPort == 0 {
			continue
		}
		protocol := port.Protocol
		if protocol == "" {
			protocol = "tcp"
		}
		args = append(args, "-p", fmt.Sprintf("%d:%d/%s", port.HostPort, port.ContainerPort, protocol))
	}
	containerData := spec.DataMount
	if containerData == "" {
		containerData = "/data"
	}
	args = append(args, "-v", dataDirectory+":"+containerData)
	for _, volume := range spec.Volumes {
		if volume.HostPath == "" || volume.ContainerPath == "" {
			continue
		}
		mount := volume.HostPath + ":" + volume.ContainerPath
		if volume.ReadOnly {
			mount += ":ro"
		}
		args = append(args, "-v", mount)
	}
	if spec.MemoryMb > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", spec.MemoryMb))
	}
	if spec.Cpus > 0 {
		args = append(args, "--cpus", strconv.FormatFloat(spec.Cpus, 'f', -1, 64))
	}
	args = append(args, spec.Image)
	args = append(args, spec.Command...)
	_, err := d.run(ctx, args...)
	return err
}

func (d *Docker) Start(ctx context.Context, name string) error {
	_, err := d.run(ctx, "start", name)
	return err
}

func (d *Docker) Stop(ctx context.Context, name string, timeout time.Duration, force bool) error {
	if force {
		_, err := d.run(ctx, "kill", name)
		return err
	}
	seconds := int(timeout.Seconds())
	if seconds <= 0 {
		seconds = 10
	}
	_, err := d.run(ctx, "stop", "-t", strconv.Itoa(seconds), name)
	return err
}

func (d *Docker) Restart(ctx context.Context, name string, timeout time.Duration) error {
	seconds := int(timeout.Seconds())
	if seconds <= 0 {
		seconds = 10
	}
	_, err := d.run(ctx, "restart", "-t", strconv.Itoa(seconds), name)
	return err
}

func (d *Docker) Remove(ctx context.Context, name string) error {
	_, err := d.run(ctx, "rm", "-f", name)
	return err
}

func (d *Docker) LogsCommand(ctx context.Context, name string, tail uint32) (*exec.Cmd, io.ReadCloser, error) {
	if _, err := exec.LookPath(d.binary); err != nil {
		return nil, nil, errors.New("Docker CLI не найден на агенте")
	}
	if tail == 0 {
		tail = 200
	}
	if tail > 5000 {
		tail = 5000
	}
	command := exec.CommandContext(ctx, d.binary, "logs", "--follow", "--tail", strconv.FormatUint(uint64(tail), 10), "--timestamps=false", name)
	output, err := command.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	command.Stderr = command.Stdout
	if err := command.Start(); err != nil {
		return nil, nil, err
	}
	return command, output, nil
}

func specHash(spec *agentv1.InstanceSpec) string {
	type canonical struct {
		Image         string      `json:"image"`
		Container     string      `json:"container"`
		Command       []string    `json:"command"`
		Environment   [][2]string `json:"environment"`
		Ports         [][3]string `json:"ports"`
		Volumes       [][3]string `json:"volumes"`
		Network       string      `json:"network"`
		RestartPolicy string      `json:"restart_policy"`
		MemoryMB      uint32      `json:"memory_mb"`
		CPUs          float64     `json:"cpus"`
		TTY           bool        `json:"tty"`
		DataMount     string      `json:"data_mount"`
	}
	value := canonical{
		Image:         spec.Image,
		Container:     spec.ContainerName,
		Command:       append([]string{}, spec.Command...),
		Network:       spec.NetworkMode,
		RestartPolicy: spec.RestartPolicy,
		MemoryMB:      spec.MemoryMb,
		CPUs:          spec.Cpus,
		TTY:           spec.Tty,
		DataMount:     spec.DataMount,
	}
	for _, item := range spec.Environment {
		value.Environment = append(value.Environment, [2]string{item.Key, item.Value})
	}
	sort.Slice(value.Environment, func(left, right int) bool {
		return value.Environment[left][0] < value.Environment[right][0]
	})
	for _, port := range spec.Ports {
		value.Ports = append(value.Ports, [3]string{strconv.Itoa(int(port.HostPort)), strconv.Itoa(int(port.ContainerPort)), port.Protocol})
	}
	for _, volume := range spec.Volumes {
		value.Volumes = append(value.Volumes, [3]string{volume.HostPath, volume.ContainerPath, strconv.FormatBool(volume.ReadOnly)})
	}
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:12])
}

func parseLabels(value string) map[string]string {
	labels := make(map[string]string)
	for _, part := range strings.Split(value, ",") {
		key, labelValue, found := strings.Cut(part, "=")
		if !found {
			continue
		}
		labels[strings.TrimSpace(key)] = strings.TrimSpace(labelValue)
	}
	return labels
}

func shorten(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}
