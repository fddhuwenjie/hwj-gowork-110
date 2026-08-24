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

// CryoRepo 为低温系统、预冷会话与温度读数仓储。
type CryoRepo struct{}

// NewCryoRepo 创建低温仓储。
func NewCryoRepo() *CryoRepo { return &CryoRepo{} }

// CreateSystem 登记低温系统。
func (r *CryoRepo) CreateSystem(ctx context.Context, q sqlite.Querier, c *model.CryoSystem, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`INSERT INTO cryo_systems(instrument_id,name,status,target_temp_mk,version,created_at,updated_at)
		 VALUES(?,?,?,?,1,?,?)`,
		c.InstrumentID, c.Name, c.Status, c.TargetTempMK, ts(now), ts(now))
	if err != nil {
		if IsUniqueViolation(err) {
			return apperr.New(apperr.CodeDuplicate, "该仪器已登记低温系统")
		}
		return err
	}
	c.ID, err = lastID(res)
	c.Version = 1
	c.CreatedAt, c.UpdatedAt = now, now
	return err
}

func scanCryoSystem(row interface{ Scan(...any) error }) (*model.CryoSystem, error) {
	var c model.CryoSystem
	var created, updated string
	err := row.Scan(&c.ID, &c.InstrumentID, &c.Name, &c.Status, &c.TargetTempMK,
		&c.Version, &created, &updated)
	if err != nil {
		return nil, err
	}
	if c.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if c.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &c, nil
}

// GetSystem 按 id 取低温系统。
func (r *CryoRepo) GetSystem(ctx context.Context, q sqlite.Querier, id int64) (*model.CryoSystem, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,instrument_id,name,status,target_temp_mk,version,created_at,updated_at
		 FROM cryo_systems WHERE id=?`, id)
	c, err := scanCryoSystem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("低温系统", id)
	}
	return c, err
}

// GetSystemByInstrument 按仪器取低温系统。
func (r *CryoRepo) GetSystemByInstrument(ctx context.Context, q sqlite.Querier, instrumentID int64) (*model.CryoSystem, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,instrument_id,name,status,target_temp_mk,version,created_at,updated_at
		 FROM cryo_systems WHERE instrument_id=?`, instrumentID)
	c, err := scanCryoSystem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("低温系统(instrument)", instrumentID)
	}
	return c, err
}

// UpdateSystemStatus 乐观锁更新低温系统状态。
func (r *CryoRepo) UpdateSystemStatus(ctx context.Context, q sqlite.Querier, id, version int64, status string, now time.Time) error {
	return execOptimistic(ctx, q, "cryo_systems", "低温系统", id,
		`UPDATE cryo_systems SET status=?, version=version+1, updated_at=? WHERE id=? AND version=?`,
		status, ts(now), id, version)
}

// CreateSession 创建预冷会话。
func (r *CryoRepo) CreateSession(ctx context.Context, q sqlite.Querier, s *model.PrecoolSession, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`INSERT INTO precool_sessions(cryo_system_id,status,target_temp_mk,started_at,deadline_at,finished_at,version,created_at,updated_at)
		 VALUES(?,?,?,?,?,NULL,1,?,?)`,
		s.CryoSystemID, s.Status, s.TargetTempMK, ts(s.StartedAt), ts(s.DeadlineAt), ts(now), ts(now))
	if err != nil {
		return err
	}
	s.ID, err = lastID(res)
	s.Version = 1
	s.CreatedAt, s.UpdatedAt = now, now
	return err
}

