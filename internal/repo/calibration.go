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

// CalibrationRepo 为校准方案与校准记录仓储。
type CalibrationRepo struct{}

// NewCalibrationRepo 创建校准仓储。
func NewCalibrationRepo() *CalibrationRepo { return &CalibrationRepo{} }

// CreatePlan 插入校准方案（草稿）。
func (r *CalibrationRepo) CreatePlan(ctx context.Context, q sqlite.Querier, p *model.CalibrationPlan, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`INSERT INTO calibration_plans(instrument_id,version_no,params,status,valid_from,valid_until,created_by,approved_by,version,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,'',1,?,?)`,
		p.InstrumentID, p.VersionNo, p.Params, p.Status, ts(p.ValidFrom), ts(p.ValidUntil), p.CreatedBy, ts(now), ts(now))
	if err != nil {
		if IsUniqueViolation(err) {
			return apperr.New(apperr.CodeDuplicate, "该仪器下方案版本号已存在").
				WithDetail("version_no", p.VersionNo)
		}
		return err
	}
	p.ID, err = lastID(res)
	p.Version = 1
	p.CreatedAt, p.UpdatedAt = now, now
	return err
}

func scanPlan(row interface{ Scan(...any) error }) (*model.CalibrationPlan, error) {
	var p model.CalibrationPlan
	var validFrom, validUntil, created, updated string
	err := row.Scan(&p.ID, &p.InstrumentID, &p.VersionNo, &p.Params, &p.Status,
		&validFrom, &validUntil, &p.CreatedBy, &p.ApprovedBy, &p.Version, &created, &updated)
	if err != nil {
		return nil, err
	}
	if p.ValidFrom, err = parseTime(validFrom); err != nil {
		return nil, err
	}
	if p.ValidUntil, err = parseTime(validUntil); err != nil {
		return nil, err
	}
	if p.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if p.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetPlan 按 id 取方案。
func (r *CalibrationRepo) GetPlan(ctx context.Context, q sqlite.Querier, id int64) (*model.CalibrationPlan, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,instrument_id,version_no,params,status,valid_from,valid_until,created_by,approved_by,version,created_at,updated_at
		 FROM calibration_plans WHERE id=?`, id)
	p, err := scanPlan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("校准方案", id)
	}
	return p, err
}

// GetActivePlan 取仪器当前启用方案，无则返回 not_found。
func (r *CalibrationRepo) GetActivePlan(ctx context.Context, q sqlite.Querier, instrumentID int64) (*model.CalibrationPlan, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,instrument_id,version_no,params,status,valid_from,valid_until,created_by,approved_by,version,created_at,updated_at
		 FROM calibration_plans WHERE instrument_id=? AND status='active' ORDER BY id DESC LIMIT 1`, instrumentID)
	p, err := scanPlan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("启用中的校准方案", instrumentID)
	}
	return p, err
}

// ListPlans 分页列出仪器方案。
func (r *CalibrationRepo) ListPlans(ctx context.Context, q sqlite.Querier, instrumentID int64, page Page) ([]model.CalibrationPlan, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT id,instrument_id,version_no,params,status,valid_from,valid_until,created_by,approved_by,version,created_at,updated_at
		 FROM calibration_plans WHERE instrument_id=? AND id > ? ORDER BY id LIMIT ?`,
		instrumentID, page.Cursor, page.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CalibrationPlan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// ApprovePlan 乐观锁审批方案。
func (r *CalibrationRepo) ApprovePlan(ctx context.Context, q sqlite.Querier, id, version int64, approver, status string, now time.Time) error {
	return execOptimistic(ctx, q, "calibration_plans", "校准方案", id,
		`UPDATE calibration_plans SET status=?, approved_by=?, version=version+1, updated_at=?
		 WHERE id=? AND version=?`,
		status, approver, ts(now), id, version)
}

// UpdatePlanStatus 乐观锁更新方案状态。
func (r *CalibrationRepo) UpdatePlanStatus(ctx context.Context, q sqlite.Querier, id, version int64, status string, now time.Time) error {
	return execOptimistic(ctx, q, "calibration_plans", "校准方案", id,
		`UPDATE calibration_plans SET status=?, version=version+1, updated_at=? WHERE id=? AND version=?`,
		status, ts(now), id, version)
}

// SupersedeActivePlans 将仪器当前 active 方案置为 superseded（启用新方案时同事务调用）。
func (r *CalibrationRepo) SupersedeActivePlans(ctx context.Context, q sqlite.Querier, instrumentID, exceptPlanID int64, now time.Time) error {
	_, err := q.ExecContext(ctx,
		`UPDATE calibration_plans SET status='superseded', version=version+1, updated_at=?
		 WHERE instrument_id=? AND status='active' AND id<>?`, ts(now), instrumentID, exceptPlanID)
	return err
}

// CreateRecord 追加校准记录（不可变证据）。
func (r *CalibrationRepo) CreateRecord(ctx context.Context, q sqlite.Querier, rec *model.CalibrationRecord, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`INSERT INTO calibration_records(plan_id,source_id,instrument_id,result,deviation_pct,evidence_uri,performed_by,performed_at,created_at)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		rec.PlanID, rec.SourceID, rec.InstrumentID, rec.Result, rec.DeviationPct,
		rec.EvidenceURI, rec.PerformedBy, ts(rec.PerformedAt), ts(now))
	if err != nil {
		return err
	}
	rec.ID, err = lastID(res)
	rec.CreatedAt = now
	return err
}

func scanRecord(row interface{ Scan(...any) error }) (*model.CalibrationRecord, error) {
	var rec model.CalibrationRecord
	var performed, created string
	err := row.Scan(&rec.ID, &rec.PlanID, &rec.SourceID, &rec.InstrumentID, &rec.Result,
		&rec.DeviationPct, &rec.EvidenceURI, &rec.PerformedBy, &performed, &created)
	if err != nil {
		return nil, err
	}
	if rec.PerformedAt, err = parseTime(performed); err != nil {
		return nil, err
	}
	if rec.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	return &rec, nil
}

// GetRecord 按 id 取校准记录。
func (r *CalibrationRepo) GetRecord(ctx context.Context, q sqlite.Querier, id int64) (*model.CalibrationRecord, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,plan_id,source_id,instrument_id,result,deviation_pct,evidence_uri,performed_by,performed_at,created_at
		 FROM calibration_records WHERE id=?`, id)
	rec, err := scanRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("校准记录", id)
	}
	return rec, err
}

// ListRecords 分页列出仪器校准记录。
func (r *CalibrationRepo) ListRecords(ctx context.Context, q sqlite.Querier, instrumentID int64, page Page) ([]model.CalibrationRecord, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT id,plan_id,source_id,instrument_id,result,deviation_pct,evidence_uri,performed_by,performed_at,created_at
		 FROM calibration_records WHERE instrument_id=? AND id > ? ORDER BY id LIMIT ?`,
		instrumentID, page.Cursor, page.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CalibrationRecord
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

// LatestPassingRecord 取指定方案下不晚于 at 执行的最新合格校准记录。
func (r *CalibrationRepo) LatestPassingRecord(ctx context.Context, q sqlite.Querier, planID int64, at time.Time) (*model.CalibrationRecord, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,plan_id,source_id,instrument_id,result,deviation_pct,evidence_uri,performed_by,performed_at,created_at
		 FROM calibration_records
		 WHERE plan_id=? AND result='pass' AND performed_at<=?
		 ORDER BY performed_at DESC, id DESC LIMIT 1`, planID, ts(at))
	rec, err := scanRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("合格校准记录", planID)
	}
	return rec, err
}
