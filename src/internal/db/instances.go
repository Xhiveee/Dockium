package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const instanceColumns = `i.id, i.name, i.agent_id, a.name, a.online, i.status, i.image, i.container_name, i.container_id,
	i.command, i.environment, i.ports, i.volumes, i.network_mode, i.restart_policy, i.memory_mb, i.cpus, i.tty,
	i.data_mount, i.console_encoding, i.auto_start, i.auto_restart, i.last_error, i.exit_code, i.last_started_at,
	i.cpu_percent, i.mem_bytes, i.mem_limit_bytes, i.net_in_bps, i.net_out_bps, i.created_at, i.updated_at`

const instanceFrom = ` FROM instances i JOIN agents a ON a.id = i.agent_id`

func scanInstance(row rowScanner) (*Instance, error) {
	var instance Instance
	var command, environment, ports, volumes []byte
	err := row.Scan(&instance.ID, &instance.Name, &instance.AgentID, &instance.AgentName, &instance.AgentOnline,
		&instance.Status, &instance.Image, &instance.ContainerName, &instance.ContainerID, &command, &environment,
		&ports, &volumes, &instance.NetworkMode, &instance.RestartPolicy, &instance.MemoryMB, &instance.CPUs,
		&instance.TTY, &instance.DataMount, &instance.Encoding, &instance.AutoStart, &instance.AutoRestart,
		&instance.LastError, &instance.ExitCode, &instance.LastStartedAt, &instance.CPUPercent, &instance.MemBytes,
		&instance.MemLimitBytes, &instance.NetInBPS, &instance.NetOutBPS, &instance.CreatedAt, &instance.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan instance: %w", err)
	}
	instance.Command = []string{}
	instance.Environment = []EnvVar{}
	instance.Ports = []PortMapping{}
	instance.Volumes = []VolumeMount{}
	if err := decodeJSON(command, &instance.Command); err != nil {
		return nil, fmt.Errorf("decode command: %w", err)
	}
	if err := decodeJSON(environment, &instance.Environment); err != nil {
		return nil, fmt.Errorf("decode environment: %w", err)
	}
	if err := decodeJSON(ports, &instance.Ports); err != nil {
		return nil, fmt.Errorf("decode ports: %w", err)
	}
	if err := decodeJSON(volumes, &instance.Volumes); err != nil {
		return nil, fmt.Errorf("decode volumes: %w", err)
	}
	return &instance, nil
}

func (d *DB) CreateInstance(ctx context.Context, instance *Instance) (*Instance, error) {
	now := Now()
	instance.CreatedAt = now
	instance.UpdatedAt = now
	if instance.Status == "" {
		instance.Status = StatusStopped
	}
	result, err := d.sql.ExecContext(ctx,
		`INSERT INTO instances (name, agent_id, status, image, container_name, container_id, command, environment,
		 ports, volumes, network_mode, restart_policy, memory_mb, cpus, tty, data_mount, console_encoding,
		 auto_start, auto_restart, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		instance.Name, instance.AgentID, instance.Status, instance.Image, instance.ContainerName,
		instance.ContainerID, encodeJSON(instance.Command), encodeJSON(instance.Environment),
		encodeJSON(instance.Ports), encodeJSON(instance.Volumes), instance.NetworkMode, instance.RestartPolicy,
		instance.MemoryMB, instance.CPUs, instance.TTY, instance.DataMount, instance.Encoding, instance.AutoStart,
		instance.AutoRestart, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("create instance: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create instance id: %w", err)
	}
	instance.ID = id
	return instance, nil
}

func (d *DB) UpdateInstance(ctx context.Context, instance *Instance) error {
	instance.UpdatedAt = Now()
	_, err := d.sql.ExecContext(ctx,
		`UPDATE instances SET name = ?, image = ?, container_name = ?, command = ?, environment = ?, ports = ?,
		 volumes = ?, network_mode = ?, restart_policy = ?, memory_mb = ?, cpus = ?, tty = ?, data_mount = ?,
		 console_encoding = ?, auto_start = ?, auto_restart = ?, updated_at = ? WHERE id = ?`,
		instance.Name, instance.Image, instance.ContainerName, encodeJSON(instance.Command),
		encodeJSON(instance.Environment), encodeJSON(instance.Ports), encodeJSON(instance.Volumes),
		instance.NetworkMode, instance.RestartPolicy, instance.MemoryMB, instance.CPUs, instance.TTY,
		instance.DataMount, instance.Encoding, instance.AutoStart, instance.AutoRestart, instance.UpdatedAt,
		instance.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("update instance: %w", err)
	}
	return nil
}

func (d *DB) SetInstanceAutomation(ctx context.Context, id int64, autoStart, autoRestart bool) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE instances SET auto_start = ?, auto_restart = ?, updated_at = ? WHERE id = ?`,
		autoStart, autoRestart, Now(), id)
	if err != nil {
		return fmt.Errorf("set instance automation: %w", err)
	}
	return nil
}

