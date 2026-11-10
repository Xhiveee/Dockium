package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	database, err := Open(context.Background(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

func TestSetupAndUsers(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)

	initialized, err := database.SetupInitialized(ctx)
	if err != nil || initialized {
		t.Fatalf("initialized = %v, err = %v", initialized, err)
	}
	admin, err := database.CreateFirstAdmin(ctx, "admin", "hash")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if admin.Role != RoleAdmin {
		t.Fatalf("role = %s", admin.Role)
	}
	if _, err := database.CreateFirstAdmin(ctx, "second", "hash"); !errors.Is(err, ErrConflict) {
		t.Fatalf("second setup error = %v", err)
	}
	initialized, _ = database.SetupInitialized(ctx)
	if !initialized {
		t.Fatal("setup must be initialized")
	}

	user, err := database.CreateUser(ctx, "operator", "hash2", RoleUser)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := database.CreateUser(ctx, "Operator", "hash3", RoleUser); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate login error = %v", err)
	}
	if found, err := database.UserByLogin(ctx, "OPERATOR"); err != nil || found.ID != user.ID {
		t.Fatalf("case-insensitive login lookup failed: %v", err)
	}
	admins, err := database.CountAdmins(ctx)
	if err != nil || admins != 1 {
		t.Fatalf("admins = %d, err = %v", admins, err)
	}
	count, _ := database.CountUsers(ctx)
	if count != 2 {
		t.Fatalf("users = %d", count)
	}
	if err := database.UpdateUser(ctx, user.ID, RoleAdmin, nil); err != nil {
		t.Fatalf("update user: %v", err)
	}
	updated, _ := database.UserByID(ctx, user.ID)
	if updated.Role != RoleAdmin {
		t.Fatalf("updated role = %s", updated.Role)
	}
}

func TestSessionsAndSecurity(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	admin, _ := database.CreateFirstAdmin(ctx, "admin", "hash")

	now := Now()
	session := &Session{ID: "hash-id", UserID: admin.ID, CreatedAt: now, LastSeenAt: now, ExpiresAt: now + 1000, IP: "127.0.0.1", UserAgent: "test"}
	if err := database.CreateSession(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if found, err := database.SessionByID(ctx, session.ID); err != nil || found.UserID != admin.ID {
		t.Fatalf("session lookup: %v", err)
	}
	if err := database.TouchSession(ctx, session.ID, now+10); err != nil {
		t.Fatalf("touch session: %v", err)
	}
	sessions, err := database.ListSessions(ctx, now)
	if err != nil || len(sessions) != 1 || sessions[0].Login != "admin" {
		t.Fatalf("list sessions: %v, %d", err, len(sessions))
	}

	for index := 0; index < 5; index++ {
		if _, err := database.InsertLoginLog(ctx, &LoginLog{Login: "admin", Success: false, IP: "10.0.0.1", CreatedAt: Now()}); err != nil {
			t.Fatalf("login log: %v", err)
		}
	}
	failures, err := database.CountFailedLogins(ctx, "10.0.0.1", Now()-60_000)
	if err != nil || failures != 5 {
		t.Fatalf("failures = %d, err = %v", failures, err)
	}
	logID, err := database.InsertLoginLog(ctx, &LoginLog{Login: "admin", Success: true, IP: "8.8.8.8", Country: "США", CreatedAt: Now()})
	if err != nil {
		t.Fatalf("login log with country: %v", err)
	}
	if err := database.UpdateLoginLogCountry(ctx, logID, "United States"); err != nil {
		t.Fatalf("update login log country: %v", err)
	}
	logs, err := database.ListLoginLogs(ctx, 10, 0)
	if err != nil || len(logs) != 6 {
		t.Fatalf("list login logs: %v, %d", err, len(logs))
	}
	if logs[0].ID != logID || logs[0].Country != "United States" {
		t.Fatalf("login log country = %+v", logs[0])
	}
	ban, err := database.CreateBan(ctx, &Ban{IP: "10.0.0.1", Reason: "test", Source: "auto", Country: "Локальная сеть", BannedAt: Now(), ExpiresAt: Now() + 60_000})
	if err != nil {
		t.Fatalf("create ban: %v", err)
	}
	if ban.Country != "Локальная сеть" {
		t.Fatalf("ban country = %q", ban.Country)
	}
	if err := database.UpdateBanCountry(ctx, ban.ID, "Netherlands"); err != nil {
		t.Fatalf("update ban country: %v", err)
	}
	if active, err := database.ActiveBan(ctx, "10.0.0.1", Now()); err != nil || active.ID != ban.ID || active.Country != "Netherlands" {
		t.Fatalf("active ban: %v", err)
	}
	bans, err := database.ListBans(ctx, Now())
	if err != nil || len(bans) != 1 || bans[0].Country != "Netherlands" {
		t.Fatalf("list bans: %v, %+v", err, bans)
	}
	if _, err := database.ActiveBan(ctx, "10.0.0.1", Now()+120_000); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired ban error = %v", err)
	}
	if _, err := database.InsertApiRequest(ctx, &ApiRequest{Login: "admin", Procedure: "test/Method", Status: 200, IP: "127.0.0.1", CreatedAt: Now()}); err != nil {
		t.Fatalf("api request: %v", err)
	}
	requests, err := database.ListApiRequests(ctx, 10, 0)
	if err != nil || len(requests) != 1 {
		t.Fatalf("api requests: %v", err)
	}
}

