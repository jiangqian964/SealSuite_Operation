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
		`CREATE TABLE IF NOT EXISTS feishu_apis (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			app_id TEXT NOT NULL,
			app_secret TEXT,
			base_url TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			is_active INTEGER NOT NULL DEFAULT 0,
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
		`CREATE TABLE IF NOT EXISTS feishu_resources (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'ip',
			tag_ids TEXT,
			tag_names TEXT,
			raw_json TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS feishu_resources_state (
			id INTEGER PRIMARY KEY,
			last_refresh_at TEXT NOT NULL DEFAULT '0001-01-01T00:00:00Z',
			last_refresh_count INTEGER NOT NULL DEFAULT 0,
			schedule_enabled INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS feishu_devices (
			did TEXT PRIMARY KEY,
			user_id TEXT NOT NULL DEFAULT '',
			os TEXT NOT NULL DEFAULT '',
			client_ip TEXT NOT NULL DEFAULT '',
			client_ip_location TEXT NOT NULL DEFAULT '',
			device_status TEXT NOT NULL DEFAULT '',
			nic_type TEXT NOT NULL DEFAULT '',
			mac_addrs TEXT,
			is_vm INTEGER NOT NULL DEFAULT 0,
			serial_number TEXT NOT NULL DEFAULT '',
			mac_addr TEXT NOT NULL DEFAULT '',
			is_virtual INTEGER NOT NULL DEFAULT 0,
			is_default INTEGER NOT NULL DEFAULT 0,
			hdd_serial_numbers TEXT,
			ssd_serial_numbers TEXT,
			cpu_serial_number TEXT,
			windows_ad_domain_name TEXT,
			groups_id TEXT NOT NULL DEFAULT '',
			groups_name TEXT NOT NULL DEFAULT '',
			groups_mode TEXT NOT NULL DEFAULT '',
			device_type TEXT NOT NULL DEFAULT '',
			trusted_status TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS feishu_devices_state (
			id INTEGER PRIMARY KEY,
			last_sync_at TEXT NOT NULL DEFAULT '0001-01-01T00:00:00Z',
			last_sync_count INTEGER NOT NULL DEFAULT 0,
			schedule_enabled INTEGER NOT NULL DEFAULT 0,
			schedule_interval TEXT NOT NULL DEFAULT '1h',
			last_import_at TEXT NOT NULL DEFAULT '0001-01-01T00:00:00Z',
			last_import_count INTEGER NOT NULL DEFAULT 0,
			import_schedule_enabled INTEGER NOT NULL DEFAULT 0,
			import_schedule_interval TEXT NOT NULL DEFAULT '1h'
		);`,
		`CREATE TABLE IF NOT EXISTS field_mappings (
			id TEXT PRIMARY KEY,
			source_field TEXT NOT NULL,
			source_label TEXT NOT NULL,
			target_field TEXT NOT NULL,
			target_label TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
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
			create_mode TEXT NOT NULL DEFAULT 'select',
			new_resource_name TEXT NOT NULL DEFAULT '',
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
		`CREATE TABLE IF NOT EXISTS device_groups (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			raw_json TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS device_groups_state (
			id INTEGER PRIMARY KEY,
			last_refresh_at TEXT NOT NULL DEFAULT '0001-01-01T00:00:00Z',
			last_refresh_count INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS dlp_events (
			id TEXT PRIMARY KEY,
			file_info_name TEXT NOT NULL DEFAULT '',
			file_info_path TEXT NOT NULL DEFAULT '',
			file_info_type TEXT NOT NULL DEFAULT '',
			leak_way_app_name TEXT NOT NULL DEFAULT '',
			evidence_url TEXT NOT NULL DEFAULT '',
			event_type TEXT NOT NULL DEFAULT '',
			user_id TEXT NOT NULL DEFAULT '',
			user_name TEXT NOT NULL DEFAULT '',
			device_id TEXT NOT NULL DEFAULT '',
			event_time TEXT NOT NULL DEFAULT '',
			imported_at TEXT NOT NULL DEFAULT '',
			analyzed INTEGER NOT NULL DEFAULT 0,
			raw_json TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_dlp_events_event_time ON dlp_events(event_time);`,
		`CREATE INDEX IF NOT EXISTS idx_dlp_events_analyzed ON dlp_events(analyzed);`,
		`CREATE TABLE IF NOT EXISTS dlp_analysis_results (
			id TEXT PRIMARY KEY,
			event_id TEXT NOT NULL,
			category TEXT NOT NULL DEFAULT '',
			should_exclude INTEGER NOT NULL DEFAULT 0,
			confidence REAL NOT NULL DEFAULT 0,
			reasoning TEXT NOT NULL DEFAULT '',
			analyzed_at TEXT NOT NULL DEFAULT '',
			whitelisted INTEGER NOT NULL DEFAULT 0,
			match_type TEXT NOT NULL DEFAULT '',
			match_value TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE INDEX IF NOT EXISTS idx_dlp_analysis_results_event_id ON dlp_analysis_results(event_id);`,
		`CREATE INDEX IF NOT EXISTS idx_dlp_analysis_results_should_exclude ON dlp_analysis_results(should_exclude);`,
		`CREATE TABLE IF NOT EXISTS dlp_whitelist (
			id TEXT PRIMARY KEY,
			match_type TEXT NOT NULL DEFAULT '',
			match_value TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE INDEX IF NOT EXISTS idx_dlp_whitelist_match_type ON dlp_whitelist(match_type);`,
		`CREATE TABLE IF NOT EXISTS dlp_analysis_state (
			id INTEGER PRIMARY KEY,
			last_sync_at TEXT NOT NULL DEFAULT '0001-01-01T00:00:00Z',
			last_sync_count INTEGER NOT NULL DEFAULT 0,
			sync_schedule_enabled INTEGER NOT NULL DEFAULT 1,
			last_analysis_at TEXT NOT NULL DEFAULT '0001-01-01T00:00:00Z',
			last_analysis_count INTEGER NOT NULL DEFAULT 0,
			analysis_schedule_enabled INTEGER NOT NULL DEFAULT 1
		);`,
		`CREATE TABLE IF NOT EXISTS dup_devices (
			did TEXT PRIMARY KEY,
			device_name TEXT NOT NULL DEFAULT '',
			serial_number TEXT NOT NULL DEFAULT '',
			hdd_serials TEXT NOT NULL DEFAULT '[]',
			ssd_serials TEXT NOT NULL DEFAULT '[]',
			cpu_serial TEXT NOT NULL DEFAULT '',
			mem_serials TEXT NOT NULL DEFAULT '[]',
			mac_addresses TEXT NOT NULL DEFAULT '[]',
			updated_time TEXT NOT NULL DEFAULT '',
			raw_json TEXT NOT NULL DEFAULT '',
			synced_at TEXT NOT NULL DEFAULT '',
			is_valid INTEGER NOT NULL DEFAULT 1
		);`,
		`CREATE INDEX IF NOT EXISTS idx_dup_devices_serial_number ON dup_devices(serial_number);`,
		`CREATE INDEX IF NOT EXISTS idx_dup_devices_is_valid ON dup_devices(is_valid);`,
		`CREATE TABLE IF NOT EXISTS dup_device_groups (
			id TEXT PRIMARY KEY,
			match_key TEXT NOT NULL DEFAULT '',
			match_value TEXT NOT NULL DEFAULT '',
			match_level TEXT NOT NULL DEFAULT '',
			device_count INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending',
			detected_at TEXT NOT NULL DEFAULT '',
			resolved_at TEXT NOT NULL DEFAULT '',
			retained_did TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE INDEX IF NOT EXISTS idx_dup_device_groups_status ON dup_device_groups(status);`,
		`CREATE INDEX IF NOT EXISTS idx_dup_device_groups_match_level ON dup_device_groups(match_level);`,
		`CREATE TABLE IF NOT EXISTS dup_device_group_members (
			id TEXT PRIMARY KEY,
			group_id TEXT NOT NULL DEFAULT '',
			did TEXT NOT NULL DEFAULT '',
			device_name TEXT NOT NULL DEFAULT '',
			serial_number TEXT NOT NULL DEFAULT '',
			hdd_serials TEXT NOT NULL DEFAULT '[]',
			ssd_serials TEXT NOT NULL DEFAULT '[]',
			cpu_serial TEXT NOT NULL DEFAULT '',
			mem_serials TEXT NOT NULL DEFAULT '[]',
			mac_addresses TEXT NOT NULL DEFAULT '[]',
			updated_time TEXT NOT NULL DEFAULT '',
			retained INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_dup_device_group_members_group_id ON dup_device_group_members(group_id);`,
		`CREATE TABLE IF NOT EXISTS dup_device_cleanup_logs (
			id TEXT PRIMARY KEY,
			group_id TEXT NOT NULL DEFAULT '',
			did TEXT NOT NULL DEFAULT '',
			device_name TEXT NOT NULL DEFAULT '',
			serial_number TEXT NOT NULL DEFAULT '',
			cleanup_type TEXT NOT NULL DEFAULT '',
			cleanup_status TEXT NOT NULL DEFAULT '',
			error_msg TEXT NOT NULL DEFAULT '',
			cleaned_at TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE INDEX IF NOT EXISTS idx_dup_device_cleanup_logs_cleaned_at ON dup_device_cleanup_logs(cleaned_at);`,
		`CREATE TABLE IF NOT EXISTS dup_device_task_state (
			id TEXT PRIMARY KEY,
			last_sync_at TEXT NOT NULL DEFAULT '',
			last_sync_count INTEGER NOT NULL DEFAULT 0,
			last_sync_error TEXT NOT NULL DEFAULT '',
			last_detect_at TEXT NOT NULL DEFAULT '',
			last_detect_groups INTEGER NOT NULL DEFAULT 0,
			last_detect_error TEXT NOT NULL DEFAULT '',
			sync_enabled INTEGER NOT NULL DEFAULT 1,
			detect_enabled INTEGER NOT NULL DEFAULT 1
		);`,
		`CREATE TABLE IF NOT EXISTS ztna_access_logs (
			id TEXT PRIMARY KEY,
			log_id TEXT NOT NULL DEFAULT '',
			user_id TEXT NOT NULL DEFAULT '',
			user_full_name TEXT NOT NULL DEFAULT '',
			department_id TEXT NOT NULL DEFAULT '',
			department_path TEXT NOT NULL DEFAULT '',
			roles_name TEXT NOT NULL DEFAULT '',
			roles_id TEXT NOT NULL DEFAULT '',
			dest_ip TEXT NOT NULL DEFAULT '',
			dest_port INTEGER NOT NULL DEFAULT 0,
			action TEXT NOT NULL DEFAULT '',
			protocol TEXT NOT NULL DEFAULT '',
			event_time TEXT NOT NULL DEFAULT '',
			imported_at TEXT NOT NULL DEFAULT '',
			raw_json TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_ztna_access_logs_event_time ON ztna_access_logs(event_time);`,
		`CREATE INDEX IF NOT EXISTS idx_ztna_access_logs_user_id ON ztna_access_logs(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_ztna_access_logs_dest_ip ON ztna_access_logs(dest_ip);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_ztna_access_logs_log_id ON ztna_access_logs(log_id);`,
		`CREATE TABLE IF NOT EXISTS ztna_access_stats (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL DEFAULT '',
			user_name TEXT NOT NULL DEFAULT '',
			user_role TEXT NOT NULL DEFAULT '',
			user_department TEXT NOT NULL DEFAULT '',
			department_id TEXT NOT NULL DEFAULT '',
			dest_ip TEXT NOT NULL DEFAULT '',
			dest_port INTEGER NOT NULL DEFAULT 0,
			protocol TEXT NOT NULL DEFAULT '',
			resource_tag TEXT NOT NULL DEFAULT '',
			first_access_time TEXT NOT NULL DEFAULT '',
			last_access_time TEXT NOT NULL DEFAULT '',
			access_days_30 INTEGER NOT NULL DEFAULT 0,
			access_days_90 INTEGER NOT NULL DEFAULT 0,
			total_access_count INTEGER NOT NULL DEFAULT 0,
			success_count INTEGER NOT NULL DEFAULT 0,
			failed_count INTEGER NOT NULL DEFAULT 0,
			avg_duration_sec INTEGER NOT NULL DEFAULT 0,
			total_bytes INTEGER NOT NULL DEFAULT 0,
			work_hour_count INTEGER NOT NULL DEFAULT 0,
			off_hour_count INTEGER NOT NULL DEFAULT 0,
			night_count INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_ztna_stats_user_dest ON ztna_access_stats(user_id, dest_ip, dest_port, protocol);`,
		`CREATE INDEX IF NOT EXISTS idx_ztna_stats_user_id ON ztna_access_stats(user_id);`,
		`CREATE TABLE IF NOT EXISTS ztna_policy_analysis (
			id TEXT PRIMARY KEY,
			stats_id TEXT NOT NULL DEFAULT '',
			user_id TEXT NOT NULL DEFAULT '',
			user_name TEXT NOT NULL DEFAULT '',
			user_role TEXT NOT NULL DEFAULT '',
			user_department TEXT NOT NULL DEFAULT '',
			dest_ip TEXT NOT NULL DEFAULT '',
			dest_port INTEGER NOT NULL DEFAULT 0,
			protocol TEXT NOT NULL DEFAULT '',
			resource_tag TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT '',
			should_revoke INTEGER NOT NULL DEFAULT 0,
			confidence REAL NOT NULL DEFAULT 0,
			reasoning TEXT NOT NULL DEFAULT '',
			suggested_policy TEXT NOT NULL DEFAULT '',
			method TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			reviewed_by TEXT NOT NULL DEFAULT '',
			reviewed_at TEXT NOT NULL DEFAULT '',
			review_comment TEXT NOT NULL DEFAULT '',
			analyzed_at TEXT NOT NULL DEFAULT '',
			revoked INTEGER NOT NULL DEFAULT 0,
			revoked_at TEXT NOT NULL DEFAULT '',
			revoke_error TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE INDEX IF NOT EXISTS idx_ztna_analysis_stats_id ON ztna_policy_analysis(stats_id);`,
		`CREATE INDEX IF NOT EXISTS idx_ztna_analysis_category ON ztna_policy_analysis(category);`,
		`CREATE INDEX IF NOT EXISTS idx_ztna_analysis_should_revoke ON ztna_policy_analysis(should_revoke);`,
		`CREATE INDEX IF NOT EXISTS idx_ztna_analysis_status ON ztna_policy_analysis(status);`,
		`CREATE TABLE IF NOT EXISTS ztna_task_state (
			id TEXT PRIMARY KEY,
			last_sync_at TEXT NOT NULL DEFAULT '',
			last_sync_count INTEGER NOT NULL DEFAULT 0,
			last_sync_error TEXT NOT NULL DEFAULT '',
			last_stats_at TEXT NOT NULL DEFAULT '',
			last_stats_user_count INTEGER NOT NULL DEFAULT 0,
			last_stats_resource_count INTEGER NOT NULL DEFAULT 0,
			last_stats_error TEXT NOT NULL DEFAULT '',
			last_analysis_at TEXT NOT NULL DEFAULT '',
			last_analysis_count INTEGER NOT NULL DEFAULT 0,
			last_analysis_error TEXT NOT NULL DEFAULT '',
			sync_schedule_enabled INTEGER NOT NULL DEFAULT 1,
			stats_schedule_enabled INTEGER NOT NULL DEFAULT 1,
			analysis_schedule_enabled INTEGER NOT NULL DEFAULT 1
		);`,
		// 自动化审批：白名单分组与待审批 Webhook 配置（单行配置，id 固定为 1）
		`CREATE TABLE IF NOT EXISTS approval_config (
			id INTEGER PRIMARY KEY,
			group_ids_json TEXT NOT NULL DEFAULT '[]',
			webhook_id TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL DEFAULT ''
		);`,
		// 自动化审批：设备申报任务流水
		`CREATE TABLE IF NOT EXISTS approval_tasks (
			id TEXT PRIMARY KEY,
			event_id TEXT NOT NULL DEFAULT '',
			device_identifier TEXT NOT NULL DEFAULT '',
			raw_payload_json TEXT NOT NULL DEFAULT '{}',
			status TEXT NOT NULL DEFAULT 'pending',
			feishu_device_record_id TEXT NOT NULL DEFAULT '',
			webhook_delivery_json TEXT NOT NULL DEFAULT '',
			error_message TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_approval_tasks_event_id ON approval_tasks(event_id);`,
		`CREATE INDEX IF NOT EXISTS idx_approval_tasks_status ON approval_tasks(status);`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	if err := ensureColumnExists(db, "feishu_resources", "tag_ids", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumnExists(db, "feishu_resources", "tag_names", "TEXT"); err != nil {
		return err
	}

	if err := ensureColumnExists(db, "external_ip_sync_tasks", "resource_name_snapshot", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnExists(db, "external_ip_sync_tasks", "resource_tag_names", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnExists(db, "external_ip_sync_tasks", "create_mode", "TEXT NOT NULL DEFAULT 'select'"); err != nil {
		return err
	}
	if err := ensureColumnExists(db, "external_ip_sync_tasks", "new_resource_name", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	if err := ensureColumnExists(db, "feishu_devices", "device_name", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnExists(db, "feishu_devices", "full_name", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnExists(db, "feishu_devices", "department_name", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnExists(db, "feishu_devices", "mem_serial_numbers", "TEXT"); err != nil {
		return err
	}
	// 飞书设备记录 ID：用于标记该设备是否已导入飞书，以及更新时的 device_record_id
	if err := ensureColumnExists(db, "feishu_devices", "feishu_device_record_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	// webhooks 表扩展：支持飞书自定义群机器人的加签密钥与消息类型（text/post/interactive）
	if err := ensureColumnExists(db, "webhooks", "secret", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnExists(db, "webhooks", "msg_type", "TEXT NOT NULL DEFAULT 'text'"); err != nil {
		return err
	}

	return nil
}

func ensureColumnExists(db *sql.DB, table, column, colType string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()

	exists := false
	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dflt_value interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt_value, &pk); err != nil {
			return err
		}
		if name == column {
			exists = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if !exists {
		_, err := db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + colType)
		if err != nil {
			return err
		}
	}
	return nil
}
