package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func (d *DB) InsertLoginLog(ctx context.Context, entry *LoginLog) (int64, error) {
	result, err := d.sql.ExecContext(ctx,
		`INSERT INTO login_logs (login, success, reason, ip, user_agent, browser, os, country, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.Login, entry.Success, entry.Reason, entry.IP, entry.UserAgent, entry.Browser, entry.OS, entry.Country, entry.CreatedAt)
	if err != nil {
		return 0, fmt.Errorf("insert login log: %w", err)
	}
	return result.LastInsertId()
}

func (d *DB) ListLoginLogs(ctx context.Context, limit, offset int) ([]*LoginLog, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT id, login, success, reason, ip, user_agent, browser, os, country, created_at
		 FROM login_logs ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list login logs: %w", err)
	}
	defer rows.Close()
	items := make([]*LoginLog, 0, limit)
	for rows.Next() {
		var item LoginLog
		if err := rows.Scan(&item.ID, &item.Login, &item.Success, &item.Reason, &item.IP, &item.UserAgent, &item.Browser, &item.OS, &item.Country, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan login log: %w", err)
		}
		items = append(items, &item)
	}
	return items, rows.Err()
}

func (d *DB) UpdateLoginLogCountry(ctx context.Context, id int64, country string) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE login_logs SET country = ? WHERE id = ?`, country, id)
	if err != nil {
		return fmt.Errorf("update login log country: %w", err)
	}
	return nil
}

func (d *DB) CountLoginLogs(ctx context.Context) (int, error) {
	var count int
	if err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM login_logs`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count login logs: %w", err)
	}
	return count, nil
}

func (d *DB) CountFailedLogins(ctx context.Context, ip string, since int64) (int, error) {
	var count int
	if err := d.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM login_logs WHERE ip = ? AND success = 0 AND created_at >= ?`, ip, since).Scan(&count); err != nil {
		return 0, fmt.Errorf("count failed logins: %w", err)
	}
	return count, nil
}

func (d *DB) ActiveBan(ctx context.Context, ip string, now int64) (*Ban, error) {
	row := d.sql.QueryRowContext(ctx,
		`SELECT id, ip, reason, source, country, banned_at, expires_at FROM ip_bans WHERE ip = ? AND expires_at > ?`, ip, now)
	var ban Ban
	if err := row.Scan(&ban.ID, &ban.IP, &ban.Reason, &ban.Source, &ban.Country, &ban.BannedAt, &ban.ExpiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("active ban: %w", err)
	}
	return &ban, nil
}

func (d *DB) CreateBan(ctx context.Context, ban *Ban) (*Ban, error) {
	_, err := d.sql.ExecContext(ctx,
		`INSERT INTO ip_bans (ip, reason, source, country, banned_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(ip) DO UPDATE SET reason = excluded.reason, source = excluded.source,
		 country = excluded.country, banned_at = excluded.banned_at, expires_at = excluded.expires_at`,
		ban.IP, ban.Reason, ban.Source, ban.Country, ban.BannedAt, ban.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("create ban: %w", err)
	}
	var stored Ban
	if err := d.sql.QueryRowContext(ctx,
		`SELECT id, ip, reason, source, country, banned_at, expires_at FROM ip_bans WHERE ip = ?`, ban.IP).
		Scan(&stored.ID, &stored.IP, &stored.Reason, &stored.Source, &stored.Country, &stored.BannedAt, &stored.ExpiresAt); err != nil {
		return nil, fmt.Errorf("read ban: %w", err)
	}
	return &stored, nil
}

func (d *DB) ListBans(ctx context.Context, now int64) ([]*Ban, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT id, ip, reason, source, country, banned_at, expires_at FROM ip_bans WHERE expires_at > ? ORDER BY id DESC`, now)
	if err != nil {
		return nil, fmt.Errorf("list bans: %w", err)
	}
	defer rows.Close()
	items := make([]*Ban, 0)
	for rows.Next() {
		var item Ban
		if err := rows.Scan(&item.ID, &item.IP, &item.Reason, &item.Source, &item.Country, &item.BannedAt, &item.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan ban: %w", err)
		}
		items = append(items, &item)
	}
	return items, rows.Err()
}

func (d *DB) CountBans(ctx context.Context, now int64) (int, error) {
	var count int
	if err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM ip_bans WHERE expires_at > ?`, now).Scan(&count); err != nil {
		return 0, fmt.Errorf("count bans: %w", err)
	}
	return count, nil
}

func (d *DB) UpdateBanCountry(ctx context.Context, id int64, country string) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE ip_bans SET country = ? WHERE id = ?`, country, id)
	if err != nil {
		return fmt.Errorf("update ban country: %w", err)
	}
	return nil
}

func (d *DB) DeleteBan(ctx context.Context, id int64) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM ip_bans WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete ban: %w", err)
	}
	return nil
}

func (d *DB) InsertApiRequest(ctx context.Context, entry *ApiRequest) (int64, error) {
	result, err := d.sql.ExecContext(ctx,
		`INSERT INTO api_requests (login, procedure, status, ip, created_at) VALUES (?, ?, ?, ?, ?)`,
		entry.Login, entry.Procedure, entry.Status, entry.IP, entry.CreatedAt)
	if err != nil {
		return 0, fmt.Errorf("insert api request: %w", err)
	}
	return result.LastInsertId()
}

func (d *DB) ListApiRequests(ctx context.Context, limit, offset int) ([]*ApiRequest, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT id, login, procedure, status, ip, created_at FROM api_requests ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list api requests: %w", err)
	}
	defer rows.Close()
	items := make([]*ApiRequest, 0, limit)
	for rows.Next() {
		var item ApiRequest
		if err := rows.Scan(&item.ID, &item.Login, &item.Procedure, &item.Status, &item.IP, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan api request: %w", err)
		}
		items = append(items, &item)
	}
	return items, rows.Err()
}

func (d *DB) CountApiRequests(ctx context.Context) (int, error) {
	var count int
	if err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_requests`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count api requests: %w", err)
	}
	return count, nil
}

func (d *DB) PurgeSecurityLogs(ctx context.Context, loginBefore, apiBefore int64) error {
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM login_logs WHERE created_at < ?`, loginBefore); err != nil {
		return fmt.Errorf("purge login logs: %w", err)
	}
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM api_requests WHERE created_at < ?`, apiBefore); err != nil {
		return fmt.Errorf("purge api requests: %w", err)
	}
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM ip_bans WHERE expires_at <= ?`, loginBefore); err != nil {
		return fmt.Errorf("purge bans: %w", err)
	}
	return nil
}
