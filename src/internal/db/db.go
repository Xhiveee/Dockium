package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	sql *sql.DB
}

func Open(ctx context.Context, path string) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)", path)
	handle, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	handle.SetMaxOpenConns(4)
	handle.SetMaxIdleConns(4)
	handle.SetConnMaxLifetime(time.Hour)
	if err := handle.PingContext(ctx); err != nil {
		handle.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &DB{sql: handle}, nil
}

func (d *DB) Close() error {
	return d.sql.Close()
}

func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.sql.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}
	var current int
	if err := d.sql.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	for index, statement := range migrations {
		version := index + 1
		if version <= current {
			continue
		}
		if err := d.applyMigration(ctx, version, statement); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) applyMigration(ctx context.Context, version int, statement string) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx, statement); err != nil {
		tx.Rollback()
		return fmt.Errorf("apply migration %d: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, version, time.Now().UnixMilli()); err != nil {
		tx.Rollback()
		return fmt.Errorf("record migration %d: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", version, err)
	}
	return nil
}

func (d *DB) Health(ctx context.Context) error {
	if err := d.sql.PingContext(ctx); err != nil {
		return err
	}
	return nil
}

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

var migrations = []string{
	`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		login TEXT NOT NULL UNIQUE COLLATE NOCASE,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user')),
		created_at INTEGER NOT NULL,
		last_login_at INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE sessions (
		id TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at INTEGER NOT NULL,
		last_seen_at INTEGER NOT NULL,
		expires_at INTEGER NOT NULL,
		ip TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX idx_sessions_user ON sessions(user_id)`,
	`CREATE INDEX idx_sessions_expires ON sessions(expires_at)`,
	`CREATE TABLE login_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		login TEXT NOT NULL DEFAULT '',
		success INTEGER NOT NULL,
		reason TEXT NOT NULL DEFAULT '',
		ip TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT '',
		browser TEXT NOT NULL DEFAULT '',
		os TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	)`,
	`CREATE INDEX idx_login_logs_created ON login_logs(created_at DESC)`,
	`CREATE INDEX idx_login_logs_ip ON login_logs(ip, created_at DESC)`,
	`CREATE TABLE ip_bans (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ip TEXT NOT NULL UNIQUE,
		reason TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'manual',
		banned_at INTEGER NOT NULL,
		expires_at INTEGER NOT NULL
	)`,
	`CREATE INDEX idx_ip_bans_expires ON ip_bans(expires_at)`,
	`CREATE TABLE api_requests (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		login TEXT NOT NULL DEFAULT '',
		procedure TEXT NOT NULL,
		status INTEGER NOT NULL DEFAULT 0,
		ip TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	)`,
	`CREATE INDEX idx_api_requests_created ON api_requests(created_at DESC)`,
	`CREATE TABLE agents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		country TEXT NOT NULL DEFAULT '',
		token_hash TEXT NOT NULL UNIQUE,
		version TEXT NOT NULL DEFAULT '',
		hostname TEXT NOT NULL DEFAULT '',
		platform TEXT NOT NULL DEFAULT '',
		arch TEXT NOT NULL DEFAULT '',
		docker_version TEXT NOT NULL DEFAULT '',
		docker_ok INTEGER NOT NULL DEFAULT 0,
		docker_error TEXT NOT NULL DEFAULT '',
		cpu_model TEXT NOT NULL DEFAULT '',
		cpu_cores INTEGER NOT NULL DEFAULT 0,
		cpu_percent REAL NOT NULL DEFAULT 0,
		mem_used INTEGER NOT NULL DEFAULT 0,
		mem_total INTEGER NOT NULL DEFAULT 0,
		disk_used INTEGER NOT NULL DEFAULT 0,
		disk_total INTEGER NOT NULL DEFAULT 0,
		net_in_bps INTEGER NOT NULL DEFAULT 0,
		net_out_bps INTEGER NOT NULL DEFAULT 0,
		latency_ms INTEGER NOT NULL DEFAULT 0,
		online INTEGER NOT NULL DEFAULT 0,
		last_seen_at INTEGER NOT NULL DEFAULT 0,
		connected_at INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL
	)`,
	`CREATE TABLE instances (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE COLLATE NOCASE,
		agent_id INTEGER NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
		status TEXT NOT NULL DEFAULT 'stopped' CHECK (status IN ('stopped', 'starting', 'running', 'stopping', 'exited', 'failed')),
		image TEXT NOT NULL DEFAULT '',
		container_name TEXT NOT NULL DEFAULT '',
		container_id TEXT NOT NULL DEFAULT '',
		command TEXT NOT NULL DEFAULT '[]',
		environment TEXT NOT NULL DEFAULT '[]',
		ports TEXT NOT NULL DEFAULT '[]',
		volumes TEXT NOT NULL DEFAULT '[]',
		network_mode TEXT NOT NULL DEFAULT 'bridge',
		restart_policy TEXT NOT NULL DEFAULT 'no',
		memory_mb INTEGER NOT NULL DEFAULT 0,
		cpus REAL NOT NULL DEFAULT 0,
		tty INTEGER NOT NULL DEFAULT 1,
		data_mount TEXT NOT NULL DEFAULT '/data',
		console_encoding TEXT NOT NULL DEFAULT 'utf-8',
		auto_start INTEGER NOT NULL DEFAULT 0,
		auto_restart INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		exit_code INTEGER NOT NULL DEFAULT 0,
		last_started_at INTEGER NOT NULL DEFAULT 0,
		cpu_percent REAL NOT NULL DEFAULT 0,
		mem_bytes INTEGER NOT NULL DEFAULT 0,
		mem_limit_bytes INTEGER NOT NULL DEFAULT 0,
		net_in_bps INTEGER NOT NULL DEFAULT 0,
		net_out_bps INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	)`,
	`CREATE INDEX idx_instances_agent ON instances(agent_id)`,
	`CREATE TABLE user_instances (
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
		control INTEGER NOT NULL DEFAULT 1,
		console INTEGER NOT NULL DEFAULT 1,
		files INTEGER NOT NULL DEFAULT 1,
		PRIMARY KEY (user_id, instance_id)
	)`,
	`CREATE TABLE scheduled_tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		action TEXT NOT NULL,
		payload TEXT NOT NULL DEFAULT '',
		kind TEXT NOT NULL CHECK (kind IN ('interval', 'daily')),
		interval_minutes INTEGER NOT NULL DEFAULT 0,
		time_of_day TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		last_run_at INTEGER NOT NULL DEFAULT 0,
		next_run_at INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	)`,
	`CREATE INDEX idx_tasks_instance ON scheduled_tasks(instance_id)`,
	`CREATE INDEX idx_tasks_due ON scheduled_tasks(enabled, next_run_at)`,
	`CREATE TABLE panel_state (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		initialized INTEGER NOT NULL DEFAULT 0
	)`,
	`INSERT INTO panel_state (id, initialized) VALUES (1, 0)`,
	`CREATE TABLE panel_settings (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		max_upload_mb INTEGER NOT NULL DEFAULT 1024 CHECK (max_upload_mb BETWEEN 1 AND 4096)
	)`,
	`INSERT INTO panel_settings (id, max_upload_mb) VALUES (1, 1024)`,
}
