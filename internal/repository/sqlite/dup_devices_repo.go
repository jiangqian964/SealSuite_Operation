package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sealsuite-operation/internal/storage"
)

// ========== DupDeviceRepository ==========

type DupDeviceRepository struct {
	baseRepo
}

func NewDupDeviceRepository(db *sql.DB) *DupDeviceRepository {
	return &DupDeviceRepository{baseRepo: baseRepo{db: db}}
}

func (r *DupDeviceRepository) ListValid() ([]storage.DupDevice, error) {
	rows, err := r.db.Query(`
		SELECT did, device_name, serial_number, hdd_serials, ssd_serials, cpu_serial,
			mem_serials, mac_addresses, updated_time, raw_json, synced_at, is_valid
		FROM dup_devices
		WHERE is_valid = 1
		ORDER BY updated_time DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []storage.DupDevice{}
	for rows.Next() {
		item, err := scanDupDevice(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *DupDeviceRepository) Count() (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM dup_devices WHERE is_valid = 1`).Scan(&count)
	return count, err
}

func (r *DupDeviceRepository) Upsert(device storage.DupDevice) error {
	now := time.Now().Format(time.RFC3339)
	if device.SyncedAt == "" {
		device.SyncedAt = now
	}

	hddJSON, _ := json.Marshal(device.HDDSerials)
	ssdJSON, _ := json.Marshal(device.SSDSerials)
	memJSON, _ := json.Marshal(device.MemSerials)
	macJSON, _ := json.Marshal(device.MacAddresses)

	_, err := r.db.Exec(`
		INSERT INTO dup_devices (did, device_name, serial_number, hdd_serials, ssd_serials, cpu_serial,
			mem_serials, mac_addresses, updated_time, raw_json, synced_at, is_valid)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(did) DO UPDATE SET
			device_name = excluded.device_name,
			serial_number = excluded.serial_number,
			hdd_serials = excluded.hdd_serials,
			ssd_serials = excluded.ssd_serials,
			cpu_serial = excluded.cpu_serial,
			mem_serials = excluded.mem_serials,
			mac_addresses = excluded.mac_addresses,
			updated_time = excluded.updated_time,
			raw_json = excluded.raw_json,
			synced_at = excluded.synced_at,
			is_valid = excluded.is_valid
	`, device.DID, device.DeviceName, device.SerialNumber, string(hddJSON), string(ssdJSON),
		device.CPUSerial, string(memJSON), string(macJSON), device.UpdatedTime,
		device.RawJSON, device.SyncedAt, boolToInt(device.IsValid))
	return err
}

func (r *DupDeviceRepository) BatchUpsert(devices []storage.DupDevice) (int, error) {
	if len(devices) == 0 {
		return 0, nil
	}
	count := 0
	for _, d := range devices {
		if err := r.Upsert(d); err == nil {
			count++
		}
	}
	return count, nil
}

func (r *DupDeviceRepository) MarkInvalid(did string) error {
	_, err := r.db.Exec(`UPDATE dup_devices SET is_valid = 0 WHERE did = ?`, did)
	return err
}

func (r *DupDeviceRepository) GetByDID(did string) (storage.DupDevice, bool, error) {
	row := r.db.QueryRow(`
		SELECT did, device_name, serial_number, hdd_serials, ssd_serials, cpu_serial,
			mem_serials, mac_addresses, updated_time, raw_json, synced_at, is_valid
		FROM dup_devices
		WHERE did = ?
	`, did)
	item, err := scanDupDevice(row)
	if err == sql.ErrNoRows {
		return item, false, nil
	}
	return item, err == nil, err
}

