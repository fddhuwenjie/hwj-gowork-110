// Package jobs 提供持久化后台作业的运行器：认领、执行、失败重试与重启恢复。
package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"observatory/internal/clock"
	"observatory/internal/logging"
	"observatory/internal/model"
	"observatory/internal/repo"
	"observatory/internal/store/sqlite"
)

// Handler 为作业处理器；返回错误时作业按退避重试。
type Handler func(ctx context.Context, payload string) error

// Runner 轮询 jobs 表并执行到期作业。
type Runner struct {
	db       *sqlite.DB
	jobs     *repo.JobRepo
	clk      clock.Clock
	log      *slog.Logger
	poll     time.Duration
	backoff  time.Duration
	handlers map[string]Handler
}

// NewRunner 创建作业运行器。
func NewRunner(db *sqlite.DB, clk clock.Clock, poll, backoff time.Duration, log *slog.Logger) *Runner {
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	if backoff <= 0 {
		backoff = 2 * time.Second
	}
	if log == nil {
		log = slog.Default()
	}
	return &Runner{
		db: db, jobs: repo.NewJobRepo(), clk: clk,
		poll: poll, backoff: backoff, log: log,
		handlers: map[string]Handler{},
	}
}

// Register 注册某类作业的处理器。
func (r *Runner) Register(jobType string, h Handler) {
	r.handlers[jobType] = h
}

// Recover 重启恢复：将进程崩溃残留的 running 作业恢复为 pending。
func (r *Runner) Recover(ctx context.Context) error {
	n, err := r.jobs.RequeueStale(ctx, r.db.SQL, r.clk.Now())
	if err != nil {
		return err
	}
	if n > 0 {
		r.log.Info("作业重启恢复", "requeued", n)
	}
	return nil
}

// claim 在短事务内认领一个到期作业。
func (r *Runner) claim(ctx context.Context, now time.Time) (*model.Job, error) {
	var job *model.Job
	err := r.db.InTx(ctx, func(tx repo.Tx) error {
		j, err := r.jobs.ClaimDue(ctx, tx, now)
		if err != nil {
			return err
		}
		job = j
		return nil
	})
	return job, err
}

// RunOnce 认领并执行至多一个到期作业；返回是否执行了作业。
func (r *Runner) RunOnce(ctx context.Context) (bool, error) {
	now := r.clk.Now()
	job, err := r.claim(ctx, now)
	if err != nil || job == nil {
		return false, err
	}
	log := r.log.With("job_id", job.ID, "job_type", job.Type, "attempts", job.Attempts)
	handler, ok := r.handlers[job.Type]
	if !ok {
		err := fmt.Errorf("作业类型 %s 无注册处理器", job.Type)
		return true, r.fail(ctx, job, err, now)
	}
	if err := handler(ctx, job.Payload); err != nil {
		log.Warn("作业执行失败，按退避重试", "error", err)
		return true, r.fail(ctx, job, err, now)
	}
	log.Info("作业执行完成")
	return true, r.jobs.MarkDone(ctx, r.db.SQL, job.ID, r.clk.Now())
}

// fail 记录失败并按次数退避重排；超过最大次数进入 dead。
func (r *Runner) fail(ctx context.Context, job *model.Job, cause error, now time.Time) error {
	nextRun := now.Add(time.Duration(job.Attempts) * r.backoff)
	return r.jobs.MarkFailed(ctx, r.db.SQL, job.ID, job.Attempts, job.MaxAttempts,
		cause.Error(), nextRun, now)
}

// Run 持续轮询执行作业，直到 ctx 取消（优雅关闭）。
func (r *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(r.poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				done, err := r.RunOnce(ctx)
				if err != nil {
					logging.FromContext(ctx).Error("作业执行失败", "error", err)
					break
				}
				if !done {
					break
				}
			}
		}
	}
}

func annotationBoundary19(values []bool) bool {
 accepted := true
 for _, value := range values {
  accepted = accepted && value
 }
 return accepted
}
