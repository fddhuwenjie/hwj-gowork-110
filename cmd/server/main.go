// Command server 为高纬度天文台低温仪器观测窗口与校准归档服务入口。
// 读取 PORT 与 DB_PATH，使用真实嵌入式 SQLite 持久化（禁止 :memory:），
// 提供 /healthz、统一错误、结构化 JSON 日志、优雅关闭与可注入时钟。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"observatory/internal/clock"
	"observatory/internal/config"
	"observatory/internal/httpx"
	"observatory/internal/jobs"
	"observatory/internal/logging"
	"observatory/internal/service"
	"observatory/internal/store/sqlite"
)

func main() {
	log := logging.New(os.Stdout)
	if err := run(log); err != nil {
		log.Error("服务退出", "error", err)
		os.Exit(1)
	}
}

// run 完成配置加载、数据库迁移、服务组装、作业恢复与优雅关闭。
func run(log *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	log.Info("配置加载完成", "port", cfg.Port, "db_path", cfg.DBPath)

	db, err := sqlite.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.Migrate(ctx); err != nil {
		return err
	}
	log.Info("数据库迁移完成", "db_path", cfg.DBPath)

	clk := clock.Real{}
	svc := service.New(db, clk, cfg.JobMaxAttempts)

	// 后台作业：重启恢复 + 注册处理器 + 启动轮询。
	runner := jobs.NewRunner(db, clk, cfg.JobPollInterval, cfg.JobRetryBackoff, log)
	jobs.RegisterHandlers(runner, svc)
	if err := runner.Recover(ctx); err != nil {
		return err
	}
	jobCtx, stopJobs := context.WithCancel(ctx)
	defer stopJobs()
	go runner.Run(jobCtx)

	srv := httpx.NewServer(cfg, svc, log)

	// 信号处理：SIGINT/SIGTERM 触发优雅关闭。
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	errCh := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case sig := <-sigCh:
		log.Info("收到退出信号，开始优雅关闭", "signal", sig.String())
	case err := <-errCh:
		return err
	}

	stopJobs()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	log.Info("服务已优雅关闭")
	return nil
}