func (d *DB) InstanceByID(ctx context.Context, id int64) (*Instance, error) {
	row := d.sql.QueryRowContext(ctx, `SELECT `+instanceColumns+instanceFrom+` WHERE i.id = ?`, id)
	return scanInstance(row)
}

func (d *DB) ListInstances(ctx context.Context) ([]*Instance, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+instanceColumns+instanceFrom+` ORDER BY i.name COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	defer rows.Close()
	return collectInstances(rows)
}

func (d *DB) ListInstancesByAgent(ctx context.Context, agentID int64) ([]*Instance, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+instanceColumns+instanceFrom+` WHERE i.agent_id = ? ORDER BY i.name COLLATE NOCASE`, agentID)
	if err != nil {
		return nil, fmt.Errorf("list agent instances: %w", err)
	}
	defer rows.Close()
	return collectInstances(rows)
}

func (d *DB) ListUserInstances(ctx context.Context, userID int64) ([]*Instance, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT `+instanceColumns+instanceFrom+` JOIN user_instances ui ON ui.instance_id = i.id
		 WHERE ui.user_id = ? ORDER BY i.name COLLATE NOCASE`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user instances: %w", err)
	}
	defer rows.Close()
	return collectInstances(rows)
}

func collectInstances(rows *sql.Rows) ([]*Instance, error) {
	items := make([]*Instance, 0)
	for rows.Next() {
		instance, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, instance)
	}
	return items, rows.Err()
}

func (d *DB) DeleteInstance(ctx context.Context, id int64) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM instances WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete instance: %w", err)
	}
	return nil
}

func (d *DB) SetInstanceStatus(ctx context.Context, id int64, status, message string, exitCode int32) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE instances SET status = ?, last_error = ?, exit_code = ?, updated_at = ? WHERE id = ?`,
		status, message, exitCode, Now(), id)
	if err != nil {
		return fmt.Errorf("set instance status: %w", err)
	}
	return nil
}

func (d *DB) MarkInstanceStarted(ctx context.Context, id int64, containerID string) error {
	now := Now()
	_, err := d.sql.ExecContext(ctx,
		`UPDATE instances SET status = ?, container_id = ?, last_error = '', exit_code = 0, last_started_at = ?,
		 updated_at = ? WHERE id = ?`, StatusRunning, containerID, now, now, id)
	if err != nil {
		return fmt.Errorf("mark instance started: %w", err)
	}
	return nil
}

func (d *DB) ApplyInstanceState(ctx context.Context, id int64, status, message, containerID string, exitCode int32, startedAt int64, usage InstanceUsageUpdate) error {
	now := Now()
	_, err := d.sql.ExecContext(ctx,
		`UPDATE instances SET status = ?, last_error = ?, container_id = COALESCE(NULLIF(?, ''), container_id),
		 exit_code = ?, last_started_at = CASE WHEN ? > 0 THEN ? ELSE last_started_at END,
		 cpu_percent = ?, mem_bytes = ?, mem_limit_bytes = ?, net_in_bps = ?, net_out_bps = ?, updated_at = ?
		 WHERE id = ?`,
		status, message, containerID, exitCode, startedAt, startedAt, usage.CPUPercent, usage.MemBytes,
		usage.MemLimitBytes, usage.NetInBPS, usage.NetOutBPS, now, id)
	if err != nil {
		return fmt.Errorf("apply instance state: %w", err)
	}
	return nil
}

type InstanceUsageUpdate struct {
	CPUPercent    float64
	MemBytes      uint64
	MemLimitBytes uint64
	NetInBPS      uint64
	NetOutBPS     uint64
}

func (d *DB) SetInstanceUsage(ctx context.Context, id int64, usage InstanceUsageUpdate) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE instances SET cpu_percent = ?, mem_bytes = ?, mem_limit_bytes = ?, net_in_bps = ?, net_out_bps = ? WHERE id = ?`,
		usage.CPUPercent, usage.MemBytes, usage.MemLimitBytes, usage.NetInBPS, usage.NetOutBPS, id)
	if err != nil {
		return fmt.Errorf("set instance usage: %w", err)
	}
	return nil
}

