package panelapi

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	panelv1 "dockium/gen/panel/v1"
	"dockium/internal/db"
	"dockium/internal/present"
	"dockium/internal/scheduler"
)

func (s *Server) ListSchedules(ctx context.Context, request *connect.Request[panelv1.ListScheduledTasksRequest]) (*connect.Response[panelv1.ScheduledTaskList], error) {
	if request.Msg.InstanceId > 0 {
		if _, _, err := s.instanceFor(ctx, request.Msg.InstanceId, nil, ""); err != nil {
			return nil, err
		}
	} else if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	tasks, err := s.store.ListTasks(ctx, request.Msg.InstanceId)
	if err != nil {
		return nil, rpcError(err, "не удалось получить задачи")
	}
	items := make([]*panelv1.ScheduledTask, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, present.Task(task))
	}
	return connect.NewResponse(&panelv1.ScheduledTaskList{Items: items}), nil
}

func (s *Server) CreateSchedule(ctx context.Context, request *connect.Request[panelv1.CreateScheduledTaskRequest]) (*connect.Response[panelv1.ScheduledTask], error) {
	instance, _, err := s.instanceFor(ctx, request.Msg.InstanceId, func(permission db.Permission) bool { return permission.Control }, "нет прав на управление экземпляром")
	if err != nil {
		return nil, err
	}
	message := request.Msg
	action, err := scheduleActionName(message.Action)
	if err != nil {
		return nil, err
	}
	kind, err := scheduleKindName(message.Kind)
	if err != nil {
		return nil, err
	}
	if err := validateSchedule(message.Name, action, message.Payload, kind, message.IntervalMinutes, message.TimeOfDay); err != nil {
		return nil, invalid(err.Error())
	}
	nextRun := int64(0)
	if message.Enabled {
		nextRun = scheduler.NextRun(kind, message.IntervalMinutes, message.TimeOfDay, time.Now())
	}
	task := &db.Task{
		InstanceID:      instance.ID,
		Name:            strings.TrimSpace(message.Name),
		Action:          action,
		Payload:         strings.TrimSpace(message.Payload),
		Kind:            kind,
		IntervalMinutes: message.IntervalMinutes,
		TimeOfDay:       message.TimeOfDay,
		Enabled:         message.Enabled,
		NextRunAt:       nextRun,
	}
	created, err := s.store.CreateTask(ctx, task)
	if err != nil {
		return nil, rpcError(err, "не удалось создать задачу")
	}
	created.InstanceName = instance.Name
	return connect.NewResponse(present.Task(created)), nil
}

func (s *Server) UpdateSchedule(ctx context.Context, request *connect.Request[panelv1.UpdateScheduledTaskRequest]) (*connect.Response[panelv1.ScheduledTask], error) {
	task, err := s.store.TaskByID(ctx, request.Msg.Id)
	if err != nil {
		return nil, rpcError(err, "задача не найдена")
	}
	if _, _, err := s.instanceFor(ctx, task.InstanceID, func(permission db.Permission) bool { return permission.Control }, "нет прав на управление экземпляром"); err != nil {
		return nil, err
	}
	message := request.Msg
	action, err := scheduleActionName(message.Action)
	if err != nil {
		return nil, err
	}
	kind, err := scheduleKindName(message.Kind)
	if err != nil {
		return nil, err
	}
	if err := validateSchedule(message.Name, action, message.Payload, kind, message.IntervalMinutes, message.TimeOfDay); err != nil {
		return nil, invalid(err.Error())
	}
	task.Name = strings.TrimSpace(message.Name)
	task.Action = action
	task.Payload = strings.TrimSpace(message.Payload)
	task.Kind = kind
	task.IntervalMinutes = message.IntervalMinutes
	task.TimeOfDay = message.TimeOfDay
	task.Enabled = message.Enabled
	if task.Enabled {
		task.NextRunAt = scheduler.NextRun(kind, task.IntervalMinutes, task.TimeOfDay, time.Now())
	} else {
		task.NextRunAt = 0
	}
	task.LastError = ""
	if err := s.store.UpdateTask(ctx, task); err != nil {
		return nil, rpcError(err, "не удалось сохранить задачу")
	}
	return connect.NewResponse(present.Task(task)), nil
}

func (s *Server) DeleteSchedule(ctx context.Context, request *connect.Request[panelv1.UserIdRequest]) (*connect.Response[emptypb.Empty], error) {
	task, err := s.store.TaskByID(ctx, request.Msg.Id)
	if err != nil {
		return nil, rpcError(err, "задача не найдена")
	}
	if _, _, err := s.instanceFor(ctx, task.InstanceID, func(permission db.Permission) bool { return permission.Control }, "нет прав на управление экземпляром"); err != nil {
		return nil, err
	}
	if err := s.store.DeleteTask(ctx, task.ID); err != nil {
		return nil, rpcError(err, "не удалось удалить задачу")
	}
	return emptyResponse(), nil
}

func scheduleActionName(action panelv1.ScheduleAction) (string, error) {
	switch action {
	case panelv1.ScheduleAction_SCHEDULE_ACTION_START:
		return "start", nil
	case panelv1.ScheduleAction_SCHEDULE_ACTION_STOP:
		return "stop", nil
	case panelv1.ScheduleAction_SCHEDULE_ACTION_RESTART:
		return "restart", nil
	case panelv1.ScheduleAction_SCHEDULE_ACTION_COMMAND:
		return "command", nil
	default:
		return "", invalid("выберите действие задачи")
	}
}

func scheduleKindName(kind panelv1.ScheduleKind) (string, error) {
	switch kind {
	case panelv1.ScheduleKind_SCHEDULE_KIND_INTERVAL:
		return "interval", nil
	case panelv1.ScheduleKind_SCHEDULE_KIND_DAILY:
		return "daily", nil
	default:
		return "", invalid("выберите тип расписания")
	}
}

func validateSchedule(name, action, payload, kind string, interval uint32, timeOfDay string) error {
	name = strings.TrimSpace(name)
	if len(name) < 2 || len(name) > 64 {
		return errors.New("название задачи: от 2 до 64 символов")
	}
	if action == "command" && strings.TrimSpace(payload) == "" {
		return errors.New("для действия «команда» укажите текст команды")
	}
	if len(payload) > 4096 {
		return errors.New("слишком длинная команда")
	}
	switch kind {
	case "interval":
		if interval < 1 || interval > 10080 {
			return errors.New("интервал: от 1 до 10080 минут")
		}
	case "daily":
		if _, _, err := scheduler.ParseTimeOfDay(timeOfDay); err != nil {
			return err
		}
	}
	return nil
}
