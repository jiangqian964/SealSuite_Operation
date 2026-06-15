package db

import "database/sql"

func Migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS app_connections (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			scheme TEXT NOT NULL,
			host TEXT NOT NULL,
			port INTEGER NOT NULL,
			access_key_id TEXT NOT NULL,
			secret_ref TEXT NOT NULL,
			secret_value TEXT NOT NULL,
			is_active INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS llm_apis (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			provider TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key TEXT NOT NULL,
			model TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			is_active INTEGER NOT NULL DEFAULT 0,
			tags_json TEXT NOT NULL DEFAULT '[]',
			settings_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS webhooks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			provider TEXT NOT NULL,
			url TEXT NOT NULL,
			method TEXT NOT NULL,
			headers_json TEXT NOT NULL DEFAULT '{}',
			auth_type TEXT NOT NULL DEFAULT '',
			body_template TEXT NOT NULL DEFAULT '',
			timeout_sec INTEGER NOT NULL DEFAULT 10,
			retry_count INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS api_templates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			method TEXT NOT NULL,
			path TEXT NOT NULL,
			category TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			query_schema_json TEXT NOT NULL DEFAULT '{}',
			path_params_schema_json TEXT NOT NULL DEFAULT '{}',
			body_schema_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS output_templates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			format TEXT NOT NULL,
			content_template TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS task_drafts (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			mode TEXT NOT NULL,
			cycle_mode TEXT NOT NULL DEFAULT 'once',
			run_count INTEGER NOT NULL DEFAULT 1,
			run_until TEXT NOT NULL DEFAULT '',
			source_template_id TEXT NOT NULL DEFAULT '',
			webhook_config_id TEXT NOT NULL DEFAULT '',
			webhook_enabled INTEGER NOT NULL DEFAULT 0,
			input_config_json TEXT NOT NULL DEFAULT '{}',
			transform_config_json TEXT NOT NULL DEFAULT '{}',
			llm_config_json TEXT NOT NULL DEFAULT '{}',
			output_config_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS complex_tasks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			goal TEXT NOT NULL DEFAULT '',
			execution_mode TEXT NOT NULL DEFAULT '',
			webhook_config_id TEXT NOT NULL DEFAULT '',
			webhook_enabled INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS complex_task_steps (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			step_order INTEGER NOT NULL,
			type TEXT NOT NULL,
			name TEXT NOT NULL,
			config_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS job_schedules (
			id TEXT PRIMARY KEY,
			draft_id TEXT NOT NULL DEFAULT '',
			target_type TEXT NOT NULL,
			target_id TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			webhook_config_id TEXT NOT NULL DEFAULT '',
			webhook_enabled INTEGER NOT NULL DEFAULT 0,
			start_at TEXT NOT NULL DEFAULT '',
			end_at TEXT NOT NULL DEFAULT '',
			schedule_type TEXT NOT NULL DEFAULT 'interval',
			cron_expr TEXT NOT NULL DEFAULT '',
			interval_expr TEXT NOT NULL DEFAULT '',
			timezone TEXT NOT NULL DEFAULT 'Asia/Shanghai',
			post_filter_json TEXT NOT NULL DEFAULT '{}',
			field_select_json TEXT NOT NULL DEFAULT '{}',
			truncate_rules_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS external_ip_sync_tasks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			source_type TEXT NOT NULL,
			source_url TEXT NOT NULL,
			ip_version TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			resource_name_snapshot TEXT NOT NULL DEFAULT '',
			write_action TEXT NOT NULL,
			feilian_api_path TEXT NOT NULL,
			dry_run INTEGER NOT NULL DEFAULT 0,
			skip_when_empty INTEGER NOT NULL DEFAULT 1,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS legacy_jobs (
			name TEXT PRIMARY KEY,
			enabled INTEGER NOT NULL DEFAULT 1,
			cron_expr TEXT NOT NULL,
			type TEXT NOT NULL,
			params_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS app_meta (
			meta_key TEXT PRIMARY KEY,
			meta_value TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS job_runs (
			id TEXT PRIMARY KEY,
			source_type TEXT NOT NULL,
			source_id TEXT NOT NULL,
			target_type TEXT NOT NULL,
			target_id TEXT NOT NULL,
			status TEXT NOT NULL,
			trigger_source TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			error_message TEXT NOT NULL DEFAULT '',
			result_json TEXT NOT NULL DEFAULT '{}'
		);`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id TEXT PRIMARY KEY,
			actor TEXT NOT NULL,
			action TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			payload_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL
		);`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}
