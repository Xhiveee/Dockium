package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type AgentTelemetry struct {
	Version       string
	Hostname      string
	Platform      string
	Arch          string
	DockerVersion string
	DockerOK      bool
	DockerError   string
	CPUModel      string
	CPUCores      uint32
	CPUPercent    float64
	MemUsed       uint64
	MemTotal      uint64
	DiskUsed      uint64
	DiskTotal     uint64
	NetInBPS      uint64
	NetOutBPS     uint64
	LatencyMS     uint32
}

const agentColumns = `id, name, country, token_hash, version, hostname, platform, arch, docker_version,
	docker_ok, docker_error, cpu_model, cpu_cores, cpu_percent, mem_used, mem_total, disk_used, disk_total,
	net_in_bps, net_out_bps, latency_ms, online, enabled, last_seen_at, connected_at, created_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAgent(row rowScanner) (*Agent, error) {
	var agent Agent
	err := row.Scan(&agent.ID, &agent.Name, &agent.Country, &agent.TokenHash, &agent.Version, &agent.Hostname,
		&agent.Platform, &agent.Arch, &agent.DockerVersion, &agent.DockerOK, &agent.DockerError, &agent.CPUModel,
		&agent.CPUCores, &agent.CPUPercent, &agent.MemUsed, &agent.MemTotal, &agent.DiskUsed, &agent.DiskTotal,
		&agent.NetInBPS, &agent.NetOutBPS, &agent.LatencyMS, &agent.Online, &agent.Enabled, &agent.LastSeenAt, &agent.ConnectedAt,
		&agent.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan agent: %w", err)
	}
	return &agent, nil
}

func (d *DB) CreateAgent(ctx context.Context, name, country, tokenHash string) (*Agent, error) {
	now := Now()
	result, err := d.sql.ExecContext(ctx,
		`INSERT INTO agents (name, country, token_hash, created_at) VALUES (?, ?, ?, ?)`, name, country, tokenHash, now)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create agent id: %w", err)
	}
	return &Agent{ID: id, Name: name, Country: country, TokenHash: tokenHash, Enabled: true, CreatedAt: now}, nil
}

func (d *DB) AgentByTokenHash(ctx context.Context, hash string) (*Agent, error) {
	row := d.sql.QueryRowContext(ctx, `SELECT `+agentColumns+` FROM agents WHERE token_hash = ?`, hash)
	return scanAgent(row)
}

func (d *DB) AgentByID(ctx context.Context, id int64) (*Agent, error) {
	row := d.sql.QueryRowContext(ctx, `SELECT `+agentColumns+` FROM agents WHERE id = ?`, id)
	return scanAgent(row)
}

func (d *DB) ListAgents(ctx context.Context) ([]*Agent, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+agentColumns+` FROM agents ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()
	items := make([]*Agent, 0)
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, agent)
	}
	return items, rows.Err()
}

func (d *DB) UpdateAgent(ctx context.Context, id int64, name, country string, enabled bool) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE agents SET name = ?, country = ?, enabled = ? WHERE id = ?`, name, country, enabled, id)
	if err != nil {
		return fmt.Errorf("update agent: %w", err)
	}
	return nil
}

func (d *DB) DeleteAgent(ctx context.Context, id int64) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM agents WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	return nil
}

func (d *DB) RotateAgentToken(ctx context.Context, id int64, tokenHash string) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE agents SET token_hash = ? WHERE id = ?`, tokenHash, id)
	if err != nil {
		return fmt.Errorf("rotate agent token: %w", err)
	}
	return nil
}

func (d *DB) MarkAgentOnline(ctx context.Context, id int64, version, hostname, platform, arch, dockerVersion, dockerOKError, cpuModel string, cpuCores uint32, connectedAt int64) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE agents SET online = 1, connected_at = ?, last_seen_at = ?, version = ?, hostname = ?, platform = ?, arch = ?, docker_version = ?, docker_ok = ?, docker_error = ?, cpu_model = ?, cpu_cores = ?
		 WHERE id = ?`,
		connectedAt, connectedAt, version, hostname, platform, arch, dockerVersion, dockerOKError == "", dockerOKError, cpuModel, cpuCores, id)
	if err != nil {
		return fmt.Errorf("mark agent online: %w", err)
	}
	return nil
}

func (d *DB) MarkAllAgentsOffline(ctx context.Context) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE agents SET online = 0, latency_ms = 0`)
	if err != nil {
		return fmt.Errorf("mark all agents offline: %w", err)
	}
	return nil
}

func (d *DB) MarkAgentOffline(ctx context.Context, id int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE agents SET online = 0, latency_ms = 0 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("mark agent offline: %w", err)
	}
	return nil
}

func (d *DB) ApplyAgentTelemetry(ctx context.Context, id int64, telemetry AgentTelemetry, seenAt int64) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE agents SET online = 1, last_seen_at = ?, version = ?, hostname = ?, platform = ?, arch = ?,
		 docker_version = ?, docker_ok = ?, docker_error = ?, cpu_model = ?, cpu_cores = ?, cpu_percent = ?,
		 mem_used = ?, mem_total = ?, disk_used = ?, disk_total = ?, net_in_bps = ?, net_out_bps = ?, latency_ms = ?
		 WHERE id = ?`,
		seenAt, telemetry.Version, telemetry.Hostname, telemetry.Platform, telemetry.Arch, telemetry.DockerVersion,
		telemetry.DockerOK, telemetry.DockerError, telemetry.CPUModel, telemetry.CPUCores, telemetry.CPUPercent,
		telemetry.MemUsed, telemetry.MemTotal, telemetry.DiskUsed, telemetry.DiskTotal, telemetry.NetInBPS,
		telemetry.NetOutBPS, telemetry.LatencyMS, id)
	if err != nil {
		return fmt.Errorf("apply agent telemetry: %w", err)
	}
	return nil
}

func (d *DB) SetAgentLatency(ctx context.Context, id int64, latencyMS uint32) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE agents SET latency_ms = ? WHERE id = ?`, latencyMS, id)
	if err != nil {
		return fmt.Errorf("set agent latency: %w", err)
	}
	return nil
}

func (d *DB) CountAgents(ctx context.Context) (total, online int, err error) {
	if err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(online), 0) FROM agents`).Scan(&total, &online); err != nil {
		return 0, 0, fmt.Errorf("count agents: %w", err)
	}
	return total, online, nil
}

func (d *DB) OnlineAgents(ctx context.Context) ([]*Agent, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+agentColumns+` FROM agents WHERE online = 1`)
	if err != nil {
		return nil, fmt.Errorf("online agents: %w", err)
	}
	defer rows.Close()
	items := make([]*Agent, 0)
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, agent)
	}
	return items, rows.Err()
}
