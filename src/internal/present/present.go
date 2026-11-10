package present

import (
	panelv1 "dockium/gen/panel/v1"
	"dockium/internal/db"
)

func User(user *db.User, online bool) *panelv1.User {
	if user == nil {
		return nil
	}
	return &panelv1.User{
		Id:          user.ID,
		Login:       user.Login,
		Role:        user.Role,
		CreatedAt:   user.CreatedAt,
		LastLoginAt: user.LastLoginAt,
		Online:      online,
	}
}

func Agent(agent *db.Agent, instancesTotal, instancesRunning uint32) *panelv1.Agent {
	if agent == nil {
		return nil
	}
	return &panelv1.Agent{
		Id:               agent.ID,
		Name:             agent.Name,
		Country:          agent.Country,
		Online:           agent.Online,
		Enabled:          agent.Enabled,
		Version:          agent.Version,
		Hostname:         agent.Hostname,
		Platform:         agent.Platform,
		Arch:             agent.Arch,
		DockerVersion:    agent.DockerVersion,
		DockerOk:         agent.DockerOK,
		DockerError:      agent.DockerError,
		CpuModel:         agent.CPUModel,
		CpuCores:         agent.CPUCores,
		CpuPercent:       agent.CPUPercent,
		MemoryUsedBytes:  agent.MemUsed,
		MemoryTotalBytes: agent.MemTotal,
		DiskUsedBytes:    agent.DiskUsed,
		DiskTotalBytes:   agent.DiskTotal,
		NetworkInBps:     agent.NetInBPS,
		NetworkOutBps:    agent.NetOutBPS,
		InstancesTotal:   instancesTotal,
		InstancesRunning: instancesRunning,
		LatencyMs:        agent.LatencyMS,
		LastSeenAt:       agent.LastSeenAt,
		ConnectedAt:      agent.ConnectedAt,
		CreatedAt:        agent.CreatedAt,
	}
}

func Instance(instance *db.Instance, permission db.Permission) *panelv1.Instance {
	if instance == nil {
		return nil
	}
	return &panelv1.Instance{
		Id:              instance.ID,
		Name:            instance.Name,
		AgentId:         instance.AgentID,
		AgentName:       instance.AgentName,
		AgentOnline:     instance.AgentOnline,
		Status:          Status(instance.Status),
		Image:           instance.Image,
		ContainerName:   instance.ContainerName,
		ContainerId:     instance.ContainerID,
		Command:         instance.Command,
		Environment:     envVars(instance.Environment),
		Ports:           ports(instance.Ports),
		Volumes:         volumes(instance.Volumes),
		NetworkMode:     instance.NetworkMode,
		RestartPolicy:   instance.RestartPolicy,
		MemoryMb:        instance.MemoryMB,
		Cpus:            instance.CPUs,
		Tty:             instance.TTY,
		DataMount:       instance.DataMount,
		ConsoleEncoding: instance.Encoding,
		AutoStart:       instance.AutoStart,
		AutoRestart:     instance.AutoRestart,
		LastError:       instance.LastError,
		ExitCode:        instance.ExitCode,
		LastStartedAt:   instance.LastStartedAt,
		CreatedAt:       instance.CreatedAt,
		UpdatedAt:       instance.UpdatedAt,
		Usage: &panelv1.InstanceUsage{
			CpuPercent:       instance.CPUPercent,
			MemoryBytes:      instance.MemBytes,
			MemoryLimitBytes: instance.MemLimitBytes,
			NetworkInBps:     instance.NetInBPS,
			NetworkOutBps:    instance.NetOutBPS,
		},
		Permissions: &panelv1.InstancePermissions{
			Control: permission.Control,
			Console: permission.Console,
			Files:   permission.Files,
		},
	}
}

func envVars(items []db.EnvVar) []*panelv1.EnvVar {
	result := make([]*panelv1.EnvVar, 0, len(items))
	for _, item := range items {
		result = append(result, &panelv1.EnvVar{Key: item.Key, Value: item.Value})
	}
	return result
}

func ports(items []db.PortMapping) []*panelv1.PortMapping {
	result := make([]*panelv1.PortMapping, 0, len(items))
	for _, item := range items {
		result = append(result, &panelv1.PortMapping{HostPort: item.HostPort, ContainerPort: item.ContainerPort, Protocol: item.Protocol})
	}
	return result
}

func volumes(items []db.VolumeMount) []*panelv1.VolumeMount {
	result := make([]*panelv1.VolumeMount, 0, len(items))
	for _, item := range items {
		result = append(result, &panelv1.VolumeMount{HostPath: item.HostPath, ContainerPath: item.ContainerPath, ReadOnly: item.ReadOnly})
	}
	return result
}

