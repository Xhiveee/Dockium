package panelapi

import (
	"context"

	"connectrpc.com/connect"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	panelv1 "dockium/gen/panel/v1"
)

type setupService struct{ *Server }

type authService struct{ *Server }

type userService struct{ *Server }

func (s *userService) List(ctx context.Context, request *connect.Request[panelv1.Page]) (*connect.Response[panelv1.UserList], error) {
	return s.Server.ListUsers(ctx, request)
}

func (s *userService) Create(ctx context.Context, request *connect.Request[panelv1.CreateUserRequest]) (*connect.Response[panelv1.User], error) {
	return s.Server.CreateUser(ctx, request)
}

func (s *userService) Update(ctx context.Context, request *connect.Request[panelv1.UpdateUserRequest]) (*connect.Response[panelv1.User], error) {
	return s.Server.UpdateUser(ctx, request)
}

func (s *userService) Delete(ctx context.Context, request *connect.Request[panelv1.UserIdRequest]) (*connect.Response[emptypb.Empty], error) {
	return s.Server.DeleteUser(ctx, request)
}

type agentService struct{ *Server }

func (s *agentService) List(ctx context.Context, request *connect.Request[emptypb.Empty]) (*connect.Response[panelv1.AgentList], error) {
	return s.Server.ListAgents(ctx, request)
}

func (s *agentService) Watch(ctx context.Context, request *connect.Request[emptypb.Empty], stream *connect.ServerStream[panelv1.AgentEvent]) error {
	return s.Server.WatchAgents(ctx, request, stream)
}

func (s *agentService) Create(ctx context.Context, request *connect.Request[panelv1.CreateAgentRequest]) (*connect.Response[panelv1.AgentCreated], error) {
	return s.Server.CreateAgent(ctx, request)
}

func (s *agentService) Update(ctx context.Context, request *connect.Request[panelv1.UpdateAgentRequest]) (*connect.Response[panelv1.Agent], error) {
	return s.Server.UpdateAgent(ctx, request)
}

func (s *agentService) Delete(ctx context.Context, request *connect.Request[panelv1.UserIdRequest]) (*connect.Response[emptypb.Empty], error) {
	return s.Server.DeleteAgent(ctx, request)
}

type instanceService struct{ *Server }

func (s *instanceService) List(ctx context.Context, request *connect.Request[emptypb.Empty]) (*connect.Response[panelv1.InstanceList], error) {
	return s.Server.ListInstances(ctx, request)
}

func (s *instanceService) Get(ctx context.Context, request *connect.Request[panelv1.InstanceIdRequest]) (*connect.Response[panelv1.Instance], error) {
	return s.Server.GetInstance(ctx, request)
}

func (s *instanceService) Watch(ctx context.Context, request *connect.Request[emptypb.Empty], stream *connect.ServerStream[panelv1.InstanceEvent]) error {
	return s.Server.WatchInstances(ctx, request, stream)
}

func (s *instanceService) Create(ctx context.Context, request *connect.Request[panelv1.CreateInstanceRequest]) (*connect.Response[panelv1.Instance], error) {
	return s.Server.CreateInstance(ctx, request)
}

func (s *instanceService) Update(ctx context.Context, request *connect.Request[panelv1.UpdateInstanceRequest]) (*connect.Response[panelv1.Instance], error) {
	return s.Server.UpdateInstance(ctx, request)
}

func (s *instanceService) Delete(ctx context.Context, request *connect.Request[panelv1.DeleteInstanceRequest]) (*connect.Response[emptypb.Empty], error) {
	return s.Server.DeleteInstance(ctx, request)
}

func (s *instanceService) Start(ctx context.Context, request *connect.Request[panelv1.InstanceIdRequest]) (*connect.Response[panelv1.Instance], error) {
	return s.Server.StartInstance(ctx, request)
}

func (s *instanceService) Stop(ctx context.Context, request *connect.Request[panelv1.InstanceIdRequest]) (*connect.Response[panelv1.Instance], error) {
	return s.Server.StopInstance(ctx, request)
}

