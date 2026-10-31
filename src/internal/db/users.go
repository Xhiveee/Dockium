package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func (d *DB) SetupInitialized(ctx context.Context) (bool, error) {
	var initialized bool
	if err := d.sql.QueryRowContext(ctx, `SELECT initialized FROM panel_state WHERE id = 1`).Scan(&initialized); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("read setup state: %w", err)
	}
	return initialized, nil
}

func (d *DB) CreateFirstAdmin(ctx context.Context, login, passwordHash string) (*User, error) {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin setup: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE panel_state SET initialized = 1 WHERE id = 1 AND initialized = 0`)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("mark setup: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		tx.Rollback()
		return nil, ErrConflict
	}
	now := Now()
	insert, err := tx.ExecContext(ctx,
		`INSERT INTO users (login, password_hash, role, created_at) VALUES (?, ?, 'admin', ?)`, login, passwordHash, now)
	if err != nil {
		tx.Rollback()
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("create admin: %w", err)
	}
	id, err := insert.LastInsertId()
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("create admin id: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit setup: %w", err)
	}
	return &User{ID: id, Login: login, PasswordHash: passwordHash, Role: RoleAdmin, CreatedAt: now}, nil
}

func (d *DB) CountAdmins(ctx context.Context) (int, error) {
	var count int
	if err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count admins: %w", err)
	}
	return count, nil
}

func (d *DB) CountUsers(ctx context.Context) (int, error) {
	var count int
	if err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

func (d *DB) CreateUser(ctx context.Context, login, passwordHash, role string) (*User, error) {
	now := Now()
	result, err := d.sql.ExecContext(ctx,
		`INSERT INTO users (login, password_hash, role, created_at) VALUES (?, ?, ?, ?)`,
		login, passwordHash, role, now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create user id: %w", err)
	}
	return &User{ID: id, Login: login, PasswordHash: passwordHash, Role: role, CreatedAt: now}, nil
}

func (d *DB) UserByLogin(ctx context.Context, login string) (*User, error) {
	row := d.sql.QueryRowContext(ctx,
		`SELECT id, login, password_hash, role, created_at, last_login_at FROM users WHERE login = ? COLLATE NOCASE`, login)
	var user User
	if err := row.Scan(&user.ID, &user.Login, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.LastLoginAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get user by login: %w", err)
	}
	return &user, nil
}

func (d *DB) UserByID(ctx context.Context, id int64) (*User, error) {
	row := d.sql.QueryRowContext(ctx,
		`SELECT id, login, password_hash, role, created_at, last_login_at FROM users WHERE id = ?`, id)
	var user User
	if err := row.Scan(&user.ID, &user.Login, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.LastLoginAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &user, nil
}

func (d *DB) ListUsers(ctx context.Context, limit, offset int) ([]*User, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT id, login, password_hash, role, created_at, last_login_at FROM users ORDER BY id LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	users := make([]*User, 0, limit)
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Login, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.LastLoginAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, &user)
	}
	return users, rows.Err()
}

func (d *DB) UpdateUser(ctx context.Context, id int64, role string, passwordHash *string) error {
	if passwordHash != nil {
		_, err := d.sql.ExecContext(ctx, `UPDATE users SET role = ?, password_hash = ? WHERE id = ?`, role, *passwordHash, id)
		if err != nil {
			return fmt.Errorf("update user: %w", err)
		}
		return nil
	}
	_, err := d.sql.ExecContext(ctx, `UPDATE users SET role = ? WHERE id = ?`, role, id)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

func (d *DB) UpdateLogin(ctx context.Context, id int64, login string) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE users SET login = ? WHERE id = ?`, login, id)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("update login: %w", err)
	}
	return nil
}

func (d *DB) DeleteUser(ctx context.Context, id int64) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func (d *DB) MarkLogin(ctx context.Context, id int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, Now(), id)
	if err != nil {
		return fmt.Errorf("mark login: %w", err)
	}
	return nil
}

func (d *DB) CreateSession(ctx context.Context, session *Session) error {
	_, err := d.sql.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, created_at, last_seen_at, expires_at, ip, user_agent) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.UserID, session.CreatedAt, session.LastSeenAt, session.ExpiresAt, session.IP, session.UserAgent)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (d *DB) SessionByID(ctx context.Context, id string) (*Session, error) {
	row := d.sql.QueryRowContext(ctx,
		`SELECT id, user_id, created_at, last_seen_at, expires_at, ip, user_agent FROM sessions WHERE id = ?`, id)
	var session Session
	if err := row.Scan(&session.ID, &session.UserID, &session.CreatedAt, &session.LastSeenAt, &session.ExpiresAt, &session.IP, &session.UserAgent); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get session: %w", err)
	}
	return &session, nil
}

func (d *DB) TouchSession(ctx context.Context, id string, now int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

type SessionRow struct {
	Session
	Login string
}

func (d *DB) ListSessions(ctx context.Context, now int64) ([]*SessionRow, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT s.id, s.user_id, s.created_at, s.last_seen_at, s.expires_at, s.ip, s.user_agent, u.login
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.expires_at > ? ORDER BY s.last_seen_at DESC`, now)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	items := make([]*SessionRow, 0)
	for rows.Next() {
		var item SessionRow
		if err := rows.Scan(&item.ID, &item.UserID, &item.CreatedAt, &item.LastSeenAt, &item.ExpiresAt, &item.IP, &item.UserAgent, &item.Login); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		items = append(items, &item)
	}
	return items, rows.Err()
}

func (d *DB) DeleteSession(ctx context.Context, id string) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (d *DB) DeleteUserSessions(ctx context.Context, userID int64, except string) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id <> ?`, userID, except)
	if err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

func (d *DB) PurgeExpiredSessions(ctx context.Context, now int64) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now)
	if err != nil {
		return fmt.Errorf("purge sessions: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
