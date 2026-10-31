package panelapi

import (
	"context"

	"connectrpc.com/connect"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	panelv1 "dockium/gen/panel/v1"
	"dockium/internal/db"
	"dockium/internal/present"
)

func (s *Server) GetDashboard(ctx context.Context, _ *connect.Request[emptypb.Empty]) (*connect.Response[panelv1.DashboardOverview], error) {
	state, err := s.requireUser(ctx)
	if err != nil {
		return nil, err
	}
	overview := &panelv1.DashboardOverview{GeneratedAt: db.Now()}
	if state.user.Role == db.RoleAdmin {
		agents, err := s.agentsSnapshot(ctx)
		if err != nil {
			return nil, err
		}
		overview.Agents = agents
		overview.AgentsTotal = uint32(len(agents))
		instances, permissions, err := s.visibleInstancesWithPermissions(ctx, state.user)
		if err != nil {
			return nil, rpcError(err, "не удалось получить экземпляры")
		}
		for _, instance := range instances {
			overview.Instances = append(overview.Instances, present.Instance(instance, permissions[instance.ID]))
			if instance.Status == db.StatusRunning {
				overview.InstancesRunning++
			}
			if instance.Status == db.StatusFailed || instance.Status == db.StatusExited {
				overview.InstancesAttention++
			}
		}
		overview.InstancesTotal = uint32(len(instances))
		users, err := s.store.CountUsers(ctx)
		if err != nil {
			return nil, rpcError(err, "не удалось посчитать пользователей")
		}
		overview.UsersTotal = uint32(users)
		for _, agent := range agents {
			if !agent.Online {
				continue
			}
			overview.AgentsOnline++
			overview.CpuPercent += agent.CpuPercent
			overview.MemoryUsedBytes += agent.MemoryUsedBytes
			overview.MemoryTotalBytes += agent.MemoryTotalBytes
			overview.DiskUsedBytes += agent.DiskUsedBytes
			overview.DiskTotalBytes += agent.DiskTotalBytes
		}
		if overview.AgentsOnline > 0 {
			overview.CpuPercent /= float64(overview.AgentsOnline)
		}
		tasks, err := s.store.UpcomingTasks(ctx, db.Now(), 5)
		if err == nil {
			for _, task := range tasks {
				overview.UpcomingTasks = append(overview.UpcomingTasks, present.Task(task))
			}
		}
		logs, err := s.store.ListLoginLogs(ctx, 5, 0)
		if err == nil {
			for _, entry := range logs {
				overview.RecentLogins = append(overview.RecentLogins, present.LoginLog(entry))
			}
		}
		return connect.NewResponse(overview), nil
	}
	instances, permissions, err := s.visibleInstancesWithPermissions(ctx, state.user)
	if err != nil {
		return nil, rpcError(err, "не удалось получить экземпляры")
	}
	for _, instance := range instances {
		overview.Instances = append(overview.Instances, present.Instance(instance, permissions[instance.ID]))
		if instance.Status == db.StatusRunning {
			overview.InstancesRunning++
		}
		if instance.Status == db.StatusFailed || instance.Status == db.StatusExited {
			overview.InstancesAttention++
		}
		overview.CpuPercent += instance.CPUPercent
		overview.MemoryUsedBytes += instance.MemBytes
		overview.MemoryTotalBytes += instance.MemLimitBytes
	}
	overview.InstancesTotal = uint32(len(instances))
	return connect.NewResponse(overview), nil
}
