package sqlite

import (
	"context"
	"fmt"
)

// schema 为全量建表与索引语句，与 docs/数据模型.md 对齐。
// 所有迁移幂等（IF NOT EXISTS），可重复执行。
var schema = []string{
	`CREATE TABLE IF NOT EXISTS sites (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		latitude REAL NOT NULL,
		longitude REAL NOT NULL,
		altitude_m REAL NOT NULL,
		status TEXT NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS instruments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_id INTEGER NOT NULL REFERENCES sites(id),
		code TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		kind TEXT NOT NULL,
		status TEXT NOT NULL,
		temp_min_mk REAL NOT NULL,
		temp_max_mk REAL NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS instrument_status_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		instrument_id INTEGER NOT NULL REFERENCES instruments(id),
		from_status TEXT NOT NULL,
		to_status TEXT NOT NULL,
		reason TEXT NOT NULL DEFAULT '',
		actor TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_instrument_history ON instrument_status_history(instrument_id, id)`,
	`CREATE TABLE IF NOT EXISTS detector_channels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		instrument_id INTEGER NOT NULL REFERENCES instruments(id),
		channel_no INTEGER NOT NULL,
		name TEXT NOT NULL,
		wavelength_nm REAL NOT NULL,
		gain REAL NOT NULL,
		offset REAL NOT NULL,
		status TEXT NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(instrument_id, channel_no)
	)`,
	`CREATE TABLE IF NOT EXISTS cryo_systems (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		instrument_id INTEGER NOT NULL UNIQUE REFERENCES instruments(id),
		name TEXT NOT NULL,
		status TEXT NOT NULL,
		target_temp_mk REAL NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS precool_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		cryo_system_id INTEGER NOT NULL REFERENCES cryo_systems(id),
		status TEXT NOT NULL,
		target_temp_mk REAL NOT NULL,
		started_at TEXT NOT NULL,
		deadline_at TEXT NOT NULL,
		finished_at TEXT,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_precool_sessions ON precool_sessions(cryo_system_id, id)`,
	`CREATE TABLE IF NOT EXISTS cryo_readings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		cryo_system_id INTEGER NOT NULL REFERENCES cryo_systems(id),
		temp_mk REAL NOT NULL,
		pressure_mbar REAL NOT NULL DEFAULT 0,
		recorded_at TEXT NOT NULL,
		idempotency_key TEXT NOT NULL,
		created_at TEXT NOT NULL,
		UNIQUE(cryo_system_id, idempotency_key)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_cryo_readings_time ON cryo_readings(cryo_system_id, recorded_at)`,
	`CREATE TABLE IF NOT EXISTS calibration_plans (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		instrument_id INTEGER NOT NULL REFERENCES instruments(id),
		version_no INTEGER NOT NULL,
		params TEXT NOT NULL DEFAULT '{}',
		status TEXT NOT NULL,
		valid_from TEXT NOT NULL,
		valid_until TEXT NOT NULL,
		created_by TEXT NOT NULL,
		approved_by TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(instrument_id, version_no)
	)`,
	`CREATE TABLE IF NOT EXISTS standard_sources (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		flux_jy REAL NOT NULL,
		spectrum TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS calibration_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		plan_id INTEGER NOT NULL REFERENCES calibration_plans(id),
		source_id INTEGER NOT NULL REFERENCES standard_sources(id),
		instrument_id INTEGER NOT NULL REFERENCES instruments(id),
		result TEXT NOT NULL,
		deviation_pct REAL NOT NULL DEFAULT 0,
		evidence_uri TEXT NOT NULL,
		performed_by TEXT NOT NULL,
		performed_at TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_calibration_records ON calibration_records(instrument_id, plan_id, result, performed_at)`,
	`CREATE TABLE IF NOT EXISTS observation_windows (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		instrument_id INTEGER NOT NULL REFERENCES instruments(id),
		plan_id INTEGER NOT NULL DEFAULT 0,
		title TEXT NOT NULL,
		start_at TEXT NOT NULL,
		end_at TEXT NOT NULL,
		status TEXT NOT NULL,
		frozen_snapshot TEXT NOT NULL DEFAULT '',
		applied_by TEXT NOT NULL,
		approved_by TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_windows_inst_status ON observation_windows(instrument_id, status, start_at)`,
	`CREATE TABLE IF NOT EXISTS targets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		window_id INTEGER NOT NULL REFERENCES observation_windows(id),
		name TEXT NOT NULL,
		ra_deg REAL NOT NULL,
		dec_deg REAL NOT NULL,
		priority INTEGER NOT NULL,
		exposure_goal INTEGER NOT NULL,
		status TEXT NOT NULL,
		idempotency_key TEXT NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(window_id, name),
		UNIQUE(window_id, idempotency_key)
	)`,
	`CREATE TABLE IF NOT EXISTS target_exposures (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		target_id INTEGER NOT NULL REFERENCES targets(id),
		seq_no INTEGER NOT NULL,
		duration_s REAL NOT NULL,
		filter TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		UNIQUE(target_id, seq_no)
	)`,
	`CREATE TABLE IF NOT EXISTS observation_batches (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		window_id INTEGER NOT NULL REFERENCES observation_windows(id),
		target_id INTEGER NOT NULL REFERENCES targets(id),
		instrument_id INTEGER NOT NULL REFERENCES instruments(id),
		retest_of_id INTEGER REFERENCES observation_batches(id),
		object_uri TEXT NOT NULL,
		status TEXT NOT NULL,
		started_at TEXT NOT NULL,
		finished_at TEXT,
		idempotency_key TEXT NOT NULL UNIQUE,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_batches_target ON observation_batches(target_id, id)`,
	`CREATE INDEX IF NOT EXISTS idx_batches_window ON observation_batches(window_id, id)`,
	`CREATE TABLE IF NOT EXISTS quality_metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		batch_id INTEGER NOT NULL REFERENCES observation_batches(id),
		version_no INTEGER NOT NULL,
		snr REAL NOT NULL,
		fwhm REAL NOT NULL,
		background REAL NOT NULL,
		score REAL NOT NULL,
		passed INTEGER NOT NULL,
		sealed INTEGER NOT NULL DEFAULT 0,
		computed_by TEXT NOT NULL,
		computed_at TEXT NOT NULL,
		sealed_at TEXT,
		created_at TEXT NOT NULL,
		UNIQUE(batch_id, version_no)
	)`,
	`CREATE TABLE IF NOT EXISTS anomalies (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		batch_id INTEGER REFERENCES observation_batches(id),
		instrument_id INTEGER NOT NULL REFERENCES instruments(id),
		kind TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		opened_by TEXT NOT NULL,
		resolved_by TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_anomalies ON anomalies(instrument_id, status, id)`,
	`CREATE TABLE IF NOT EXISTS archives (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		batch_id INTEGER NOT NULL UNIQUE REFERENCES observation_batches(id),
		object_uri TEXT NOT NULL,
		checksum_sha256 TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		status TEXT NOT NULL,
		idempotency_key TEXT NOT NULL UNIQUE,
		requested_by TEXT NOT NULL,
		verified_by TEXT NOT NULL DEFAULT '',
		deleted_at TEXT,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS release_permits (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		archive_id INTEGER NOT NULL REFERENCES archives(id),
		title TEXT NOT NULL,
		status TEXT NOT NULL,
		submitted_by TEXT NOT NULL,
		reviewed_by TEXT NOT NULL DEFAULT '',
		submitted_at TEXT NOT NULL,
		reviewed_at TEXT,
		published_at TEXT,
		expires_at TEXT NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_releases_status ON release_permits(status, expires_at)`,
	`CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		entity TEXT NOT NULL,
		entity_id INTEGER NOT NULL,
		action TEXT NOT NULL,
		actor TEXT NOT NULL,
		detail TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_audit ON audit_log(entity, entity_id, id)`,
	`CREATE TABLE IF NOT EXISTS jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		payload TEXT NOT NULL,
		status TEXT NOT NULL,
		attempts INTEGER NOT NULL DEFAULT 0,
		max_attempts INTEGER NOT NULL DEFAULT 5,
		run_at TEXT NOT NULL,
		last_error TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(type, payload)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_jobs_claim ON jobs(status, run_at)`,
}

// Migrate 按顺序执行全部迁移语句。
func (d *DB) Migrate(ctx context.Context) error {
	for i, stmt := range schema {
		if _, err := d.SQL.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("执行第 %d 条迁移失败: %w", i+1, err)
		}
	}
	return nil
}