func (s *instanceService) Restart(ctx context.Context, request *connect.Request[panelv1.InstanceIdRequest]) (*connect.Response[panelv1.Instance], error) {
	return s.Server.RestartInstance(ctx, request)
}

func (s *instanceService) Kill(ctx context.Context, request *connect.Request[panelv1.InstanceIdRequest]) (*connect.Response[panelv1.Instance], error) {
	return s.Server.KillInstance(ctx, request)
}

type consoleService struct{ *Server }

func (s *consoleService) Attach(ctx context.Context, request *connect.Request[panelv1.AttachConsoleRequest], stream *connect.ServerStream[panelv1.ConsoleLine]) error {
	return s.Server.AttachConsole(ctx, request, stream)
}

func (s *consoleService) Send(ctx context.Context, request *connect.Request[panelv1.SendConsoleCommandRequest]) (*connect.Response[emptypb.Empty], error) {
	return s.Server.SendConsoleCommand(ctx, request)
}

type fileService struct{ *Server }

type dockerService struct{ *Server }

func (s *dockerService) ListImages(ctx context.Context, request *connect.Request[panelv1.AgentIdRequest]) (*connect.Response[panelv1.DockerImageList], error) {
	return s.Server.ListDockerImages(ctx, request)
}

func (s *dockerService) ListContainers(ctx context.Context, request *connect.Request[panelv1.AgentIdRequest]) (*connect.Response[panelv1.DockerContainerList], error) {
	return s.Server.ListDockerContainers(ctx, request)
}

func (s *dockerService) Inspect(ctx context.Context, request *connect.Request[panelv1.InspectDockerRequest]) (*connect.Response[panelv1.InspectDockerResponse], error) {
	return s.Server.InspectDocker(ctx, request)
}

func (s *dockerService) RemoveImage(ctx context.Context, request *connect.Request[panelv1.RemoveImageRequest]) (*connect.Response[emptypb.Empty], error) {
	return s.Server.RemoveDockerImage(ctx, request)
}

func (s *dockerService) PullImage(ctx context.Context, request *connect.Request[panelv1.PullImageRequest], stream *connect.ServerStream[panelv1.BuildOutput]) error {
	return s.Server.PullDockerImage(ctx, request, stream)
}

func (s *dockerService) BuildImage(ctx context.Context, request *connect.Request[panelv1.BuildImageRequest], stream *connect.ServerStream[panelv1.BuildOutput]) error {
	return s.Server.BuildDockerImage(ctx, request, stream)
}

type scheduleService struct{ *Server }

func (s *scheduleService) List(ctx context.Context, request *connect.Request[panelv1.ListScheduledTasksRequest]) (*connect.Response[panelv1.ScheduledTaskList], error) {
	return s.Server.ListSchedules(ctx, request)
}

func (s *scheduleService) Create(ctx context.Context, request *connect.Request[panelv1.CreateScheduledTaskRequest]) (*connect.Response[panelv1.ScheduledTask], error) {
	return s.Server.CreateSchedule(ctx, request)
}

func (s *scheduleService) Update(ctx context.Context, request *connect.Request[panelv1.UpdateScheduledTaskRequest]) (*connect.Response[panelv1.ScheduledTask], error) {
	return s.Server.UpdateSchedule(ctx, request)
}

func (s *scheduleService) Delete(ctx context.Context, request *connect.Request[panelv1.UserIdRequest]) (*connect.Response[emptypb.Empty], error) {
	return s.Server.DeleteSchedule(ctx, request)
}

type securityService struct{ *Server }

type securityEventsService struct{ *Server }

func (s *securityEventsService) Watch(ctx context.Context, request *connect.Request[emptypb.Empty], stream *connect.ServerStream[panelv1.SecurityEvent]) error {
	return s.Server.WatchSecurity(ctx, request, stream)
}

type dashboardService struct{ *Server }

func (s *dashboardService) GetOverview(ctx context.Context, request *connect.Request[emptypb.Empty]) (*connect.Response[panelv1.DashboardOverview], error) {
	return s.Server.GetDashboard(ctx, request)
}