func Status(status string) panelv1.InstanceStatus {
	switch status {
	case db.StatusStopped:
		return panelv1.InstanceStatus_INSTANCE_STATUS_STOPPED
	case db.StatusStarting:
		return panelv1.InstanceStatus_INSTANCE_STATUS_STARTING
	case db.StatusRunning:
		return panelv1.InstanceStatus_INSTANCE_STATUS_RUNNING
	case db.StatusStopping:
		return panelv1.InstanceStatus_INSTANCE_STATUS_STOPPING
	case db.StatusExited:
		return panelv1.InstanceStatus_INSTANCE_STATUS_EXITED
	case db.StatusFailed:
		return panelv1.InstanceStatus_INSTANCE_STATUS_FAILED
	default:
		return panelv1.InstanceStatus_INSTANCE_STATUS_UNSPECIFIED
	}
}

func StatusFromAgent(value int32) string {
	switch value {
	case 1:
		return db.StatusStopped
	case 2:
		return db.StatusStarting
	case 3:
		return db.StatusRunning
	case 4:
		return db.StatusStopping
	case 5:
		return db.StatusExited
	case 6:
		return db.StatusFailed
	default:
		return db.StatusStopped
	}
}

func Task(task *db.Task) *panelv1.ScheduledTask {
	if task == nil {
		return nil
	}
	return &panelv1.ScheduledTask{
		Id:              task.ID,
		InstanceId:      task.InstanceID,
		InstanceName:    task.InstanceName,
		Name:            task.Name,
		Action:          TaskAction(task.Action),
		Payload:         task.Payload,
		Kind:            TaskKind(task.Kind),
		IntervalMinutes: task.IntervalMinutes,
		TimeOfDay:       task.TimeOfDay,
		Enabled:         task.Enabled,
		LastRunAt:       task.LastRunAt,
		NextRunAt:       task.NextRunAt,
		LastError:       task.LastError,
		CreatedAt:       task.CreatedAt,
	}
}

func TaskAction(action string) panelv1.ScheduleAction {
	switch action {
	case "start":
		return panelv1.ScheduleAction_SCHEDULE_ACTION_START
	case "stop":
		return panelv1.ScheduleAction_SCHEDULE_ACTION_STOP
	case "restart":
		return panelv1.ScheduleAction_SCHEDULE_ACTION_RESTART
	case "command":
		return panelv1.ScheduleAction_SCHEDULE_ACTION_COMMAND
	default:
		return panelv1.ScheduleAction_SCHEDULE_ACTION_UNSPECIFIED
	}
}

func TaskKind(kind string) panelv1.ScheduleKind {
	switch kind {
	case "interval":
		return panelv1.ScheduleKind_SCHEDULE_KIND_INTERVAL
	case "daily":
		return panelv1.ScheduleKind_SCHEDULE_KIND_DAILY
	default:
		return panelv1.ScheduleKind_SCHEDULE_KIND_UNSPECIFIED
	}
}

func LoginLog(entry *db.LoginLog) *panelv1.LoginLog {
	if entry == nil {
		return nil
	}
	return &panelv1.LoginLog{
		Id:        entry.ID,
		Login:     entry.Login,
		Success:   entry.Success,
		Reason:    entry.Reason,
		Ip:        entry.IP,
		UserAgent: entry.UserAgent,
		Browser:   entry.Browser,
		Os:        entry.OS,
		CreatedAt: entry.CreatedAt,
	}
}

func Ban(ban *db.Ban) *panelv1.Ban {
	if ban == nil {
		return nil
	}
	return &panelv1.Ban{
		Id:        ban.ID,
		Ip:        ban.IP,
		Reason:    ban.Reason,
		Source:    ban.Source,
		BannedAt:  ban.BannedAt,
		ExpiresAt: ban.ExpiresAt,
	}
}

func ApiRequest(entry *db.ApiRequest) *panelv1.ApiRequest {
	if entry == nil {
		return nil
	}
	return &panelv1.ApiRequest{
		Id:        entry.ID,
		Login:     entry.Login,
		Procedure: entry.Procedure,
		Status:    uint32(entry.Status),
		Ip:        entry.IP,
		CreatedAt: entry.CreatedAt,
	}
}

func SessionInfo(item *db.SessionRow, current bool) *panelv1.SessionInfo {
	if item == nil {
		return nil
	}
	return &panelv1.SessionInfo{
		Id:         item.ID,
		UserId:     item.UserID,
		Login:      item.Login,
		Ip:         item.IP,
		UserAgent:  item.UserAgent,
		CreatedAt:  item.CreatedAt,
		LastSeenAt: item.LastSeenAt,
		ExpiresAt:  item.ExpiresAt,
		Current:    current,
	}
}