// GetSession 按 id 取预冷会话。
func (r *CryoRepo) GetSession(ctx context.Context, q sqlite.Querier, id int64) (*model.PrecoolSession, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,cryo_system_id,status,target_temp_mk,started_at,deadline_at,finished_at,version,created_at,updated_at
		 FROM precool_sessions WHERE id=?`, id)
	return scanSession(row)
}

func scanSession(row interface{ Scan(...any) error }) (*model.PrecoolSession, error) {
	var s model.PrecoolSession
	var started, deadline, created, updated string
	var finished *string
	err := row.Scan(&s.ID, &s.CryoSystemID, &s.Status, &s.TargetTempMK,
		&started, &deadline, &finished, &s.Version, &created, &updated)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("预冷会话", nil)
		}
		return nil, err
	}
	if s.StartedAt, err = parseTime(started); err != nil {
		return nil, err
	}
	if s.DeadlineAt, err = parseTime(deadline); err != nil {
		return nil, err
	}
	if s.FinishedAt, err = parseTimePtr(finished); err != nil {
		return nil, err
	}
	if s.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if s.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &s, nil
}

// FinishSession 乐观锁结束预冷会话（转稳/超时/中止）。
func (r *CryoRepo) FinishSession(ctx context.Context, q sqlite.Querier, id, version int64, status string, now time.Time) error {
	return execOptimistic(ctx, q, "precool_sessions", "预冷会话", id,
		`UPDATE precool_sessions SET status=?, finished_at=?, version=version+1, updated_at=?
		 WHERE id=? AND version=?`,
		status, ts(now), ts(now), id, version)
}

// ListSessions 分页列出预冷会话。
func (r *CryoRepo) ListSessions(ctx context.Context, q sqlite.Querier, cryoID int64, page Page) ([]model.PrecoolSession, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT id,cryo_system_id,status,target_temp_mk,started_at,deadline_at,finished_at,version,created_at,updated_at
		 FROM precool_sessions WHERE cryo_system_id=? AND id > ? ORDER BY id LIMIT ?`,
		cryoID, page.Cursor, page.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.PrecoolSession
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// InsertReading 插入温度读数；幂等键冲突时返回已存在读数且 replay=true。
func (r *CryoRepo) InsertReading(ctx context.Context, q sqlite.Querier, rd *model.CryoReading, now time.Time) (replay bool, err error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO cryo_readings(cryo_system_id,temp_mk,pressure_mbar,recorded_at,idempotency_key,created_at)
		 VALUES(?,?,?,?,?,?)`,
		rd.CryoSystemID, rd.TempMK, rd.PressureMbar, ts(rd.RecordedAt), rd.IdempotencyKey, ts(now))
	if err == nil {
		rd.ID, err = lastID(res)
		rd.CreatedAt = now
		return false, err
	}
	if !IsUniqueViolation(err) {
		return false, err
	}
	existing, gerr := r.GetReadingByKey(ctx, q, rd.CryoSystemID, rd.IdempotencyKey)
	if gerr != nil {
		return false, gerr
	}
	*rd = *existing
	return true, nil
}

// GetReadingByKey 按幂等键取读数。
func (r *CryoRepo) GetReadingByKey(ctx context.Context, q sqlite.Querier, cryoID int64, key string) (*model.CryoReading, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,cryo_system_id,temp_mk,pressure_mbar,recorded_at,idempotency_key,created_at
		 FROM cryo_readings WHERE cryo_system_id=? AND idempotency_key=?`, cryoID, key)
	return scanReading(row)
}

func scanReading(row interface{ Scan(...any) error }) (*model.CryoReading, error) {
	var rd model.CryoReading
	var recorded, created string
	err := row.Scan(&rd.ID, &rd.CryoSystemID, &rd.TempMK, &rd.PressureMbar,
		&recorded, &rd.IdempotencyKey, &created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("温度读数", nil)
		}
		return nil, err
	}
	if rd.RecordedAt, err = parseTime(recorded); err != nil {
		return nil, err
	}
	if rd.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	return &rd, nil
}

// LatestReading 取低温系统最新一条读数。
func (r *CryoRepo) LatestReading(ctx context.Context, q sqlite.Querier, cryoID int64) (*model.CryoReading, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,cryo_system_id,temp_mk,pressure_mbar,recorded_at,idempotency_key,created_at
		 FROM cryo_readings WHERE cryo_system_id=? ORDER BY recorded_at DESC, id DESC LIMIT 1`, cryoID)
	return scanReading(row)
}

// ListReadings 分页列出读数。
func (r *CryoRepo) ListReadings(ctx context.Context, q sqlite.Querier, cryoID int64, page Page) ([]model.CryoReading, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT id,cryo_system_id,temp_mk,pressure_mbar,recorded_at,idempotency_key,created_at
		 FROM cryo_readings WHERE cryo_system_id=? AND id > ? ORDER BY id LIMIT ?`,
		cryoID, page.Cursor, page.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CryoReading
	for rows.Next() {
		rd, err := scanReading(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rd)
	}
	return out, rows.Err()
}
