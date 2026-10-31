package control

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	agentv1 "dockium/gen/agent/v1"
	"dockium/internal/db"
	"dockium/internal/hub"
	"dockium/internal/present"
	"dockium/internal/realtime"
)

type Controller struct {
	Store     *db.DB
	Hub       *hub.Hub
	Publisher *realtime.Publisher
	Log       *slog.Logger
}

func New(store *db.DB, agentHub *hub.Hub, publisher *realtime.Publisher, log *slog.Logger) *Controller {
	return &Controller{Store: store, Hub: agentHub, Publisher: publisher, Log: log}
}

func (c *Controller) Publish(instance *db.Instance) {
	if instance != nil {
		c.Publisher.InstanceUpdated(instance)
	}
}

func (c *Controller) SetStatus(ctx context.Context, instance *db.Instance, status, message string, exitCode int32) (*db.Instance, error) {
	if err := c.Store.SetInstanceStatus(ctx, instance.ID, status, message, exitCode); err != nil {
		return nil, err
	}
	updated, err := c.Store.InstanceByID(ctx, instance.ID)
	if err != nil {
		return nil, err
	}
	c.Publish(updated)
	return updated, nil
}

func (c *Controller) ApplyAgentState(ctx context.Context, instance *db.Instance, state *agentv1.InstanceState) (*db.Instance, error) {
	if state == nil {
		return nil, fmt.Errorf("агент не вернул состояние экземпляра")
	}
	status := present.StatusFromAgent(int32(state.Status))
	usage := db.InstanceUsageUpdate{
		CPUPercent:    state.CpuPercent,
		MemBytes:      state.MemoryBytes,
		MemLimitBytes: state.MemoryLimitBytes,
		NetInBPS:      state.NetworkInBps,
		NetOutBPS:     state.NetworkOutBps,
	}
	if err := c.Store.ApplyInstanceState(ctx, instance.ID, status, state.Error, state.ContainerId, state.ExitCode, state.StartedAt, usage); err != nil {
		return nil, err
	}
	updated, err := c.Store.InstanceByID(ctx, instance.ID)
	if err != nil {
		return nil, err
	}
	c.Publish(updated)
	return updated, nil
}

func (c *Controller) Ensure(ctx context.Context, instance *db.Instance) (*db.Instance, error) {
	if !c.Hub.Online(instance.AgentID) {
		return nil, hub.ErrOffline
	}
	command := &agentv1.Command{Payload: &agentv1.Command_Ensure{Ensure: &agentv1.EnsureInstance{
		Spec: hub.SpecFromInstance(instance),
	}}}
	result, err := c.Hub.Execute(ctx, instance.AgentID, command, 5*time.Minute)
	if err != nil {
		return nil, err
	}
	if state := result.GetInstance(); state != nil {
		return c.ApplyAgentState(ctx, instance, state)
	}
	return instance, nil
}

func (c *Controller) Start(ctx context.Context, instance *db.Instance) (*db.Instance, error) {
	if !c.Hub.Online(instance.AgentID) {
		return nil, hub.ErrOffline
	}
	current, err := c.SetStatus(ctx, instance, db.StatusStarting, "", 0)
	if err != nil {
		return nil, err
	}
	if _, err := c.Ensure(ctx, current); err != nil {
		failed, _ := c.SetStatus(ctx, current, db.StatusFailed, err.Error(), current.ExitCode)
		if failed != nil {
			return failed, err
		}
		return current, err
	}
	command := &agentv1.Command{Payload: &agentv1.Command_Start{Start: &agentv1.InstanceTarget{Id: current.ID}}}
	result, err := c.Hub.Execute(ctx, current.AgentID, command, 2*time.Minute)
	if err != nil {
		failed, _ := c.SetStatus(ctx, current, db.StatusFailed, err.Error(), current.ExitCode)
		if failed != nil {
			return failed, err
		}
		return current, err
	}
	if state := result.GetInstance(); state != nil {
		return c.ApplyAgentState(ctx, current, state)
	}
	return c.Store.InstanceByID(ctx, current.ID)
}

func (c *Controller) Stop(ctx context.Context, instance *db.Instance, force bool) (*db.Instance, error) {
	if !c.Hub.Online(instance.AgentID) {
		return nil, hub.ErrOffline
	}
	status := db.StatusStopping
	if force {
		status = db.StatusStopping
	}
	current, err := c.SetStatus(ctx, instance, status, "", instance.ExitCode)
	if err != nil {
		return nil, err
	}
	command := &agentv1.Command{Payload: &agentv1.Command_Stop{Stop: &agentv1.StopInstance{
		Id: current.ID, TimeoutSeconds: 15, Force: force,
	}}}
	result, err := c.Hub.Execute(ctx, current.AgentID, command, 2*time.Minute)
	if err != nil {
		failed, _ := c.SetStatus(ctx, current, db.StatusFailed, err.Error(), current.ExitCode)
		if failed != nil {
			return failed, err
		}
		return current, err
	}
	state := result.GetInstance()
	if state != nil {
		state.Status = agentv1.InstanceStatus_INSTANCE_STATUS_STOPPED
		return c.ApplyAgentState(ctx, current, state)
	}
	return c.SetStatus(ctx, current, db.StatusStopped, "", 0)
}

func (c *Controller) Restart(ctx context.Context, instance *db.Instance) (*db.Instance, error) {
	if !c.Hub.Online(instance.AgentID) {
		return nil, hub.ErrOffline
	}
	current, err := c.SetStatus(ctx, instance, db.StatusStarting, "", instance.ExitCode)
	if err != nil {
		return nil, err
	}
	command := &agentv1.Command{Payload: &agentv1.Command_Restart{Restart: &agentv1.InstanceTarget{Id: current.ID}}}
	result, err := c.Hub.Execute(ctx, current.AgentID, command, 2*time.Minute)
	if err != nil {
		failed, _ := c.SetStatus(ctx, current, db.StatusFailed, err.Error(), current.ExitCode)
		if failed != nil {
			return failed, err
		}
		return current, err
	}
	if state := result.GetInstance(); state != nil {
		return c.ApplyAgentState(ctx, current, state)
	}
	return c.Store.InstanceByID(ctx, current.ID)
}

func (c *Controller) RemoveFromAgent(ctx context.Context, instance *db.Instance, removeData bool) error {
	if !c.Hub.Online(instance.AgentID) {
		return hub.ErrOffline
	}
	command := &agentv1.Command{Payload: &agentv1.Command_Remove{Remove: &agentv1.RemoveInstance{
		Id: instance.ID, RemoveData: removeData,
	}}}
	_, err := c.Hub.Execute(ctx, instance.AgentID, command, 2*time.Minute)
	return err
}

func (c *Controller) PublishState(ctx context.Context, instanceID int64) {
	instance, err := c.Store.InstanceByID(ctx, instanceID)
	if err != nil {
		return
	}
	c.Publish(instance)
}
