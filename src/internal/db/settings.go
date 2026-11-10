package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func (d *DB) PanelSettings(ctx context.Context) (*PanelSettings, error) {
	row := d.sql.QueryRowContext(ctx, `SELECT max_upload_mb FROM panel_settings WHERE id = 1`)
	var settings PanelSettings
	if err := row.Scan(&settings.MaxUploadMB); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &PanelSettings{MaxUploadMB: 1024}, nil
		}
		return nil, fmt.Errorf("read panel settings: %w", err)
	}
	return &settings, nil
}

func (d *DB) UpdatePanelSettings(ctx context.Context, settings *PanelSettings) error {
	if settings == nil {
		return errors.New("panel settings are required")
	}
	if settings.MaxUploadMB < 1 || settings.MaxUploadMB > 4096 {
		return errors.New("max upload size must be between 1 and 4096 MB")
	}
	if _, err := d.sql.ExecContext(ctx, `UPDATE panel_settings SET max_upload_mb = ? WHERE id = 1`, settings.MaxUploadMB); err != nil {
		return fmt.Errorf("update panel settings: %w", err)
	}
	return nil
}
