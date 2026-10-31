package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	agentv1 "dockium/gen/agent/v1"
	"dockium/internal/control"
	"dockium/internal/db"
	"dockium/internal/hub"
)

type Scheduler struct {
	Store   *db.DB
	Control *control.Controller
	Hub     *hub.Hub
	Log     *slog.Logger

	mu      sync.Mutex
	running map[int64]bool
}

func New(store *db.DB, controller *control.Controller, agentHub *hub.Hub, log *slog.Logger) *Scheduler {
	return &Scheduler{Store: store, Control: controller, Hub: agentHub, Log: log, running: make(map[int64]bool)}
}

func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	tasks, err := s.Store.DueTasks(ctx, time.Now().UTC().UnixMilli())
	if err != nil {
		s.Log.Error("scheduler query", "error", err)
		return
	}
	for _, task := range tasks {
		s.mu.Lock()
		if s.running[task.ID] {
			s.mu.Unlock()
			continue
		}
		s.running[task.ID] = true
		s.mu.Unlock()
		go func(task *db.Task) {
			defer func() {
				s.mu.Lock()
				delete(s.running, task.ID)
				s.mu.Unlock()
			}()
			s.execute(ctx, task)
		}(task)
	}
}

func (s *Scheduler) execute(parent context.Context, task *db.Task) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()
	instance, err := s.Store.InstanceByID(ctx, task.InstanceID)
	if err != nil {
		s.mark(ctx, task, fmt.Errorf("экземпляр не найден"))
		return
	}
	var runErr error
	switch task.Action {
	case "start":
		_, runErr = s.Control.Start(ctx, instance)
	case "stop":
		_, runErr = s.Control.Stop(ctx, instance, false)
	case "restart":
		_, runErr = s.Control.Restart(ctx, instance)
	case "command":
		runErr = s.sendCommand(ctx, instance, task.Payload)
	default:
		runErr = fmt.Errorf("неизвестное действие %q", task.Action)
	}
	if runErr != nil {
		s.Log.Warn("scheduled task failed", "task_id", task.ID, "instance_id", task.InstanceID, "error", runErr)
	}
	s.mark(ctx, task, runErr)
}

func (s *Scheduler) sendCommand(ctx context.Context, instance *db.Instance, payload string) error {
	if strings.TrimSpace(payload) == "" {
		return fmt.Errorf("не задана команда")
	}
	command := &agentv1.Command{Payload: &agentv1.Command_ConsoleInput{ConsoleInput: &agentv1.ConsoleInput{
		InstanceId: instance.ID, Text: payload,
	}}}
	_, err := s.Hub.Execute(ctx, instance.AgentID, command, 30*time.Second)
	return err
}

func (s *Scheduler) mark(ctx context.Context, task *db.Task, taskErr error) {
	now := time.Now()
	next := NextRun(task.Kind, task.IntervalMinutes, task.TimeOfDay, now)
	message := ""
	if taskErr != nil {
		message = taskErr.Error()
	}
	if err := s.Store.MarkTaskRun(ctx, task.ID, now.UTC().UnixMilli(), next, true, message); err != nil {
		s.Log.Error("mark scheduled task", "task_id", task.ID, "error", err)
	}
}

func NextRun(kind string, intervalMinutes uint32, timeOfDay string, from time.Time) int64 {
	switch kind {
	case "interval":
		if intervalMinutes == 0 {
			return 0
		}
		return from.Add(time.Duration(intervalMinutes) * time.Minute).UTC().UnixMilli()
	case "daily":
		hour, minute, err := ParseTimeOfDay(timeOfDay)
		if err != nil {
			return 0
		}
		local := from
		candidate := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, local.Location())
		if !candidate.After(local) {
			candidate = candidate.Add(24 * time.Hour)
		}
		return candidate.UTC().UnixMilli()
	default:
		return 0
	}
}

func ParseTimeOfDay(value string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("время должно быть в формате ЧЧ:ММ")
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("некорректный час")
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("некорректные минуты")
	}
	return hour, minute, nil
}