func (r *DupDeviceRepository) GetBySerialNumber(serialNumber string) ([]storage.DupDevice, error) {
	rows, err := r.db.Query(`
		SELECT did, device_name, serial_number, hdd_serials, ssd_serials, cpu_serial,
			mem_serials, mac_addresses, updated_time, raw_json, synced_at, is_valid
		FROM dup_devices
		WHERE serial_number = ? AND is_valid = 1
		ORDER BY updated_time DESC
	`, serialNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []storage.DupDevice{}
	for rows.Next() {
		item, err := scanDupDevice(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanDupDevice(scanner interface{ Scan(...interface{}) error }) (storage.DupDevice, error) {
	var (
		item    storage.DupDevice
		hddJSON string
		ssdJSON string
		memJSON string
		macJSON string
		isValid int
	)
	err := scanner.Scan(
		&item.DID, &item.DeviceName, &item.SerialNumber,
		&hddJSON, &ssdJSON, &item.CPUSerial, &memJSON, &macJSON,
		&item.UpdatedTime, &item.RawJSON, &item.SyncedAt, &isValid,
	)
	if err != nil {
		return item, err
	}
	json.Unmarshal([]byte(hddJSON), &item.HDDSerials)
	json.Unmarshal([]byte(ssdJSON), &item.SSDSerials)
	json.Unmarshal([]byte(memJSON), &item.MemSerials)
	json.Unmarshal([]byte(macJSON), &item.MacAddresses)
	item.IsValid = intToBool(isValid)
	return item, nil
}

// ========== DupDeviceGroupRepository ==========

type DupDeviceGroupRepository struct {
	baseRepo
}

func NewDupDeviceGroupRepository(db *sql.DB) *DupDeviceGroupRepository {
	return &DupDeviceGroupRepository{baseRepo: baseRepo{db: db}}
}

func (r *DupDeviceGroupRepository) List(status, matchLevel string, limit, offset int) ([]storage.DupDeviceGroup, int, error) {
	return r.ListWithDistinct(status, matchLevel, limit, offset, false)
}

func (r *DupDeviceGroupRepository) ListWithDistinct(status, matchLevel string, limit, offset int, distinct bool) ([]storage.DupDeviceGroup, int, error) {
	whereClauses := []string{}
	whereArgs := []interface{}{}

	if status != "" {
		whereClauses = append(whereClauses, "status = ?")
		whereArgs = append(whereArgs, status)
	}
	if matchLevel != "" {
		whereClauses = append(whereClauses, "match_level = ?")
		whereArgs = append(whereArgs, matchLevel)
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	countRow := r.db.QueryRow(`SELECT COUNT(*) FROM dup_device_groups`+whereSQL, whereArgs...)
	var total int
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT id, match_key, match_value, match_level, device_count, status, detected_at, resolved_at, retained_did
		FROM dup_device_groups` + whereSQL + ` ORDER BY detected_at DESC`

	rows, err := r.db.Query(query, whereArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []storage.DupDeviceGroup{}
	for rows.Next() {
		var g storage.DupDeviceGroup
		if err := rows.Scan(&g.ID, &g.MatchKey, &g.MatchValue, &g.MatchLevel,
			&g.DeviceCount, &g.Status, &g.DetectedAt, &g.ResolvedAt, &g.RetainedDID); err != nil {
			return nil, 0, err
		}
		items = append(items, g)
	}

	if distinct {
		seen := make(map[string]bool)
		uniqueItems := []storage.DupDeviceGroup{}
		for _, item := range items {
			if !seen[item.MatchValue] {
				seen[item.MatchValue] = true
				uniqueItems = append(uniqueItems, item)
			}
		}
		items = uniqueItems
		total = len(items)
	}

	if limit > 0 {
		start := offset
		end := offset + limit
		if start >= len(items) {
			items = []storage.DupDeviceGroup{}
		} else if end > len(items) {
			items = items[start:]
		} else {
			items = items[start:end]
		}
	}

	return items, total, rows.Err()
}

func (r *DupDeviceGroupRepository) Get(id string) (storage.DupDeviceGroup, bool, error) {
	var g storage.DupDeviceGroup
	err := r.db.QueryRow(`SELECT id, match_key, match_value, match_level, device_count, status, detected_at, resolved_at, retained_did
		FROM dup_device_groups WHERE id = ?`, id).Scan(
		&g.ID, &g.MatchKey, &g.MatchValue, &g.MatchLevel,
		&g.DeviceCount, &g.Status, &g.DetectedAt, &g.ResolvedAt, &g.RetainedDID)
	if err == sql.ErrNoRows {
		return g, false, nil
	}
	return g, err == nil, err
}

func (r *DupDeviceGroupRepository) Upsert(group storage.DupDeviceGroup) error {
	_, err := r.db.Exec(`
		INSERT INTO dup_device_groups (id, match_key, match_value, match_level, device_count, status, detected_at, resolved_at, retained_did)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			match_key = excluded.match_key,
			match_value = excluded.match_value,
			match_level = excluded.match_level,
			device_count = excluded.device_count,
			status = excluded.status,
			resolved_at = excluded.resolved_at,
			retained_did = excluded.retained_did
	`, group.ID, group.MatchKey, group.MatchValue, group.MatchLevel,
		group.DeviceCount, group.Status, group.DetectedAt, group.ResolvedAt, group.RetainedDID)
	return err
}

func (r *DupDeviceGroupRepository) CountByStatus(status string) (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM dup_device_groups WHERE status = ?`, status).Scan(&count)
	return count, err
}

func (r *DupDeviceGroupRepository) ClearPending() error {
	_, err := r.db.Exec(`DELETE FROM dup_device_groups WHERE status = 'pending'`)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`DELETE FROM dup_device_group_members WHERE group_id IN (
		SELECT id FROM dup_device_groups WHERE status = 'pending'
	)`)
	return err
}

func (r *DupDeviceGroupRepository) CountByMatchLevel(matchLevel string) (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM dup_device_groups WHERE match_level = ? AND status = 'pending'`, matchLevel).Scan(&count)
	return count, err
}

// ========== DupDeviceGroupMemberRepository ==========

type DupDeviceGroupMemberRepository struct {
	baseRepo
}

func NewDupDeviceGroupMemberRepository(db *sql.DB) *DupDeviceGroupMemberRepository {
	return &DupDeviceGroupMemberRepository{baseRepo: baseRepo{db: db}}
}

func (r *DupDeviceGroupMemberRepository) ListByGroup(groupID string) ([]storage.DupDeviceGroupMember, error) {
	rows, err := r.db.Query(`
		SELECT id, group_id, did, device_name, serial_number, hdd_serials, ssd_serials, cpu_serial,
			mem_serials, mac_addresses, updated_time, retained
		FROM dup_device_group_members
		WHERE group_id = ?
		ORDER BY updated_time DESC
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []storage.DupDeviceGroupMember{}
	for rows.Next() {
		item, err := scanDupGroupMember(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *DupDeviceGroupMemberRepository) Upsert(member storage.DupDeviceGroupMember) error {
	hddJSON, _ := json.Marshal(member.HDDSerials)
	ssdJSON, _ := json.Marshal(member.SSDSerials)
	memJSON, _ := json.Marshal(member.MemSerials)
	macJSON, _ := json.Marshal(member.MacAddresses)

	_, err := r.db.Exec(`
		INSERT INTO dup_device_group_members (id, group_id, did, device_name, serial_number,
			hdd_serials, ssd_serials, cpu_serial, mem_serials, mac_addresses, updated_time, retained)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			device_name = excluded.device_name,
			hdd_serials = excluded.hdd_serials,
			ssd_serials = excluded.ssd_serials,
			cpu_serial = excluded.cpu_serial,
			mem_serials = excluded.mem_serials,
			mac_addresses = excluded.mac_addresses,
			updated_time = excluded.updated_time,
			retained = excluded.retained
	`, member.ID, member.GroupID, member.DID, member.DeviceName, member.SerialNumber,
		string(hddJSON), string(ssdJSON), member.CPUSerial, string(memJSON), string(macJSON),
		member.UpdatedTime, boolToInt(member.Retained))
	return err
}

func (r *DupDeviceGroupMemberRepository) DeleteByGroup(groupID string) error {
	_, err := r.db.Exec(`DELETE FROM dup_device_group_members WHERE group_id = ?`, groupID)
	return err
}

func scanDupGroupMember(scanner interface{ Scan(...interface{}) error }) (storage.DupDeviceGroupMember, error) {
	var (
		item     storage.DupDeviceGroupMember
		hddJSON  string
		ssdJSON  string
		memJSON  string
		macJSON  string
		retained int
	)
	err := scanner.Scan(
		&item.ID, &item.GroupID, &item.DID, &item.DeviceName, &item.SerialNumber,
		&hddJSON, &ssdJSON, &item.CPUSerial, &memJSON, &macJSON,
		&item.UpdatedTime, &retained,
	)
	if err != nil {
		return item, err
	}
	json.Unmarshal([]byte(hddJSON), &item.HDDSerials)
	json.Unmarshal([]byte(ssdJSON), &item.SSDSerials)
	json.Unmarshal([]byte(memJSON), &item.MemSerials)
	json.Unmarshal([]byte(macJSON), &item.MacAddresses)
	item.Retained = intToBool(retained)
	return item, nil
}

// ========== DupDeviceCleanupLogRepository ==========

type DupDeviceCleanupLogRepository struct {
	baseRepo
}

func NewDupDeviceCleanupLogRepository(db *sql.DB) *DupDeviceCleanupLogRepository {
	return &DupDeviceCleanupLogRepository{baseRepo: baseRepo{db: db}}
}

func (r *DupDeviceCleanupLogRepository) List(limit, offset int) ([]storage.DupDeviceCleanupLog, int, error) {
	countRow := r.db.QueryRow(`SELECT COUNT(*) FROM dup_device_cleanup_logs`)
	var total int
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT id, group_id, did, device_name, serial_number, cleanup_type, cleanup_status, error_msg, cleaned_at
		FROM dup_device_cleanup_logs ORDER BY cleaned_at DESC`
	args := []interface{}{}
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []storage.DupDeviceCleanupLog{}
	for rows.Next() {
		var l storage.DupDeviceCleanupLog
		if err := rows.Scan(&l.ID, &l.GroupID, &l.DID, &l.DeviceName, &l.SerialNumber,
			&l.CleanupType, &l.CleanupStatus, &l.ErrorMsg, &l.CleanedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, l)
	}
	return items, total, rows.Err()
}

func (r *DupDeviceCleanupLogRepository) Add(log storage.DupDeviceCleanupLog) error {
	_, err := r.db.Exec(`
		INSERT INTO dup_device_cleanup_logs (id, group_id, did, device_name, serial_number, cleanup_type, cleanup_status, error_msg, cleaned_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, log.ID, log.GroupID, log.DID, log.DeviceName, log.SerialNumber,
		log.CleanupType, log.CleanupStatus, log.ErrorMsg, log.CleanedAt)
	return err
}

func (r *DupDeviceCleanupLogRepository) Count() (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM dup_device_cleanup_logs`).Scan(&count)
	return count, err
}

func (r *DupDeviceCleanupLogRepository) CountByStatus(status string) (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM dup_device_cleanup_logs WHERE cleanup_status = ?`, status).Scan(&count)
	return count, err
}

func (r *DupDeviceCleanupLogRepository) UpdateStatus(id, status, errorMsg string) error {
	_, err := r.db.Exec(`
		UPDATE dup_device_cleanup_logs SET cleanup_status = ?, error_msg = ?, cleaned_at = ? WHERE id = ?
	`, status, errorMsg, time.Now().Format(time.RFC3339), id)
	return err
}

func (r *DupDeviceCleanupLogRepository) ListByStatus(status string, limit, offset int) ([]storage.DupDeviceCleanupLog, int, error) {
	whereSQL := ""
	whereArgs := []interface{}{}
	if status != "" {
		whereSQL = " WHERE cleanup_status = ?"
		whereArgs = append(whereArgs, status)
	}

	countRow := r.db.QueryRow(`SELECT COUNT(*) FROM dup_device_cleanup_logs`+whereSQL, whereArgs...)
	var total int
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT id, group_id, did, device_name, serial_number, cleanup_type, cleanup_status, error_msg, cleaned_at
		FROM dup_device_cleanup_logs` + whereSQL + ` ORDER BY cleaned_at DESC`
	args := append([]interface{}{}, whereArgs...)
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []storage.DupDeviceCleanupLog{}
	for rows.Next() {
		var l storage.DupDeviceCleanupLog
		if err := rows.Scan(&l.ID, &l.GroupID, &l.DID, &l.DeviceName, &l.SerialNumber,
			&l.CleanupType, &l.CleanupStatus, &l.ErrorMsg, &l.CleanedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, l)
	}
	return items, total, rows.Err()
}

// ========== DupDeviceTaskStateRepository ==========

type DupDeviceTaskStateRepository struct {
	baseRepo
}

func NewDupDeviceTaskStateRepository(db *sql.DB) *DupDeviceTaskStateRepository {
	return &DupDeviceTaskStateRepository{baseRepo: baseRepo{db: db}}
}

func (r *DupDeviceTaskStateRepository) Get() (storage.DupDeviceTaskState, error) {
	var state storage.DupDeviceTaskState
	var syncEnabled, detectEnabled int
	err := r.db.QueryRow(`
		SELECT id, last_sync_at, last_sync_count, last_sync_error,
			last_detect_at, last_detect_groups, last_detect_error,
			sync_enabled, detect_enabled
		FROM dup_device_task_state WHERE id = 'state'
	`).Scan(
		&state.ID, &state.LastSyncAt, &state.LastSyncCount, &state.LastSyncError,
		&state.LastDetectAt, &state.LastDetectGroups, &state.LastDetectError,
		&syncEnabled, &detectEnabled,
	)
	if err == sql.ErrNoRows {
		state.ID = "state"
		state.SyncEnabled = true
		state.DetectEnabled = true
		return state, nil
	}
	state.SyncEnabled = intToBool(syncEnabled)
	state.DetectEnabled = intToBool(detectEnabled)
	return state, err
}

func (r *DupDeviceTaskStateRepository) Save(state storage.DupDeviceTaskState) error {
	_, err := r.db.Exec(`
		INSERT INTO dup_device_task_state (id, last_sync_at, last_sync_count, last_sync_error,
			last_detect_at, last_detect_groups, last_detect_error, sync_enabled, detect_enabled)
		VALUES ('state', ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			last_sync_at = excluded.last_sync_at,
			last_sync_count = excluded.last_sync_count,
			last_sync_error = excluded.last_sync_error,
			last_detect_at = excluded.last_detect_at,
			last_detect_groups = excluded.last_detect_groups,
			last_detect_error = excluded.last_detect_error,
			sync_enabled = excluded.sync_enabled,
			detect_enabled = excluded.detect_enabled
	`, state.LastSyncAt, state.LastSyncCount, state.LastSyncError,
		state.LastDetectAt, state.LastDetectGroups, state.LastDetectError,
		boolToInt(state.SyncEnabled), boolToInt(state.DetectEnabled))
	return err
}

func (r *DupDeviceTaskStateRepository) UpdateSyncEnabled(enabled bool) error {
	_, err := r.db.Exec(`
		UPDATE dup_device_task_state SET sync_enabled = ? WHERE id = 'state'
	`, boolToInt(enabled))
	if err == nil {
		return nil
	}
	_, err = r.db.Exec(`
		INSERT INTO dup_device_task_state (id, sync_enabled) VALUES ('state', ?)
	`, boolToInt(enabled))
	return err
}

func (r *DupDeviceTaskStateRepository) UpdateDetectEnabled(enabled bool) error {
	_, err := r.db.Exec(`
		UPDATE dup_device_task_state SET detect_enabled = ? WHERE id = 'state'
	`, boolToInt(enabled))
	if err == nil {
		return nil
	}
	_, err = r.db.Exec(`
		INSERT INTO dup_device_task_state (id, detect_enabled) VALUES ('state', ?)
	`, boolToInt(enabled))
	return err
}

// ========== Helpers ==========

func genDupID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}
