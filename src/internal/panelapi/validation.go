package panelapi

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	panelv1 "dockium/gen/panel/v1"
	"dockium/internal/db"
)

var (
	containerNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)
	envKeyPattern        = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	networkPattern       = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)
	uploadIDPattern      = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,80}$`)
)

var allowedEncodings = map[string]bool{
	"utf-8":        true,
	"windows-1251": true,
	"cp866":        true,
	"koi8-r":       true,
}

var allowedRestartPolicies = map[string]bool{
	"no":             true,
	"always":         true,
	"unless-stopped": true,
	"on-failure":     true,
}

func slugContainerName(name string) string {
	lower := strings.ToLower(name)
	var builder strings.Builder
	lastDash := false
	for _, symbol := range lower {
		if (symbol >= 'a' && symbol <= 'z') || (symbol >= '0' && symbol <= '9') {
			builder.WriteRune(symbol)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		slug = "instance"
	}
	if len(slug) > 32 {
		slug = slug[:32]
	}
	sum := sha1.Sum([]byte(name))
	return fmt.Sprintf("dockium-%s-%s", slug, hex.EncodeToString(sum[:2]))
}

func validateInstanceInput(name, image, containerName, network, restartPolicy, dataMount, encoding string,
	command []string, environment []*panelv1.EnvVar, ports []*panelv1.PortMapping,
	volumes []*panelv1.VolumeMount, memoryMB uint32, cpus float64) error {
	name = strings.TrimSpace(name)
	if len(name) < 2 || len(name) > 48 {
		return fmt.Errorf("название экземпляра: от 2 до 48 символов")
	}
	if strings.TrimSpace(image) == "" {
		return fmt.Errorf("укажите Docker-образ")
	}
	if len(image) > 255 {
		return fmt.Errorf("слишком длинное имя образа")
	}
	if containerName != "" && !containerNamePattern.MatchString(containerName) {
		return fmt.Errorf("имя контейнера: латиница, цифры, дефис, точка и подчёркивание")
	}
	if network != "" && !networkPattern.MatchString(network) {
		return fmt.Errorf("некорректное имя Docker-сети")
	}
	if restartPolicy != "" && !allowedRestartPolicies[restartPolicy] {
		return fmt.Errorf("некорректная политика перезапуска")
	}
	if dataMount != "" && !strings.HasPrefix(dataMount, "/") {
		return fmt.Errorf("путь данных в контейнере должен начинаться с /")
	}
	if encoding != "" && !allowedEncodings[encoding] {
		return fmt.Errorf("неподдерживаемая кодировка консоли")
	}
	if len(command) > 64 {
		return fmt.Errorf("слишком много аргументов команды")
	}
	for _, argument := range command {
		if len(argument) > 2048 {
			return fmt.Errorf("слишком длинный аргумент команды")
		}
	}
	if len(environment) > 128 {
		return fmt.Errorf("слишком много переменных окружения")
	}
	seenEnv := make(map[string]bool)
	for _, item := range environment {
		if !envKeyPattern.MatchString(item.Key) {
			return fmt.Errorf("некорректное имя переменной окружения: %s", item.Key)
		}
		if seenEnv[item.Key] {
			return fmt.Errorf("переменная окружения %s указана дважды", item.Key)
		}
		seenEnv[item.Key] = true
		if len(item.Value) > 4096 {
			return fmt.Errorf("слишком длинное значение переменной %s", item.Key)
		}
	}
	if len(ports) > 64 {
		return fmt.Errorf("слишком много портов")
	}
	seenPorts := make(map[string]bool)
	for _, port := range ports {
		if port.HostPort == 0 || port.HostPort > 65535 || port.ContainerPort == 0 || port.ContainerPort > 65535 {
			return fmt.Errorf("порты должны быть в диапазоне 1–65535")
		}
		protocol := strings.ToLower(port.Protocol)
		if protocol == "" {
			protocol = "tcp"
		}
		if protocol != "tcp" && protocol != "udp" {
			return fmt.Errorf("протокол порта: tcp или udp")
		}
		key := fmt.Sprintf("%d/%s", port.HostPort, protocol)
		if seenPorts[key] {
			return fmt.Errorf("порт %d/%s указан дважды", port.HostPort, protocol)
		}
		seenPorts[key] = true
	}
	if len(volumes) > 16 {
		return fmt.Errorf("слишком много дополнительных томов")
	}
	for _, volume := range volumes {
		if !filepath.IsAbs(volume.HostPath) {
			return fmt.Errorf("путь на агенте должен быть абсолютным: %s", volume.HostPath)
		}
		if !strings.HasPrefix(volume.ContainerPath, "/") {
			return fmt.Errorf("путь в контейнере должен начинаться с /")
		}
	}
	if memoryMB > 1_048_576 {
		return fmt.Errorf("слишком большой лимит памяти")
	}
	if cpus < 0 || cpus > 128 {
		return fmt.Errorf("некорректный лимит CPU")
	}
	return nil
}

func instanceFromCreate(request *panelv1.CreateInstanceRequest) *db.Instance {
	containerName := strings.TrimSpace(request.ContainerName)
	if containerName == "" {
		containerName = slugContainerName(request.Name)
	}
	network := request.NetworkMode
	if network == "" {
		network = "bridge"
	}
	restartPolicy := request.RestartPolicy
	if restartPolicy == "" {
		restartPolicy = "no"
	}
	dataMount := request.DataMount
	if dataMount == "" {
		dataMount = "/data"
	}
	encoding := request.ConsoleEncoding
	if encoding == "" {
		encoding = "utf-8"
	}
	return &db.Instance{
		Name:          strings.TrimSpace(request.Name),
		AgentID:       request.AgentId,
		Status:        db.StatusStopped,
		Image:         strings.TrimSpace(request.Image),
		ContainerName: containerName,
		Command:       cleanStrings(request.Command),
		Environment:   envFromProto(request.Environment),
		Ports:         portsFromProto(request.Ports),
		Volumes:       volumesFromProto(request.Volumes),
		NetworkMode:   network,
		RestartPolicy: restartPolicy,
		MemoryMB:      request.MemoryMb,
		CPUs:          request.Cpus,
		TTY:           request.Tty,
		DataMount:     dataMount,
		Encoding:      encoding,
		AutoStart:     request.AutoStart,
		AutoRestart:   request.AutoRestart,
	}
}

func applyUpdate(instance *db.Instance, request *panelv1.UpdateInstanceRequest) {
	instance.Name = strings.TrimSpace(request.Name)
	instance.Image = strings.TrimSpace(request.Image)
	if name := strings.TrimSpace(request.ContainerName); name != "" {
		instance.ContainerName = name
	}
	instance.Command = cleanStrings(request.Command)
	instance.Environment = envFromProto(request.Environment)
	instance.Ports = portsFromProto(request.Ports)
	instance.Volumes = volumesFromProto(request.Volumes)
	instance.NetworkMode = request.NetworkMode
	if instance.NetworkMode == "" {
		instance.NetworkMode = "bridge"
	}
	instance.RestartPolicy = request.RestartPolicy
	if instance.RestartPolicy == "" {
		instance.RestartPolicy = "no"
	}
	instance.MemoryMB = request.MemoryMb
	instance.CPUs = request.Cpus
	instance.TTY = request.Tty
	instance.DataMount = request.DataMount
	if instance.DataMount == "" {
		instance.DataMount = "/data"
	}
	instance.Encoding = request.ConsoleEncoding
	if instance.Encoding == "" {
		instance.Encoding = "utf-8"
	}
	instance.AutoStart = request.AutoStart
	instance.AutoRestart = request.AutoRestart
}

func envFromProto(items []*panelv1.EnvVar) []db.EnvVar {
	result := make([]db.EnvVar, 0, len(items))
	for _, item := range items {
		result = append(result, db.EnvVar{Key: strings.TrimSpace(item.Key), Value: item.Value})
	}
	return result
}

func portsFromProto(items []*panelv1.PortMapping) []db.PortMapping {
	result := make([]db.PortMapping, 0, len(items))
	for _, item := range items {
		protocol := strings.ToLower(strings.TrimSpace(item.Protocol))
		if protocol == "" {
			protocol = "tcp"
		}
		result = append(result, db.PortMapping{HostPort: item.HostPort, ContainerPort: item.ContainerPort, Protocol: protocol})
	}
	return result
}

func volumesFromProto(items []*panelv1.VolumeMount) []db.VolumeMount {
	result := make([]db.VolumeMount, 0, len(items))
	for _, item := range items {
		result = append(result, db.VolumeMount{
			HostPath:      strings.TrimSpace(item.HostPath),
			ContainerPath: strings.TrimSpace(item.ContainerPath),
			ReadOnly:      item.ReadOnly,
		})
	}
	return result
}

func cleanStrings(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