func (d *DB) CountInstances(ctx context.Context) (total, running, attention int, err error) {
	row := d.sql.QueryRowContext(ctx,
		`SELECT COUNT(*),
		 COALESCE(SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(CASE WHEN status IN ('failed', 'exited') THEN 1 ELSE 0 END), 0)
		 FROM instances`)
	if err := row.Scan(&total, &running, &attention); err != nil {
		return 0, 0, 0, fmt.Errorf("count instances: %w", err)
	}
	return total, running, attention, nil
}

func (d *DB) InstanceCounts(ctx context.Context, agentID int64) (uint32, uint32, error) {
	var total, running int64
	err := d.sql.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END), 0)
		 FROM instances WHERE agent_id = ?`, agentID).Scan(&total, &running)
	if err != nil {
		return 0, 0, fmt.Errorf("instance counts: %w", err)
	}
	return uint32(total), uint32(running), nil
}

func (d *DB) InstanceCountsByAgent(ctx context.Context) (map[int64][2]uint32, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT agent_id, COUNT(*), COALESCE(SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END), 0)
		 FROM instances GROUP BY agent_id`)
	if err != nil {
		return nil, fmt.Errorf("instance counts by agent: %w", err)
	}
	defer rows.Close()
	result := make(map[int64][2]uint32)
	for rows.Next() {
		var agentID int64
		var total, running uint32
		if err := rows.Scan(&agentID, &total, &running); err != nil {
			return nil, fmt.Errorf("scan instance counts: %w", err)
		}
		result[agentID] = [2]uint32{total, running}
	}
	return result, rows.Err()
}

func (d *DB) InstanceNameTaken(ctx context.Context, name string, exceptID int64) (bool, error) {
	var count int
	err := d.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM instances WHERE name = ? COLLATE NOCASE AND id <> ?`, name, exceptID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check instance name: %w", err)
	}
	return count > 0, nil
}

type InstanceAccess struct {
	InstanceID int64
	Control    bool
	Console    bool
	Files      bool
}

func (d *DB) ListAccess(ctx context.Context, userID int64) ([]InstanceAccess, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT instance_id, control, console, files FROM user_instances WHERE user_id = ? ORDER BY instance_id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list access: %w", err)
	}
	defer rows.Close()
	items := make([]InstanceAccess, 0)
	for rows.Next() {
		var item InstanceAccess
		if err := rows.Scan(&item.InstanceID, &item.Control, &item.Console, &item.Files); err != nil {
			return nil, fmt.Errorf("scan access: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (d *DB) SetAccess(ctx context.Context, userID int64, items []InstanceAccess) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin access update: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_instances WHERE user_id = ?`, userID); err != nil {
		tx.Rollback()
		return fmt.Errorf("clear access: %w", err)
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_instances (user_id, instance_id, control, console, files) VALUES (?, ?, ?, ?, ?)`,
			userID, item.InstanceID, item.Control, item.Console, item.Files); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert access: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit access: %w", err)
	}
	return nil
}

func (d *DB) Permission(ctx context.Context, userID, instanceID int64) (Permission, error) {
	row := d.sql.QueryRowContext(ctx,
		`SELECT control, console, files FROM user_instances WHERE user_id = ? AND instance_id = ?`, userID, instanceID)
	var permission Permission
	if err := row.Scan(&permission.Control, &permission.Console, &permission.Files); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Permission{}, ErrNotFound
		}
		return Permission{}, fmt.Errorf("get permission: %w", err)
	}
	return permission, nil
}
