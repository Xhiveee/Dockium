package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const taskColumns = `t.id, t.instance_id, i.name, t.name, t.action, t.payload, t.kind, t.interval_minutes,
	t.time_of_day, t.enabled, t.last_run_at, t.next_run_at, t.last_error, t.created_at`

const taskFrom = ` FROM scheduled_tasks t JOIN instances i ON i.id = t.instance_id`

func scanTask(row rowScanner) (*Task, error) {
	var task Task
	err := row.Scan(&task.ID, &task.InstanceID, &task.InstanceName, &task.Name, &task.Action, &task.Payload,
		&task.Kind, &task.IntervalMinutes, &task.TimeOfDay, &task.Enabled, &task.LastRunAt, &task.NextRunAt,
		&task.LastError, &task.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan task: %w", err)
	}
	return &task, nil
}

func (d *DB) ListTasks(ctx context.Context, instanceID int64) ([]*Task, error) {
	query := `SELECT ` + taskColumns + taskFrom
	args := []any{}
	if instanceID > 0 {
		query += ` WHERE t.instance_id = ?`
		args = append(args, instanceID)
	}
	query += ` ORDER BY t.next_run_at, t.id`
	rows, err := d.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	items := make([]*Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, task)
	}
	return items, rows.Err()
}

func (d *DB) TaskByID(ctx context.Context, id int64) (*Task, error) {
	row := d.sql.QueryRowContext(ctx, `SELECT `+taskColumns+taskFrom+` WHERE t.id = ?`, id)
	return scanTask(row)
}

func (d *DB) CreateTask(ctx context.Context, task *Task) (*Task, error) {
	task.CreatedAt = Now()
	result, err := d.sql.ExecContext(ctx,
		`INSERT INTO scheduled_tasks (instance_id, name, action, payload, kind, interval_minutes, time_of_day,
		 enabled, next_run_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.InstanceID, task.Name, task.Action, task.Payload, task.Kind, task.IntervalMinutes, task.TimeOfDay,
		task.Enabled, task.NextRunAt, task.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create task id: %w", err)
	}
	task.ID = id
	return task, nil
}

func (d *DB) UpdateTask(ctx context.Context, task *Task) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE scheduled_tasks SET name = ?, action = ?, payload = ?, kind = ?, interval_minutes = ?,
		 time_of_day = ?, enabled = ?, next_run_at = ? WHERE id = ?`,
		task.Name, task.Action, task.Payload, task.Kind, task.IntervalMinutes, task.TimeOfDay, task.Enabled,
		task.NextRunAt, task.ID)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	return nil
}

func (d *DB) DeleteTask(ctx context.Context, id int64) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM scheduled_tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}

func (d *DB) DueTasks(ctx context.Context, now int64) ([]*Task, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT `+taskColumns+taskFrom+` WHERE t.enabled = 1 AND t.next_run_at > 0 AND t.next_run_at <= ? ORDER BY t.next_run_at`, now)
	if err != nil {
		return nil, fmt.Errorf("due tasks: %w", err)
	}
	defer rows.Close()
	items := make([]*Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, task)
	}
	return items, rows.Err()
}

func (d *DB) MarkTaskRun(ctx context.Context, id int64, lastRun, nextRun int64, enabled bool, message string) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE scheduled_tasks SET last_run_at = ?, next_run_at = ?, enabled = ?, last_error = ? WHERE id = ?`,
		lastRun, nextRun, enabled, message, id)
	if err != nil {
		return fmt.Errorf("mark task run: %w", err)
	}
	return nil
}

func (d *DB) UpcomingTasks(ctx context.Context, now int64, limit int) ([]*Task, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT `+taskColumns+taskFrom+` WHERE t.enabled = 1 AND t.next_run_at > ? ORDER BY t.next_run_at LIMIT ?`,
		now, limit)
	if err != nil {
		return nil, fmt.Errorf("upcoming tasks: %w", err)
	}
	defer rows.Close()
	items := make([]*Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, task)
	}
	return items, rows.Err()
}
