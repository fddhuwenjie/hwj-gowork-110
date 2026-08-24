package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"observatory/internal/apperr"
	"observatory/internal/model"
	"observatory/internal/store/sqlite"
)

// InstrumentRepo 为仪器仓储。
type InstrumentRepo struct{}

// NewInstrumentRepo 创建仪器仓储。
func NewInstrumentRepo() *InstrumentRepo { return &InstrumentRepo{} }

// Create 插入仪器。
func (r *InstrumentRepo) Create(ctx context.Context, q sqlite.Querier, in *model.Instrument, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`INSERT INTO instruments(site_id,code,name,kind,status,temp_min_mk,temp_max_mk,version,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,1,?,?)`,
		in.SiteID, in.Code, in.Name, in.Kind, in.Status, in.TempMinMK, in.TempMaxMK, ts(now), ts(now))
	if err != nil {
		if IsUniqueViolation(err) {
			return apperr.New(apperr.CodeDuplicate, "仪器编码已存在").WithDetail("code", in.Code)
		}
		return err
	}
	in.ID, err = lastID(res)
	in.Version = 1
	in.CreatedAt, in.UpdatedAt = now, now
	return err
}

func scanInstrument(row interface{ Scan(...any) error }) (*model.Instrument, error) {
	var in model.Instrument
	var created, updated string
	err := row.Scan(&in.ID, &in.SiteID, &in.Code, &in.Name, &in.Kind, &in.Status,
		&in.TempMinMK, &in.TempMaxMK, &in.Version, &created, &updated)
	if err != nil {
		return nil, err
	}
	if in.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if in.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &in, nil
}

// Get 按 id 取仪器。
func (r *InstrumentRepo) Get(ctx context.Context, q sqlite.Querier, id int64) (*model.Instrument, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,site_id,code,name,kind,status,temp_min_mk,temp_max_mk,version,created_at,updated_at
		 FROM instruments WHERE id=?`, id)
	in, err := scanInstrument(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("仪器", id)
	}
	return in, err
}

// List 键集分页列出仪器，可按站点与状态过滤。
func (r *InstrumentRepo) List(ctx context.Context, q sqlite.Querier, siteID int64, status string, page Page) ([]model.Instrument, error) {
	page = page.Normalize()
	query := `SELECT id,site_id,code,name,kind,status,temp_min_mk,temp_max_mk,version,created_at,updated_at
		FROM instruments WHERE id > ?`
	args := []any{page.Cursor}
	if siteID > 0 {
		query += ` AND site_id = ?`
		args = append(args, siteID)
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY id LIMIT ?`
	args = append(args, page.Limit)
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Instrument
	for rows.Next() {
		in, err := scanInstrument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *in)
	}
	return out, rows.Err()
}

// Update 乐观锁更新仪器配置。
func (r *InstrumentRepo) Update(ctx context.Context, q sqlite.Querier, in *model.Instrument, now time.Time) error {
	err := execOptimistic(ctx, q, "instruments", "仪器", in.ID,
		`UPDATE instruments SET name=?, kind=?, temp_min_mk=?, temp_max_mk=?, version=version+1, updated_at=?
		 WHERE id=? AND version=?`,
		in.Name, in.Kind, in.TempMinMK, in.TempMaxMK, ts(now), in.ID, in.Version)
	if err != nil {
		return err
	}
	in.Version++
	in.UpdatedAt = now
	return nil
}

// UpdateStatus 乐观锁更新仪器状态。
func (r *InstrumentRepo) UpdateStatus(ctx context.Context, q sqlite.Querier, id, version int64, status string, now time.Time) error {
	return execOptimistic(ctx, q, "instruments", "仪器", id,
		`UPDATE instruments SET status=?, version=version+1, updated_at=? WHERE id=? AND version=?`,
		status, ts(now), id, version)
}

// AddHistory 追加仪器状态历史（不可变）。
func (r *InstrumentRepo) AddHistory(ctx context.Context, q sqlite.Querier, h *model.InstrumentStatusHistory, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`INSERT INTO instrument_status_history(instrument_id,from_status,to_status,reason,actor,created_at)
		 VALUES(?,?,?,?,?,?)`,
		h.InstrumentID, h.FromStatus, h.ToStatus, h.Reason, h.Actor, ts(now))
	if err != nil {
		return err
	}
	h.ID, err = lastID(res)
	h.CreatedAt = now
	return err
}

// ListHistory 按时间序列出仪器状态历史。
func (r *InstrumentRepo) ListHistory(ctx context.Context, q sqlite.Querier, instrumentID int64, page Page) ([]model.InstrumentStatusHistory, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT id,instrument_id,from_status,to_status,reason,actor,created_at
		 FROM instrument_status_history WHERE instrument_id=? AND id > ? ORDER BY id LIMIT ?`,
		instrumentID, page.Cursor, page.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.InstrumentStatusHistory
	for rows.Next() {
		var h model.InstrumentStatusHistory
		var created string
		if err := rows.Scan(&h.ID, &h.InstrumentID, &h.FromStatus, &h.ToStatus, &h.Reason, &h.Actor, &created); err != nil {
			return nil, err
		}
		if h.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
