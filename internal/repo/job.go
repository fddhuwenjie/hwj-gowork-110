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

// JobRepo 为后台作业仓储；作业持久化以支持重启恢复与失败重试。
type JobRepo struct{}

// NewJobRepo 创建作业仓储。
func NewJobRepo() *JobRepo { return &JobRepo{} }

// Enqueue 排程作业；相同 (type,payload) 已存在时忽略（防重复排程）。
func (r *JobRepo) Enqueue(ctx context.Context, q sqlite.Querier, jobType, payload string,
	runAt time.Time, maxAttempts int, now time.Time) (int64, error) {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	res, err := q.ExecContext(ctx,
		`INSERT INTO jobs(type,payload,status,attempts,max_attempts,run_at,last_error,created_at,updated_at)
		 VALUES(?,?,'pending',0,?,?,'',?,?)`,
		jobType, payload, maxAttempts, ts(runAt), ts(now), ts(now))
	if err != nil {
		if IsUniqueViolation(err) {
			return 0, nil
		}
		return 0, err
	}
	return lastID(res)
}

func scanJob(row interface{ Scan(...any) error }) (*model.Job, error) {
	var j model.Job
	var runAt, created, updated string
	err := row.Scan(&j.ID, &j.Type, &j.Payload, &j.Status, &j.Attempts, &j.MaxAttempts,
		&runAt, &j.LastError, &created, &updated)
	if err != nil {
		return nil, err
	}
	if j.RunAt, err = parseTime(runAt); err != nil {
		return nil, err
	}
	if j.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if j.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &j, nil
}

const jobCols = `id,type,payload,status,attempts,max_attempts,run_at,last_error,created_at,updated_at`

// Get 按 id 取作业。
func (r *JobRepo) Get(ctx context.Context, q sqlite.Querier, id int64) (*model.Job, error) {
	row := q.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE id=?`, id)
	j, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("作业", id)
	}
	return j, err
}

// List 分页列出作业，可按类型与状态过滤。
func (r *JobRepo) List(ctx context.Context, q sqlite.Querier, jobType, status string, page Page) ([]model.Job, error) {
	page = page.Normalize()
	query := `SELECT ` + jobCols + ` FROM jobs WHERE id > ?`
	args := []any{page.Cursor}
	if jobType != "" {
		query += ` AND type = ?`
		args = append(args, jobType)
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
	var out []model.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

// ClaimDue 在调用方事务内认领一个到期作业：pending/failed(未超次数) 且 run_at<=now。
// 无到期作业时返回 nil, nil。
func (r *JobRepo) ClaimDue(ctx context.Context, q sqlite.Querier, now time.Time) (*model.Job, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+jobCols+` FROM jobs
		 WHERE run_at<=? AND (status='pending' OR (status='failed' AND attempts<max_attempts))
		 ORDER BY run_at, id LIMIT 1`, ts(now))
	j, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	res, err := q.ExecContext(ctx,
		`UPDATE jobs SET status='running', attempts=attempts+1, updated_at=?
		 WHERE id=? AND status=? AND attempts=?`,
		ts(now), j.ID, j.Status, j.Attempts)
	if err != nil {
		return nil, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, apperr.Conflict("作业已被其他执行者认领").WithDetail("job_id", j.ID)
	}
	j.Status = "running"
	j.Attempts++
	return j, nil
}

// MarkDone 标记作业完成。
func (r *JobRepo) MarkDone(ctx context.Context, q sqlite.Querier, id int64, now time.Time) error {
	_, err := q.ExecContext(ctx,
		`UPDATE jobs SET status='done', last_error='', updated_at=? WHERE id=?`, ts(now), id)
	return err
}

// MarkFailed 标记作业失败；未超最大次数时按退避时间重新排程，否则进入 dead。
func (r *JobRepo) MarkFailed(ctx context.Context, q sqlite.Querier, id int64, attempts, maxAttempts int,
	errMsg string, nextRunAt time.Time, now time.Time) error {
	status := "failed"
	if attempts >= maxAttempts {
		status = "dead"
	}
	_, err := q.ExecContext(ctx,
		`UPDATE jobs SET status=?, last_error=?, run_at=?, updated_at=? WHERE id=?`,
		status, errMsg, ts(nextRunAt), ts(now), id)
	return err
}

// RequeueStale 进程重启恢复：将 running 状态的作业恢复为 pending。
func (r *JobRepo) RequeueStale(ctx context.Context, q sqlite.Querier, now time.Time) (int64, error) {
	res, err := q.ExecContext(ctx,
		`UPDATE jobs SET status='pending', updated_at=? WHERE status='running'`, ts(now))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Retry 人工重试：failed/dead 作业恢复 pending 并立即到期。
func (r *JobRepo) Retry(ctx context.Context, q sqlite.Querier, id int64, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`UPDATE jobs SET status='pending', run_at=?, updated_at=? WHERE id=? AND status IN ('failed','dead')`,
		ts(now), ts(now), id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return apperr.Conflict("仅 failed/dead 状态的作业允许重试").WithDetail("job_id", id)
	}
	return nil
}