func TestAgentsAndInstances(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	admin, _ := database.CreateFirstAdmin(ctx, "admin", "hash")

	agent, err := database.CreateAgent(ctx, "local", "RU", "token-hash")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if found, err := database.AgentByTokenHash(ctx, "token-hash"); err != nil || found.ID != agent.ID {
		t.Fatalf("agent token lookup: %v", err)
	}
	if err := database.MarkAgentOnline(ctx, agent.ID, "1.0.0", "host", "linux", "amd64", "29.0", "", "cpu", 8, Now()); err != nil {
		t.Fatalf("online: %v", err)
	}
	if err := database.ApplyAgentTelemetry(ctx, agent.ID, AgentTelemetry{CPUPercent: 12.5, MemUsed: 100, MemTotal: 200, LatencyMS: 7}, Now()); err != nil {
		t.Fatalf("telemetry: %v", err)
	}
	online, err := database.AgentByID(ctx, agent.ID)
	if err != nil || !online.Online || online.CPUPercent != 12.5 || online.LatencyMS != 7 {
		t.Fatalf("agent state: %+v", online)
	}
	if !online.Enabled {
		t.Fatal("agent must be enabled by default")
	}
	if err := database.UpdateAgent(ctx, agent.ID, "renamed", "US", false); err != nil {
		t.Fatalf("update agent: %v", err)
	}
	disabled, err := database.AgentByID(ctx, agent.ID)
	if err != nil || disabled.Name != "renamed" || disabled.Country != "US" || disabled.Enabled {
		t.Fatalf("updated agent = %+v, err = %v", disabled, err)
	}

	instance, err := database.CreateInstance(ctx, &Instance{
		Name:          "test",
		AgentID:       agent.ID,
		Image:         "nginx:alpine",
		ContainerName: "dockium-test",
		Command:       []string{"nginx", "-g", "daemon off;"},
		Environment:   []EnvVar{{Key: "A", Value: "B"}},
		Ports:         []PortMapping{{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"}},
		NetworkMode:   "bridge",
		RestartPolicy: "no",
		TTY:           true,
		DataMount:     "/data",
		Encoding:      "utf-8",
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	found, err := database.InstanceByID(ctx, instance.ID)
	if err != nil || len(found.Ports) != 1 || found.Environment[0].Key != "A" || !found.AgentOnline {
		t.Fatalf("instance = %+v, err = %v", found, err)
	}
	if err := database.MarkInstanceStarted(ctx, instance.ID, "container-id"); err != nil {
		t.Fatalf("started: %v", err)
	}
	if err := database.ApplyInstanceState(ctx, instance.ID, StatusRunning, "", "container-id", 0, Now(), InstanceUsageUpdate{CPUPercent: 3.2}); err != nil {
		t.Fatalf("state: %v", err)
	}
	found, _ = database.InstanceByID(ctx, instance.ID)
	if found.Status != StatusRunning || found.CPUPercent != 3.2 || found.ContainerID != "container-id" {
		t.Fatalf("instance state = %+v", found)
	}
	total, running, attention, err := database.CountInstances(ctx)
	if err != nil || total != 1 || running != 1 || attention != 0 {
		t.Fatalf("counts = %d/%d/%d, err = %v", total, running, attention, err)
	}
	if err := database.SetAccess(ctx, admin.ID, []InstanceAccess{{InstanceID: instance.ID, Control: true, Console: false, Files: true}}); err != nil {
		t.Fatalf("set access: %v", err)
	}
	permission, err := database.Permission(ctx, admin.ID, instance.ID)
	if err != nil || !permission.Control || permission.Console || !permission.Files {
		t.Fatalf("permission = %+v, err = %v", permission, err)
	}
	access, _ := database.ListAccess(ctx, admin.ID)
	if len(access) != 1 {
		t.Fatalf("access list = %+v", access)
	}
	userInstances, err := database.ListUserInstances(ctx, admin.ID)
	if err != nil || len(userInstances) != 1 {
		t.Fatalf("user instances: %v", err)
	}
}

func TestTasks(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	agent, _ := database.CreateAgent(ctx, "local", "", "hash")
	instance, err := database.CreateInstance(ctx, &Instance{Name: "game", AgentID: agent.ID, Image: "alpine", ContainerName: "game-container", NetworkMode: "bridge", RestartPolicy: "no", TTY: true, DataMount: "/data", Encoding: "utf-8"})
	if err != nil {
		t.Fatalf("instance: %v", err)
	}
	task, err := database.CreateTask(ctx, &Task{
		InstanceID:      instance.ID,
		Name:            "restart",
		Action:          "restart",
		Kind:            "interval",
		Enabled:         true,
		IntervalMinutes: 5,
		NextRunAt:       1000,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	due, err := database.DueTasks(ctx, 2000)
	if err != nil || len(due) != 1 || due[0].InstanceName != "game" {
		t.Fatalf("due tasks = %+v, err = %v", due, err)
	}
	if err := database.MarkTaskRun(ctx, task.ID, 2000, 5000, true, "ok"); err != nil {
		t.Fatalf("mark run: %v", err)
	}
	updated, _ := database.TaskByID(ctx, task.ID)
	if updated.LastRunAt != 2000 || updated.NextRunAt != 5000 {
		t.Fatalf("task = %+v", updated)
	}
	upcoming, err := database.UpcomingTasks(ctx, 0, 10)
	if err != nil || len(upcoming) != 1 {
		t.Fatalf("upcoming = %v, %v", len(upcoming), err)
	}
	if err := database.DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("delete task: %v", err)
	}
}

func TestDeleteAgentCascadesInstances(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	agent, _ := database.CreateAgent(ctx, "local", "", "hash")
	if _, err := database.CreateInstance(ctx, &Instance{Name: "game", AgentID: agent.ID, Image: "alpine", ContainerName: "game", NetworkMode: "bridge", RestartPolicy: "no", DataMount: "/data", Encoding: "utf-8"}); err != nil {
		t.Fatalf("instance: %v", err)
	}
	if err := database.DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatalf("delete agent: %v", err)
	}
	instances, err := database.ListInstances(ctx)
	if err != nil || len(instances) != 0 {
		t.Fatalf("cascade failed: %d, %v", len(instances), err)
	}
}

func TestPanelSettings(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)

	settings, err := database.PanelSettings(ctx)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if settings.MaxUploadMB != 1024 {
		t.Fatalf("default max upload = %d, want 1024", settings.MaxUploadMB)
	}
	if err := database.UpdatePanelSettings(ctx, &PanelSettings{MaxUploadMB: 2048}); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	settings, err = database.PanelSettings(ctx)
	if err != nil || settings.MaxUploadMB != 2048 {
		t.Fatalf("updated max upload = %+v, err = %v", settings, err)
	}
	if err := database.UpdatePanelSettings(ctx, &PanelSettings{MaxUploadMB: 5000}); err == nil {
		t.Fatal("out-of-range max upload must be rejected")
	}
}
