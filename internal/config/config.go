// Package config 负责从环境变量加载服务配置。
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config 汇总服务运行所需的全部配置项。
type Config struct {
	// Port 为 HTTP 监听端口，来自 PORT，默认 8080。
	Port int
	// DBPath 为 SQLite 数据库文件路径，来自 DB_PATH，禁止 :memory:。
	DBPath string
	// JobPollInterval 为后台作业轮询间隔。
	JobPollInterval time.Duration
	// JobRetryBackoff 为失败作业的基础退避时长。
	JobRetryBackoff time.Duration
	// JobMaxAttempts 为作业最大尝试次数。
	JobMaxAttempts int
	// ShutdownTimeout 为优雅关闭等待时长。
	ShutdownTimeout time.Duration
}

// Load 读取环境变量并返回配置；DB_PATH 缺失或非法时返回错误。
func Load(getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	cfg := Config{
		Port:            8080,
		DBPath:          getenv("DB_PATH"),
		JobPollInterval: 500 * time.Millisecond,
		JobRetryBackoff: 2 * time.Second,
		JobMaxAttempts:  5,
		ShutdownTimeout: 10 * time.Second,
	}
	if cfg.DBPath == "" {
		return Config{}, fmt.Errorf("DB_PATH 环境变量必须提供，且不得为 :memory:")
	}
	if cfg.DBPath == ":memory:" {
		return Config{}, fmt.Errorf("DB_PATH 不得为 :memory:，服务要求真实文件持久化")
	}
	if p := getenv("PORT"); p != "" {
		port, err := strconv.Atoi(p)
		if err != nil || port <= 0 || port > 65535 {
			return Config{}, fmt.Errorf("PORT 非法: %q", p)
		}
		cfg.Port = port
	}
	if v := getenv("JOB_POLL_INTERVAL_MS"); v != "" {
		ms, err := strconv.Atoi(v)
		if err != nil || ms < 10 {
			return Config{}, fmt.Errorf("JOB_POLL_INTERVAL_MS 非法: %q", v)
		}
		cfg.JobPollInterval = time.Duration(ms) * time.Millisecond
	}
	if v := getenv("JOB_MAX_ATTEMPTS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("JOB_MAX_ATTEMPTS 非法: %q", v)
		}
		cfg.JobMaxAttempts = n
	}
	return cfg, nil
}

// Addr 返回 HTTP 监听地址。
func (c Config) Addr() string {
	return fmt.Sprintf(":%d", c.Port)
}
