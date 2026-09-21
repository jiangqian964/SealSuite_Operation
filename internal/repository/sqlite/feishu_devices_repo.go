package sqlite

import (
	"database/sql"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

type FeishuDevicesRepository struct {
	baseRepo
}

func NewFeishuDevicesRepository(db *sql.DB) *FeishuDevicesRepository {
	return &FeishuDevicesRepository{baseRepo: baseRepo{db: db}}
}

func (r *FeishuDevicesRepository) List(osFilter, trustedStatusFilter, groupFilter string) ([]storage.FeishuDeviceItem, error) {
	osList := splitFilterList(osFilter)
	trustedList := splitFilterList(trustedStatusFilter)
	groupList := splitFilterList(groupFilter)
	return r.ListMulti(osList, trustedList, groupList, "AND")
}

func splitFilterList(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" || v == "*" {
		return []string{}
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" && p != "*" {
			out = append(out, p)
		}
	}
	return out
}

func (r *FeishuDevicesRepository) ListMulti(osList, trustedStatusList, groupList []string, logicMode string) ([]storage.FeishuDeviceItem, error) {
	baseQuery := `SELECT did, user_id, device_name, full_name, department_name, os, client_ip, client_ip_location, device_status, nic_type, mac_addrs, is_vm, serial_number, mac_addr, is_virtual, is_default, hdd_serial_numbers, ssd_serial_numbers, cpu_serial_number, mem_serial_numbers, windows_ad_domain_name, groups_id, groups_name, groups_mode, device_type, trusted_status, feishu_device_record_id, created_at, updated_at FROM feishu_devices`
	params := []interface{}{}
	filters := []string{}

	logicMode = strings.ToUpper(strings.TrimSpace(logicMode))
	if logicMode != "AND" && logicMode != "OR" {
		logicMode = "AND"
	}

	if len(osList) > 0 {
		parts := make([]string, 0, len(osList))
		for _, v := range osList {
			parts = append(parts, "os = ?")
			params = append(params, v)
		}
		filters = append(filters, "("+strings.Join(parts, " OR ")+")")
	}

	if len(trustedStatusList) > 0 {
		parts := make([]string, 0, len(trustedStatusList))
		for _, v := range trustedStatusList {
			parts = append(parts, "trusted_status = ?")
			params = append(params, v)
		}
		filters = append(filters, "("+strings.Join(parts, " OR ")+")")
	}

	if len(groupList) > 0 {
		parts := make([]string, 0, len(groupList))
		for _, v := range groupList {
			parts = append(parts, "groups_name LIKE ?")
			params = append(params, "%"+v+"%")
		}
		filters = append(filters, "("+strings.Join(parts, " OR ")+")")
	}

	query := baseQuery
	if len(filters) > 0 {
		query += " WHERE " + strings.Join(filters, " "+logicMode+" ")
	}
	query += " ORDER BY updated_at DESC, did ASC"

	rows, err := r.db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]storage.FeishuDeviceItem, 0)
	for rows.Next() {
		var (
			item          storage.FeishuDeviceItem
			isVM          int
			isVirtual     int
			isDefault     int
			macAddrs      sql.NullString
			hddSerials    sql.NullString
			ssdSerials    sql.NullString
			cpuSerial     sql.NullString
			memSerials    sql.NullString
			windowsDomain sql.NullString
		)
		err := rows.Scan(
			&item.DID, &item.UserID, &item.DeviceName, &item.FullName, &item.DepartmentName,
			&item.OS, &item.ClientIP, &item.ClientIPLocation,
			&item.DeviceStatus, &item.NICType, &macAddrs, &isVM, &item.SerialNumber,
			&item.MacAddr, &isVirtual, &isDefault, &hddSerials, &ssdSerials,
			&cpuSerial, &memSerials, &windowsDomain, &item.GroupsID, &item.GroupsName, &item.GroupsMode,
			&item.DeviceType, &item.TrustedStatus, &item.FeishuDeviceRecordID, &item.CreatedAt, &item.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		item.IsVM = isVM != 0
		item.IsVirtual = isVirtual != 0
		item.IsDefault = isDefault != 0
		if macAddrs.Valid {
			item.MacAddrs = macAddrs.String
		}
		if hddSerials.Valid {
			item.HDDSerialNumbers = hddSerials.String
		}
		if ssdSerials.Valid {
			item.SSDSerialNumbers = ssdSerials.String
		}
		if cpuSerial.Valid {
			item.CPUSerialNumber = cpuSerial.String
		}
		if memSerials.Valid {
			item.MemSerialNumbers = memSerials.String
		}
		if windowsDomain.Valid {
			item.WindowsADDomainName = windowsDomain.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *FeishuDevicesRepository) Get(did string) (*storage.FeishuDeviceItem, bool, error) {
	var (
		item          storage.FeishuDeviceItem
		isVM          int
		isVirtual     int
		isDefault     int
		macAddrs      sql.NullString
		hddSerials    sql.NullString
		ssdSerials    sql.NullString
		cpuSerial     sql.NullString
		memSerials    sql.NullString
		windowsDomain sql.NullString
	)
	err := r.db.QueryRow(`
		SELECT did, user_id, device_name, full_name, department_name, os, client_ip, client_ip_location, device_status, nic_type, mac_addrs, is_vm, serial_number, mac_addr, is_virtual, is_default, hdd_serial_numbers, ssd_serial_numbers, cpu_serial_number, mem_serial_numbers, windows_ad_domain_name, groups_id, groups_name, groups_mode, device_type, trusted_status, feishu_device_record_id, created_at, updated_at
		FROM feishu_devices
		WHERE did = ?
	`, strings.TrimSpace(did)).Scan(
		&item.DID, &item.UserID, &item.DeviceName, &item.FullName, &item.DepartmentName,
		&item.OS, &item.ClientIP, &item.ClientIPLocation,
		&item.DeviceStatus, &item.NICType, &macAddrs, &isVM, &item.SerialNumber,
		&item.MacAddr, &isVirtual, &isDefault, &hddSerials, &ssdSerials,
		&cpuSerial, &memSerials, &windowsDomain, &item.GroupsID, &item.GroupsName, &item.GroupsMode,
		&item.DeviceType, &item.TrustedStatus, &item.FeishuDeviceRecordID, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	item.IsVM = isVM != 0
	item.IsVirtual = isVirtual != 0
	item.IsDefault = isDefault != 0
	if macAddrs.Valid {
		item.MacAddrs = macAddrs.String
	}
	if hddSerials.Valid {
		item.HDDSerialNumbers = hddSerials.String
	}
	if ssdSerials.Valid {
		item.SSDSerialNumbers = ssdSerials.String
	}
	if cpuSerial.Valid {
		item.CPUSerialNumber = cpuSerial.String
	}
	if memSerials.Valid {
		item.MemSerialNumbers = memSerials.String
	}
	if windowsDomain.Valid {
		item.WindowsADDomainName = windowsDomain.String
	}
	return &item, true, nil
}

func (r *FeishuDevicesRepository) Upsert(item storage.FeishuDeviceItem) error {
	now := time.Now().Format(time.RFC3339)
	_, err := r.db.Exec(`
		INSERT INTO feishu_devices (
			did, user_id, device_name, full_name, department_name, os, client_ip, client_ip_location, device_status, nic_type,
			mac_addrs, is_vm, serial_number, mac_addr, is_virtual, is_default,
			hdd_serial_numbers, ssd_serial_numbers, cpu_serial_number, mem_serial_numbers, windows_ad_domain_name,
			groups_id, groups_name, groups_mode, device_type, trusted_status, feishu_device_record_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(did) DO UPDATE SET
			user_id = excluded.user_id,
			device_name = excluded.device_name,
			full_name = excluded.full_name,
			department_name = excluded.department_name,
			os = excluded.os,
			client_ip = excluded.client_ip,
			client_ip_location = excluded.client_ip_location,
			device_status = excluded.device_status,
			nic_type = excluded.nic_type,
			mac_addrs = excluded.mac_addrs,
			is_vm = excluded.is_vm,
			serial_number = excluded.serial_number,
			mac_addr = excluded.mac_addr,
			is_virtual = excluded.is_virtual,
			is_default = excluded.is_default,
			hdd_serial_numbers = excluded.hdd_serial_numbers,
			ssd_serial_numbers = excluded.ssd_serial_numbers,
			cpu_serial_number = excluded.cpu_serial_number,
			mem_serial_numbers = excluded.mem_serial_numbers,
			windows_ad_domain_name = excluded.windows_ad_domain_name,
			groups_id = excluded.groups_id,
			groups_name = excluded.groups_name,
			groups_mode = excluded.groups_mode,
			device_type = excluded.device_type,
			trusted_status = excluded.trusted_status,
			feishu_device_record_id = COALESCE(NULLIF(excluded.feishu_device_record_id, ''), feishu_devices.feishu_device_record_id),
			updated_at = excluded.updated_at
	`,
		item.DID, item.UserID, item.DeviceName, item.FullName, item.DepartmentName,
		item.OS, item.ClientIP, item.ClientIPLocation,
		item.DeviceStatus, item.NICType, item.MacAddrs, boolToInt(item.IsVM), item.SerialNumber,
		item.MacAddr, boolToInt(item.IsVirtual), boolToInt(item.IsDefault),
		item.HDDSerialNumbers, item.SSDSerialNumbers, item.CPUSerialNumber, item.MemSerialNumbers, item.WindowsADDomainName,
		item.GroupsID, item.GroupsName, item.GroupsMode, item.DeviceType, item.TrustedStatus, item.FeishuDeviceRecordID,
		item.CreatedAt, now,
	)
	return err
}

func (r *FeishuDevicesRepository) ReplaceAll(items []storage.FeishuDeviceItem) (int, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	// 全量同步前先备份已有设备的飞书 device_record_id，避免同步后丢失导入状态
	recordIDMap := map[string]string{}
	if rows, qErr := tx.Query(`SELECT did, feishu_device_record_id FROM feishu_devices WHERE feishu_device_record_id != ''`); qErr == nil {
		for rows.Next() {
			var did, rid string
			if sErr := rows.Scan(&did, &rid); sErr == nil {
				recordIDMap[did] = rid
			}
		}
		_ = rows.Close()
	}

	if _, err := tx.Exec(`DELETE FROM feishu_devices`); err != nil {
		return 0, err
	}

	now := time.Now().Format(time.RFC3339)
	count := 0
	for _, item := range items {
		item.DID = strings.TrimSpace(item.DID)
		if item.DID == "" {
			continue
		}
		if item.CreatedAt == "" {
			item.CreatedAt = now
		}
		// 恢复该设备已有的飞书 device_record_id
		if rid, ok := recordIDMap[item.DID]; ok && rid != "" {
			item.FeishuDeviceRecordID = rid
		}
		if _, err := tx.Exec(`
			INSERT INTO feishu_devices (
				did, user_id, device_name, full_name, department_name, os, client_ip, client_ip_location, device_status, nic_type,
				mac_addrs, is_vm, serial_number, mac_addr, is_virtual, is_default,
				hdd_serial_numbers, ssd_serial_numbers, cpu_serial_number, mem_serial_numbers, windows_ad_domain_name,
				groups_id, groups_name, groups_mode, device_type, trusted_status, feishu_device_record_id, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			item.DID, item.UserID, item.DeviceName, item.FullName, item.DepartmentName,
			item.OS, item.ClientIP, item.ClientIPLocation,
			item.DeviceStatus, item.NICType, item.MacAddrs, boolToInt(item.IsVM), item.SerialNumber,
			item.MacAddr, boolToInt(item.IsVirtual), boolToInt(item.IsDefault),
			item.HDDSerialNumbers, item.SSDSerialNumbers, item.CPUSerialNumber, item.MemSerialNumbers, item.WindowsADDomainName,
			item.GroupsID, item.GroupsName, item.GroupsMode, item.DeviceType, item.TrustedStatus, item.FeishuDeviceRecordID,
			item.CreatedAt, now,
		); err != nil {
			return count, err
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		return count, err
	}
	return count, nil
}

func (r *FeishuDevicesRepository) UpdateTrustedStatus(did, trustedStatus string) error {
	_, err := r.db.Exec(`UPDATE feishu_devices SET trusted_status = ?, updated_at = ? WHERE did = ?`,
		trustedStatus, time.Now().Format(time.RFC3339), strings.TrimSpace(did))
	return err
}

// UpdateFeishuDeviceRecordID 更新设备对应的飞书 device_record_id，用于标记已导入及后续更新
func (r *FeishuDevicesRepository) UpdateFeishuDeviceRecordID(did, recordID string) error {
	_, err := r.db.Exec(`UPDATE feishu_devices SET feishu_device_record_id = ?, updated_at = ? WHERE did = ?`,
		strings.TrimSpace(recordID), time.Now().Format(time.RFC3339), strings.TrimSpace(did))
	return err
}

func (r *FeishuDevicesRepository) GetState() (storage.FeishuDeviceSyncState, error) {
	var state storage.FeishuDeviceSyncState
	var (
		lastSyncAt             sql.NullString
		lastSyncCount          sql.NullInt64
		scheduleEnabled        sql.NullInt64
		scheduleInterval       sql.NullString
		lastImportAt           sql.NullString
		lastImportCount        sql.NullInt64
		importScheduleEnabled  sql.NullInt64
		importScheduleInterval sql.NullString
	)

	err := r.db.QueryRow(`
		SELECT last_sync_at, last_sync_count, schedule_enabled, schedule_interval,
		       last_import_at, last_import_count, import_schedule_enabled, import_schedule_interval
		FROM feishu_devices_state
		LIMIT 1
	`).Scan(
		&lastSyncAt, &lastSyncCount, &scheduleEnabled, &scheduleInterval,
		&lastImportAt, &lastImportCount, &importScheduleEnabled, &importScheduleInterval,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			_, _ = r.db.Exec(`INSERT OR IGNORE INTO feishu_devices_state (id) VALUES (1)`)
			return state, nil
		}
		return state, err
	}

	if lastSyncAt.Valid {
		state.LastSyncAt = lastSyncAt.String
	}
	if lastSyncCount.Valid {
		state.LastSyncCount = int(lastSyncCount.Int64)
	}
	state.ScheduleEnabled = scheduleEnabled.Int64 != 0
	if scheduleInterval.Valid {
		state.ScheduleInterval = scheduleInterval.String
	}
	if lastImportAt.Valid {
		state.LastImportAt = lastImportAt.String
	}
	if lastImportCount.Valid {
		state.LastImportCount = int(lastImportCount.Int64)
	}
	state.ImportScheduleEnabled = importScheduleEnabled.Int64 != 0
	if importScheduleInterval.Valid {
		state.ImportScheduleInterval = importScheduleInterval.String
	}

	return state, nil
}

func (r *FeishuDevicesRepository) UpdateState(state storage.FeishuDeviceSyncState) error {
	_, err := r.db.Exec(`INSERT OR IGNORE INTO feishu_devices_state (id) VALUES (1)`)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`
		UPDATE feishu_devices_state SET
			last_sync_at = ?, last_sync_count = ?, schedule_enabled = ?, schedule_interval = ?,
			last_import_at = ?, last_import_count = ?, import_schedule_enabled = ?, import_schedule_interval = ?
		WHERE id = 1
	`,
		state.LastSyncAt, state.LastSyncCount, boolToInt(state.ScheduleEnabled), state.ScheduleInterval,
		state.LastImportAt, state.LastImportCount, boolToInt(state.ImportScheduleEnabled), state.ImportScheduleInterval,
	)
	return err
}

func (r *FeishuDevicesRepository) GetDistinctOS() ([]string, error) {
	rows, err := r.db.Query(`SELECT DISTINCT os FROM feishu_devices WHERE os != '' ORDER BY os ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var os string
		if err := rows.Scan(&os); err != nil {
			return nil, err
		}
		result = append(result, os)
	}
	return result, rows.Err()
}

func (r *FeishuDevicesRepository) GetDistinctGroups() ([]string, error) {
	rows, err := r.db.Query(`SELECT DISTINCT groups_name FROM feishu_devices WHERE groups_name != '' ORDER BY groups_name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		result = append(result, name)
	}
	return result, rows.Err()
}

func (r *FeishuDevicesRepository) GetDistinctTrustedStatus() ([]string, error) {
	rows, err := r.db.Query(`SELECT DISTINCT trusted_status FROM feishu_devices WHERE trusted_status != '' ORDER BY trusted_status ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			return nil, err
		}
		result = append(result, status)
	}
	return result, rows.Err()
}

// FieldMappingRepository
type FieldMappingRepository struct {
	baseRepo
}

func NewFieldMappingRepository(db *sql.DB) *FieldMappingRepository {
	return &FieldMappingRepository{baseRepo: baseRepo{db: db}}
}

func (r *FieldMappingRepository) List() ([]storage.FieldMappingItem, error) {
	rows, err := r.db.Query(`SELECT id, source_field, source_label, target_field, target_label, enabled, created_at, updated_at FROM field_mappings ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]storage.FieldMappingItem, 0)
	for rows.Next() {
		var (
			item    storage.FieldMappingItem
			enabled int
		)
		if err := rows.Scan(&item.ID, &item.SourceField, &item.SourceLabel, &item.TargetField, &item.TargetLabel, &enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Enabled = enabled != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *FieldMappingRepository) Upsert(item storage.FieldMappingItem) error {
	now := time.Now().Format(time.RFC3339)
	if item.ID == "" {
		item.ID = strings.TrimSpace(item.SourceField) + "_" + strings.TrimSpace(item.TargetField)
	}
	_, err := r.db.Exec(`
		INSERT INTO field_mappings (id, source_field, source_label, target_field, target_label, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			source_field = excluded.source_field,
			source_label = excluded.source_label,
			target_field = excluded.target_field,
			target_label = excluded.target_label,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`,
		item.ID, item.SourceField, item.SourceLabel, item.TargetField, item.TargetLabel,
		boolToInt(item.Enabled), item.CreatedAt, now,
	)
	return err
}

func (r *FieldMappingRepository) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM field_mappings WHERE id = ?`, strings.TrimSpace(id))
	return err
}

func (r *FieldMappingRepository) GetEnabledMappings() ([]storage.FieldMappingItem, error) {
	rows, err := r.db.Query(`SELECT id, source_field, source_label, target_field, target_label, enabled, created_at, updated_at FROM field_mappings WHERE enabled = 1 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]storage.FieldMappingItem, 0)
	for rows.Next() {
		var (
			item    storage.FieldMappingItem
			enabled int
		)
		if err := rows.Scan(&item.ID, &item.SourceField, &item.SourceLabel, &item.TargetField, &item.TargetLabel, &enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Enabled = true
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *FieldMappingRepository) BootstrapDefaults() error {
	mappings := storage.DefaultFieldMappings
	now := time.Now().Format(time.RFC3339)
	for _, m := range mappings {
		m.ID = strings.TrimSpace(m.SourceField) + "_" + strings.TrimSpace(m.TargetField)
		m.CreatedAt = now
		_ = r.Upsert(m)
	}
	return nil
}
