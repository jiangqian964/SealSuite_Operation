package runner

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"sealsuite-operation/internal/logger"
	sqliteRepo "sealsuite-operation/internal/repository/sqlite"
	"sealsuite-operation/internal/storage"

	"go.uber.org/zap"
)

// ==================== 设备同步 ====================

// SyncDupDevices 从飞连同步全量设备信息
func (r *Runner) SyncDupDevices() (*storage.DupSyncResult, error) {
	result := &storage.DupSyncResult{
		SyncAt: time.Now().Format(time.RFC3339),
	}

	// 1. 调用飞连 API 获取全量设备
	logger.Info("[重复设备] 开始同步全量设备...")
	devices, err := r.client.SearchAllDevices()
	if err != nil {
		result.Error = err.Error()
		// 更新任务状态
		r.saveDupSyncState(0, err.Error())
		return result, fmt.Errorf("sync devices: %w", err)
	}

	logger.Info("[重复设备] 获取到设备", zap.Int("count", len(devices)))

	// 2. 转换并写入数据库
	syncedCount := 0
	for _, d := range devices {
		dupDevice := storage.DupDevice{
			DID:          d.DID,
			DeviceName:   d.DeviceName,
			SerialNumber: d.SerialNumber,
			HDDSerials:   d.HDDSerials,
			SSDSerials:   d.SSDSerials,
			CPUSerial:    d.CPUSerial,
			MemSerials:   d.MemSerials,
			MacAddresses: d.MacAddresses,
			UpdatedTime:  d.UpdatedTime,
			RawJSON:      d.RawJSON,
			SyncedAt:     result.SyncAt,
			IsValid:      true,
		}
		if err := r.dupDeviceRepo.Upsert(dupDevice); err != nil {
			logger.Warn("[重复设备] 写入设备失败",
				zap.String("did", d.DID),
				zap.Error(err),
			)
			continue
		}
		syncedCount++
	}

	result.SyncedCount = syncedCount
	logger.Info("[重复设备] 同步完成", zap.Int("synced", syncedCount))

	// 3. 更新任务状态
	r.saveDupSyncState(syncedCount, "")

	return result, nil
}

// saveDupSyncState 保存同步状态
func (r *Runner) saveDupSyncState(count int, errMsg string) {
	state, _ := r.dupTaskStateRepo.Get()
	state.LastSyncAt = time.Now().Format(time.RFC3339)
	state.LastSyncCount = count
	state.LastSyncError = errMsg
	_ = r.dupTaskStateRepo.Save(state)
}

// ==================== 重复检测 ====================

// DetectDupDevices 检测重复设备
func (r *Runner) DetectDupDevices() (*storage.DupDetectResult, error) {
	result := &storage.DupDetectResult{
		DetectAt: time.Now().Format(time.RFC3339),
	}

	// 1. 获取所有有效设备
	devices, err := r.dupDeviceRepo.ListValid()
	if err != nil {
		result.Error = err.Error()
		r.saveDupDetectState(0, 0, err.Error())
		return result, fmt.Errorf("list valid devices: %w", err)
	}

	logger.Info("[重复设备] 开始检测重复设备", zap.Int("total", len(devices)))

	// 2. 按序列号分组
	serialGroups := make(map[string][]storage.DupDevice)
	for _, d := range devices {
		if d.SerialNumber == "" {
			continue // 没有序列号的跳过
		}
		serialGroups[d.SerialNumber] = append(serialGroups[d.SerialNumber], d)
	}

	// 3. 清理旧的 pending 分组
	if err := r.dupGroupRepo.ClearPending(); err != nil {
		logger.Warn("[重复设备] 清理旧分组失败", zap.Error(err))
	}

	// 4. 检测重复组
	highMatchCount := 0
	serialOnlyCount := 0
	detectedGroups := 0

	for serial, group := range serialGroups {
		if len(group) < 2 {
			continue // 只有一个设备，不是重复
		}

		// 计算匹配级别
		matchLevel := calcMatchLevel(group)

		groupID := fmt.Sprintf("dup_%s_%d", serial, time.Now().UnixNano())

		// 按更新时间降序排序
		sort.Slice(group, func(i, j int) bool {
			return group[i].UpdatedTime > group[j].UpdatedTime
		})

		// 保存分组
		dupGroup := storage.DupDeviceGroup{
			ID:          groupID,
			MatchKey:    "serial_number",
			MatchValue:  serial,
			MatchLevel:  matchLevel,
			DeviceCount: len(group),
			Status:      storage.DupGroupStatusPending,
			DetectedAt:  result.DetectAt,
		}
		if err := r.dupGroupRepo.Upsert(dupGroup); err != nil {
			logger.Warn("[重复设备] 保存分组失败", zap.Error(err))
			continue
		}

		// 保存组成员
		for idx, d := range group {
			member := storage.DupDeviceGroupMember{
				ID:           fmt.Sprintf("%s_%d", groupID, idx),
				GroupID:      groupID,
				DID:          d.DID,
				DeviceName:   d.DeviceName,
				SerialNumber: d.SerialNumber,
				HDDSerials:   d.HDDSerials,
				SSDSerials:   d.SSDSerials,
				CPUSerial:    d.CPUSerial,
				MemSerials:   d.MemSerials,
				MacAddresses: d.MacAddresses,
				UpdatedTime:  d.UpdatedTime,
				Retained:     idx == 0, // 默认最新的为保留
			}
			if err := r.dupGroupMemberRepo.Upsert(member); err != nil {
				logger.Warn("[重复设备] 保存组成员失败", zap.Error(err))
			}
		}

		detectedGroups++
		if matchLevel == storage.DupMatchLevelHighMatch {
			highMatchCount++
		} else {
			serialOnlyCount++
		}
	}

	result.DetectedGroups = detectedGroups
	result.HighMatchCount = highMatchCount
	result.SerialOnlyCount = serialOnlyCount

	logger.Info("[重复设备] 检测完成",
		zap.Int("groups", detectedGroups),
		zap.Int("high_match", highMatchCount),
		zap.Int("serial_only", serialOnlyCount),
	)

	// 5. 更新任务状态
	r.saveDupDetectState(detectedGroups, highMatchCount, "")

	return result, nil
}

// calcMatchLevel 计算设备组的匹配级别
// 高匹配：序列号 + 至少2个硬件标识一致（总共≥3个匹配）
// 仅序列号：仅序列号一致
func calcMatchLevel(devices []storage.DupDevice) string {
	if len(devices) < 2 {
		return storage.DupMatchLevelSerialOnly
	}

	// 统计各个维度的匹配数
	matchCount := 1 // 序列号已经匹配，算1个

	// HDD 序列号匹配
	if allSlicesEqual(devices, func(d storage.DupDevice) []string { return d.HDDSerials }) {
		matchCount++
	}

	// SSD 序列号匹配
	if allSlicesEqual(devices, func(d storage.DupDevice) []string { return d.SSDSerials }) {
		matchCount++
	}

	// CPU 序列号匹配
	if allValuesEqual(devices, func(d storage.DupDevice) string { return d.CPUSerial }) {
		matchCount++
	}

	// 内存序列号匹配
	if allSlicesEqual(devices, func(d storage.DupDevice) []string { return d.MemSerials }) {
		matchCount++
	}

	// MAC 地址匹配
	if allSlicesEqual(devices, func(d storage.DupDevice) []string { return d.MacAddresses }) {
		matchCount++
	}

	// 总共≥3个匹配（包括序列号），即除序列号外至少2个也匹配
	if matchCount >= 3 {
		return storage.DupMatchLevelHighMatch
	}

	return storage.DupMatchLevelSerialOnly
}

// allValuesEqual 检查所有设备的指定字段值是否相等
func allValuesEqual(devices []storage.DupDevice, getter func(storage.DupDevice) string) bool {
	if len(devices) < 2 {
		return true
	}
	first := getter(devices[0])
	for _, d := range devices[1:] {
		if getter(d) != first {
			return false
		}
	}
	return true
}

// allSlicesEqual 检查所有设备的指定切片字段是否相等
func allSlicesEqual(devices []storage.DupDevice, getter func(storage.DupDevice) []string) bool {
	if len(devices) < 2 {
		return true
	}
	first := getter(devices[0])
	for _, d := range devices[1:] {
		if !stringSlicesEqual(first, getter(d)) {
			return false
		}
	}
	return true
}

// stringSlicesEqual 比较两个字符串切片是否相等
func stringSlicesEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	sortedA := append([]string{}, a...)
	sortedB := append([]string{}, b...)
	sort.Strings(sortedA)
	sort.Strings(sortedB)
	for i := range sortedA {
		if sortedA[i] != sortedB[i] {
			return false
		}
	}
	return true
}

// saveDupDetectState 保存检测状态
func (r *Runner) saveDupDetectState(groups int, highMatch int, errMsg string) {
	state, _ := r.dupTaskStateRepo.Get()
	state.LastDetectAt = time.Now().Format(time.RFC3339)
	state.LastDetectGroups = groups
	state.LastDetectError = errMsg
	_ = r.dupTaskStateRepo.Save(state)
}

// ==================== 自动清理 ====================

// AutoCleanupHighMatchDups 自动清理高匹配的重复设备
func (r *Runner) AutoCleanupHighMatchDups() (*storage.DupCleanupResult, error) {
	result := &storage.DupCleanupResult{
		CleanupAt: time.Now().Format(time.RFC3339),
	}

	// 1. 获取所有 pending 的高匹配分组
	groups, _, err := r.dupGroupRepo.List(
		storage.DupGroupStatusPending,
		storage.DupMatchLevelHighMatch,
		0, 0, // 不分页，获取全部
	)
	if err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("list high match groups: %w", err)
	}

	logger.Info("[重复设备] 开始自动清理高匹配重复设备", zap.Int("groups", len(groups)))

	cleanedCount := 0
	failedCount := 0

	for _, group := range groups {
		// 获取组成员
		members, err := r.dupGroupMemberRepo.ListByGroup(group.ID)
		if err != nil {
			logger.Warn("[重复设备] 获取组成员失败", zap.String("group_id", group.ID), zap.Error(err))
			continue
		}

		if len(members) < 2 {
			continue
		}

		// 找出保留设备（retained=true 的），同时收集待清理设备
		var retainedDID string
		var toCleanDIDs []string
		for _, m := range members {
			if m.Retained {
				retainedDID = m.DID
			} else {
				toCleanDIDs = append(toCleanDIDs, m.DID)
			}
		}

		// 如果没有标记 retained，则按 updated_time 降序，第一个为保留
		if retainedDID == "" {
			if len(members) > 0 {
				retainedDID = members[0].DID
				toCleanDIDs = toCleanDIDs[1:] // 第一个是保留的，从待清理列表中移除
			}
		}

		if len(toCleanDIDs) == 0 {
			continue
		}

		// 安全校验：确保保留设备不在待清理列表中
		for i, did := range toCleanDIDs {
			if did == retainedDID {
				logger.Warn("[重复设备] 保留设备出现在待清理列表中，已移除",
					zap.String("group_id", group.ID),
					zap.String("retained_did", retainedDID),
				)
				toCleanDIDs = append(toCleanDIDs[:i], toCleanDIDs[i+1:]...)
				break
			}
		}

		logger.Info("[重复设备] 清理分组",
			zap.String("group_id", group.ID),
			zap.String("serial", group.MatchValue),
			zap.String("retained_did", retainedDID),
			zap.Strings("to_clean_dids", toCleanDIDs),
		)

		// 执行清理
		cleanResults := r.cleanupDevices(group.ID, toCleanDIDs, storage.DupCleanupTypeAuto)

		// 统计结果
		successCount := 0
		for _, cr := range cleanResults {
			if cr.Status == storage.DupCleanupStatusSuccess {
				successCount++
				cleanedCount++
			} else {
				failedCount++
			}
		}

		// 更新分组状态
		if successCount == len(toCleanDIDs) {
			group.Status = storage.DupGroupStatusAutoResolved
			group.ResolvedAt = result.CleanupAt
			group.RetainedDID = retainedDID
			_ = r.dupGroupRepo.Upsert(group)
		}
	}

	result.CleanedCount = cleanedCount
	result.FailedCount = failedCount

	logger.Info("[重复设备] 自动清理完成",
		zap.Int("cleaned", cleanedCount),
		zap.Int("failed", failedCount),
	)

	return result, nil
}

// isDeviceNotExistError 判断是否是设备不存在的错误
func isDeviceNotExistError(errMsg string) bool {
	keywords := []string{
		"设备不存在",
		"not found",
		"Not Found",
		"不存在",
	}
	for _, kw := range keywords {
		if containsIgnoreCase(errMsg, kw) {
			return true
		}
	}
	return false
}

// containsIgnoreCase 忽略大小写的字符串包含判断
func containsIgnoreCase(s, substr string) bool {
	sLower := strings.ToLower(s)
	subLower := strings.ToLower(substr)
	return strings.Contains(sLower, subLower)
}

// safeDeleteDevices 安全删除设备，带容错机制
// 如果批量删除返回整体错误，则回退到逐个删除以精确定位每个设备的结果
func (r *Runner) safeDeleteDevices(dids []string) ([]string, map[string]string, error) {
	if len(dids) == 0 {
		return []string{}, map[string]string{}, nil
	}

	// 先尝试批量删除
	successDIDs, failedMap, err := r.client.BatchDeleteInvalidDevices(dids)
	if err == nil {
		return successDIDs, failedMap, nil
	}

	logger.Warn("[重复设备] 批量删除失败，尝试逐个删除",
		zap.Int("total", len(dids)),
		zap.Error(err),
	)

	// 批量失败，逐个删除
	finalSuccess := []string{}
	finalFailed := make(map[string]string)

	for _, did := range dids {
		singleSuccess, singleFailed, singleErr := r.client.BatchDeleteInvalidDevices([]string{did})
		if singleErr != nil {
			// 单个也失败，判断是否是设备不存在
			errMsg := singleErr.Error()
			if isDeviceNotExistError(errMsg) {
				// 设备不存在，视为成功
				logger.Info("[重复设备] 设备不存在，视为删除成功",
					zap.String("did", did),
					zap.Error(singleErr),
				)
				finalSuccess = append(finalSuccess, did)
			} else {
				finalFailed[did] = errMsg
			}
		} else {
			// 单个调用成功（code=0），处理返回结果
			for _, s := range singleSuccess {
				finalSuccess = append(finalSuccess, s)
			}
			for d, msg := range singleFailed {
				if isDeviceNotExistError(msg) {
					finalSuccess = append(finalSuccess, d)
				} else {
					finalFailed[d] = msg
				}
			}
		}
	}

	return finalSuccess, finalFailed, nil
}

// cleanupDevices 清理设备（改状态 + 删除）
func (r *Runner) cleanupDevices(groupID string, dids []string, cleanupType string) []struct {
	DID    string
	Status string
	Error  string
} {
	var results []struct {
		DID    string
		Status string
		Error  string
	}

	if len(dids) == 0 {
		return results
	}

	// 1. 批量修改状态为失效
	err := r.client.BatchUpdateDeviceStatus(dids, "invalid")
	if err != nil {
		logger.Warn("[重复设备] 批量修改设备状态失败", zap.Error(err))
		// 如果是设备不存在的错误，视为成功
		if isDeviceNotExistError(err.Error()) {
			logger.Info("[重复设备] 设备已不存在，视为成功", zap.Error(err))
			for _, did := range dids {
				results = append(results, struct {
					DID    string
					Status string
					Error  string
				}{DID: did, Status: storage.DupCleanupStatusSuccess, Error: "device not exist, skipped"})
				r.addCleanupLog(groupID, did, cleanupType, storage.DupCleanupStatusSuccess, "device not exist, skipped")
				_ = r.dupDeviceRepo.MarkInvalid(did)
			}
			return results
		}
		// 单个设备记录失败
		for _, did := range dids {
			results = append(results, struct {
				DID    string
				Status string
				Error  string
			}{DID: did, Status: storage.DupCleanupStatusFailed, Error: fmt.Sprintf("update status: %v", err)})

			// 记录清理日志
			r.addCleanupLog(groupID, did, cleanupType, storage.DupCleanupStatusFailed, fmt.Sprintf("update status: %v", err))
		}
		return results
	}

	// 2. 批量删除失效设备（带容错：批量失败则逐个删除）
	successDIDs, failedMap, err := r.safeDeleteDevices(dids)
	if err != nil {
		logger.Warn("[重复设备] 删除设备失败", zap.Error(err))
		for _, did := range dids {
			results = append(results, struct {
				DID    string
				Status string
				Error  string
			}{DID: did, Status: storage.DupCleanupStatusFailed, Error: fmt.Sprintf("delete device: %v", err)})

			r.addCleanupLog(groupID, did, cleanupType, storage.DupCleanupStatusFailed, fmt.Sprintf("delete device: %v", err))
		}
		return results
	}

	for _, did := range successDIDs {
		results = append(results, struct {
			DID    string
			Status string
			Error  string
		}{DID: did, Status: storage.DupCleanupStatusSuccess, Error: ""})
		r.addCleanupLog(groupID, did, cleanupType, storage.DupCleanupStatusSuccess, "")
		_ = r.dupDeviceRepo.MarkInvalid(did)
	}
	for did, errMsg := range failedMap {
		// 如果是设备不存在的错误，视为成功
		if isDeviceNotExistError(errMsg) {
			logger.Info("[重复设备] 设备已不存在，视为成功", zap.String("did", did), zap.String("error", errMsg))
			results = append(results, struct {
				DID    string
				Status string
				Error  string
			}{DID: did, Status: storage.DupCleanupStatusSuccess, Error: "device not exist, skipped"})
			r.addCleanupLog(groupID, did, cleanupType, storage.DupCleanupStatusSuccess, "device not exist, skipped")
			_ = r.dupDeviceRepo.MarkInvalid(did)
		} else {
			results = append(results, struct {
				DID    string
				Status string
				Error  string
			}{DID: did, Status: storage.DupCleanupStatusFailed, Error: fmt.Sprintf("delete device: %v", errMsg)})
			r.addCleanupLog(groupID, did, cleanupType, storage.DupCleanupStatusFailed, fmt.Sprintf("delete device: %v", errMsg))
		}
	}

	return results
}

// addCleanupLog 添加清理日志
func (r *Runner) addCleanupLog(groupID, did, cleanupType, status, errorMsg string) {
	// 获取设备信息用于日志
	device, exists, _ := r.dupDeviceRepo.GetByDID(did)
	deviceName := ""
	serialNumber := ""
	if exists {
		deviceName = device.DeviceName
		serialNumber = device.SerialNumber
	}

	logEntry := storage.DupDeviceCleanupLog{
		ID:            fmt.Sprintf("cleanup_%d_%s", time.Now().UnixNano(), did),
		GroupID:       groupID,
		DID:           did,
		DeviceName:    deviceName,
		SerialNumber:  serialNumber,
		CleanupType:   cleanupType,
		CleanupStatus: status,
		ErrorMsg:      errorMsg,
		CleanedAt:     time.Now().Format(time.RFC3339),
	}
	_ = r.dupCleanupLogRepo.Add(logEntry)
}

// ==================== 手动清理 ====================

// ManualResolveDupGroup 手动处理重复分组，保留指定设备
func (r *Runner) ManualResolveDupGroup(groupID, retainedDID string) (*storage.DupCleanupResult, error) {
	result := &storage.DupCleanupResult{
		CleanupAt: time.Now().Format(time.RFC3339),
	}

	// 1. 验证分组存在
	group, exists, err := r.dupGroupRepo.Get(groupID)
	if err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("get group: %w", err)
	}
	if !exists {
		result.Error = "group not found"
		return result, fmt.Errorf("group not found: %s", groupID)
	}

	// 2. 获取组成员
	members, err := r.dupGroupMemberRepo.ListByGroup(groupID)
	if err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("list group members: %w", err)
	}

	// 3. 验证 retainedDID 在组内
	found := false
	var toCleanDIDs []string
	var deviceName string
	var serialNumber string

	for _, m := range members {
		if m.DID == retainedDID {
			found = true
			deviceName = m.DeviceName
			serialNumber = m.SerialNumber
		} else {
			toCleanDIDs = append(toCleanDIDs, m.DID)
		}
	}

	if !found {
		result.Error = "retained DID not in group"
		return result, fmt.Errorf("retained DID %s not in group %s", retainedDID, groupID)
	}

	if len(toCleanDIDs) == 0 {
		result.Error = "no devices to clean"
		return result, fmt.Errorf("no devices to clean in group %s", groupID)
	}

	logger.Info("[重复设备] 手动清理分组",
		zap.String("group_id", groupID),
		zap.String("retained_did", retainedDID),
		zap.Int("clean_count", len(toCleanDIDs)),
	)

	// 4. 执行清理
	cleanResults := r.cleanupDevices(groupID, toCleanDIDs, storage.DupCleanupTypeManual)

	// 5. 统计结果
	cleanedCount := 0
	failedCount := 0
	for _, cr := range cleanResults {
		if cr.Status == storage.DupCleanupStatusSuccess {
			cleanedCount++
		} else {
			failedCount++
		}
	}

	result.CleanedCount = cleanedCount
	result.FailedCount = failedCount

	// 6. 更新分组状态
	if cleanedCount > 0 {
		group.Status = storage.DupGroupStatusManualResolved
		group.ResolvedAt = result.CleanupAt
		group.RetainedDID = retainedDID
		_ = r.dupGroupRepo.Upsert(group)

		// 更新成员的保留状态
		for _, m := range members {
			m.Retained = (m.DID == retainedDID)
			_ = r.dupGroupMemberRepo.Upsert(m)
		}
	}

	// 保留设备信息记录（用于日志）
	_ = deviceName
	_ = serialNumber

	return result, nil
}

// ==================== 全流程 ====================

// RunDupDeviceFullProcess 执行完整流程：同步 + 检测 + 自动清理
func (r *Runner) RunDupDeviceFullProcess() (*storage.DupFullResult, error) {
	result := &storage.DupFullResult{}

	logger.Info("[重复设备] 开始执行完整流程...")

	// 1. 同步设备
	syncResult, err := r.SyncDupDevices()
	if err != nil {
		result.Error = fmt.Sprintf("sync failed: %v", err)
		return result, err
	}
	result.SyncedCount = syncResult.SyncedCount

	// 2. 检测重复
	detectResult, err := r.DetectDupDevices()
	if err != nil {
		result.Error = fmt.Sprintf("detect failed: %v", err)
		return result, err
	}
	result.DetectedGroups = detectResult.DetectedGroups

	// 3. 自动清理高匹配
	cleanupResult, err := r.AutoCleanupHighMatchDups()
	if err != nil {
		result.Error = fmt.Sprintf("cleanup failed: %v", err)
		return result, err
	}
	result.CleanedCount = cleanupResult.CleanedCount

	logger.Info("[重复设备] 完整流程执行完成",
		zap.Int("synced", result.SyncedCount),
		zap.Int("groups", result.DetectedGroups),
		zap.Int("cleaned", result.CleanedCount),
	)

	return result, nil
}

// RetryFailedCleanup 重试失败的删除任务
func (r *Runner) RetryFailedCleanup() (*storage.DupCleanupResult, error) {
	result := &storage.DupCleanupResult{
		CleanupAt: time.Now().Format(time.RFC3339),
	}

	logger.Info("[重复设备] 开始重试失败的删除任务...")

	failedLogs, _, err := r.dupCleanupLogRepo.ListByStatus(storage.DupCleanupStatusFailed, 0, 0)
	if err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("list failed logs: %w", err)
	}

	if len(failedLogs) == 0 {
		logger.Info("[重复设备] 没有失败的删除任务")
		return result, nil
	}

	logger.Info("[重复设备] 发现失败任务", zap.Int("count", len(failedLogs)))

	cleanedCount := 0
	failedCount := 0
	batchSize := 100

	groupIDs := make(map[string]bool)

	for i := 0; i < len(failedLogs); i += batchSize {
		end := i + batchSize
		if end > len(failedLogs) {
			end = len(failedLogs)
		}
		batch := failedLogs[i:end]

		didToLog := make(map[string]storage.DupDeviceCleanupLog)
		var dids []string
		for _, log := range batch {
			if _, exists := didToLog[log.DID]; !exists {
				didToLog[log.DID] = log
				dids = append(dids, log.DID)
				groupIDs[log.GroupID] = true
			}
		}

		if len(dids) == 0 {
			continue
		}

		logger.Info("[重复设备] 批量处理", zap.Int("batch_start", i), zap.Int("batch_size", len(dids)))

		err := r.client.BatchUpdateDeviceStatus(dids, "invalid")
		if err != nil {
			logger.Warn("[重复设备] 批量修改状态失败", zap.Error(err))
			// 如果是设备不存在的错误，视为成功
			if isDeviceNotExistError(err.Error()) {
				logger.Info("[重复设备] 设备已不存在，视为成功", zap.Error(err))
				for _, log := range batch {
					if _, ok := didToLog[log.DID]; ok {
						_ = r.dupCleanupLogRepo.UpdateStatus(log.ID, storage.DupCleanupStatusSuccess, "device not exist, skipped")
						_ = r.dupDeviceRepo.MarkInvalid(log.DID)
						cleanedCount++
					}
				}
				continue
			}
			for _, log := range batch {
				errMsg := fmt.Sprintf("retry update status: %v", err)
				_ = r.dupCleanupLogRepo.UpdateStatus(log.ID, storage.DupCleanupStatusFailed, errMsg)
				failedCount++
			}
			continue
		}

		successDIDs, failedMap, err := r.safeDeleteDevices(dids)
		if err != nil {
			logger.Warn("[重复设备] 批量删除失败", zap.Error(err))
			// 如果是设备不存在的错误，视为成功
			if isDeviceNotExistError(err.Error()) {
				logger.Info("[重复设备] 设备已不存在，视为成功", zap.Error(err))
				for _, log := range batch {
					if _, ok := didToLog[log.DID]; ok {
						_ = r.dupCleanupLogRepo.UpdateStatus(log.ID, storage.DupCleanupStatusSuccess, "device not exist, skipped")
						_ = r.dupDeviceRepo.MarkInvalid(log.DID)
						cleanedCount++
					}
				}
				continue
			}
			for _, log := range batch {
				errMsg := fmt.Sprintf("retry delete device: %v", err)
				_ = r.dupCleanupLogRepo.UpdateStatus(log.ID, storage.DupCleanupStatusFailed, errMsg)
				failedCount++
			}
			continue
		}
		for _, did := range successDIDs {
			if log, ok := didToLog[did]; ok {
				logger.Info("[重复设备] 重试删除成功", zap.String("did", did))
				_ = r.dupCleanupLogRepo.UpdateStatus(log.ID, storage.DupCleanupStatusSuccess, "")
				_ = r.dupDeviceRepo.MarkInvalid(did)
				cleanedCount++
			}
		}
		for did, errMsg := range failedMap {
			if log, ok := didToLog[did]; ok {
				// 如果是设备不存在的错误，视为成功
				if isDeviceNotExistError(errMsg) {
					logger.Info("[重复设备] 设备已不存在，视为成功", zap.String("did", did), zap.String("error", errMsg))
					_ = r.dupCleanupLogRepo.UpdateStatus(log.ID, storage.DupCleanupStatusSuccess, "device not exist, skipped")
					_ = r.dupDeviceRepo.MarkInvalid(did)
					cleanedCount++
				} else {
					fullMsg := fmt.Sprintf("retry delete device: %v", errMsg)
					logger.Warn("[重复设备] 重试删除设备失败", zap.String("did", did), zap.String("error", errMsg))
					_ = r.dupCleanupLogRepo.UpdateStatus(log.ID, storage.DupCleanupStatusFailed, fullMsg)
					failedCount++
				}
			}
		}
	}

	for groupID := range groupIDs {
		r.updateGroupStatusAfterRetry(groupID, result.CleanupAt)
	}

	result.CleanedCount = cleanedCount
	result.FailedCount = failedCount

	logger.Info("[重复设备] 重试完成",
		zap.Int("cleaned", cleanedCount),
		zap.Int("failed", failedCount),
	)

	return result, nil
}

func (r *Runner) updateGroupStatusAfterRetry(groupID, cleanupAt string) {
	group, exists, err := r.dupGroupRepo.Get(groupID)
	if err != nil || !exists {
		return
	}

	if group.Status != storage.DupGroupStatusPending {
		return
	}

	members, err := r.dupGroupMemberRepo.ListByGroup(groupID)
	if err != nil {
		return
	}

	var retainedDID string
	var toCleanDIDs []string
	for _, m := range members {
		if m.Retained {
			retainedDID = m.DID
		} else {
			toCleanDIDs = append(toCleanDIDs, m.DID)
		}
	}

	if retainedDID == "" && len(members) > 0 {
		retainedDID = members[0].DID
	}

	allCleaned := true
	for _, did := range toCleanDIDs {
		logs, _, err := r.dupCleanupLogRepo.ListByStatus(storage.DupCleanupStatusFailed, 0, 0)
		if err != nil {
			return
		}
		for _, log := range logs {
			if log.DID == did {
				allCleaned = false
				break
			}
		}
		if !allCleaned {
			break
		}
	}

	if allCleaned {
		group.Status = storage.DupGroupStatusAutoResolved
		group.ResolvedAt = cleanupAt
		group.RetainedDID = retainedDID
		_ = r.dupGroupRepo.Upsert(group)
		logger.Info("[重复设备] 重试完成后更新分组状态", zap.String("group_id", groupID), zap.String("status", group.Status))
	}
}

// ==================== 统计信息 ====================

// GetDupDeviceSummary 获取重复设备统计概览
func (r *Runner) GetDupDeviceSummary() (map[string]interface{}, error) {
	// 设备总数
	totalDevices, err := r.dupDeviceRepo.Count()
	if err != nil {
		return nil, err
	}

	// 待处理分组数
	pendingGroups, err := r.dupGroupRepo.CountByStatus(storage.DupGroupStatusPending)
	if err != nil {
		return nil, err
	}

	// 待人工确认数（仅序列号匹配的待处理分组）
	serialOnlyGroups, err := r.dupGroupRepo.CountByMatchLevel(storage.DupMatchLevelSerialOnly)
	if err != nil {
		return nil, err
	}

	// 已自动清理数
	autoResolvedGroups, err := r.dupGroupRepo.CountByStatus(storage.DupGroupStatusAutoResolved)
	if err != nil {
		return nil, err
	}

	// 已手动清理数
	manualResolvedGroups, err := r.dupGroupRepo.CountByStatus(storage.DupGroupStatusManualResolved)
	if err != nil {
		return nil, err
	}

	// 已清理设备数
	cleanedDevices, err := r.dupCleanupLogRepo.CountByStatus(storage.DupCleanupStatusSuccess)
	if err != nil {
		return nil, err
	}

	// 高匹配分组数（待处理）
	highMatchGroups, err := r.dupGroupRepo.CountByMatchLevel(storage.DupMatchLevelHighMatch)
	if err != nil {
		return nil, err
	}

	// 任务状态
	taskState, _ := r.dupTaskStateRepo.Get()

	return map[string]interface{}{
		"total_devices":         totalDevices,
		"pending_groups":        pendingGroups,
		"manual_confirm_groups": serialOnlyGroups,
		"auto_cleaned_groups":   autoResolvedGroups,
		"manual_cleaned_groups": manualResolvedGroups,
		"cleaned_devices":       cleanedDevices,
		"high_match_groups":     highMatchGroups,
		"serial_only_groups":    serialOnlyGroups,
		"task_state":            taskState,
	}, nil
}

// ==================== 工具函数 ====================

// ListDupDeviceGroups 分页查询重复分组
func (r *Runner) ListDupDeviceGroups(status, matchLevel string, limit, offset int) ([]storage.DupDeviceGroup, int, error) {
	return r.ListDupDeviceGroupsWithDistinct(status, matchLevel, limit, offset, false)
}

// ListDupDeviceGroupsWithDistinct 分页查询重复分组（支持按序列号去重）
func (r *Runner) ListDupDeviceGroupsWithDistinct(status, matchLevel string, limit, offset int, distinct bool) ([]storage.DupDeviceGroup, int, error) {
	r.EnsureDupRepos()
	return r.dupGroupRepo.ListWithDistinct(status, matchLevel, limit, offset, distinct)
}

// GetDupDeviceGroupDetail 获取分组详情（含成员列表）
func (r *Runner) GetDupDeviceGroupDetail(groupID string) (storage.DupDeviceGroup, []storage.DupDeviceGroupMember, error) {
	r.EnsureDupRepos()
	group, exists, err := r.dupGroupRepo.Get(groupID)
	if err != nil {
		return group, nil, err
	}
	if !exists {
		return group, nil, fmt.Errorf("group not found: %s", groupID)
	}
	members, err := r.dupGroupMemberRepo.ListByGroup(groupID)
	return group, members, err
}

// ListDupDeviceGroupMembers 获取分组成员列表
func (r *Runner) ListDupDeviceGroupMembers(groupID string) ([]storage.DupDeviceGroupMember, error) {
	r.EnsureDupRepos()
	return r.dupGroupMemberRepo.ListByGroup(groupID)
}

// ListDupDeviceCleanupLogs 分页查询清理日志
func (r *Runner) ListDupDeviceCleanupLogs(status string, limit, offset int) ([]storage.DupDeviceCleanupLog, int, error) {
	r.EnsureDupRepos()
	return r.dupCleanupLogRepo.ListByStatus(status, limit, offset)
}

// GetDupDeviceTaskState 获取任务状态
func (r *Runner) GetDupDeviceTaskState() (storage.DupDeviceTaskState, error) {
	r.EnsureDupRepos()
	return r.dupTaskStateRepo.Get()
}

// UpdateDupDeviceSyncScheduleEnabled 更新同步调度开关
func (r *Runner) UpdateDupDeviceSyncScheduleEnabled(enabled bool) error {
	r.EnsureDupRepos()
	return r.dupTaskStateRepo.UpdateSyncEnabled(enabled)
}

// UpdateDupDeviceDetectScheduleEnabled 更新检测调度开关
func (r *Runner) UpdateDupDeviceDetectScheduleEnabled(enabled bool) error {
	r.EnsureDupRepos()
	return r.dupTaskStateRepo.UpdateDetectEnabled(enabled)
}

// EnsureDupRepos 确保重复设备相关的 repo 已初始化
func (r *Runner) EnsureDupRepos() {
	if r.dupDeviceRepo == nil {
		r.dupDeviceRepo = sqliteRepo.NewDupDeviceRepository(r.appDB)
	}
	if r.dupGroupRepo == nil {
		r.dupGroupRepo = sqliteRepo.NewDupDeviceGroupRepository(r.appDB)
	}
	if r.dupGroupMemberRepo == nil {
		r.dupGroupMemberRepo = sqliteRepo.NewDupDeviceGroupMemberRepository(r.appDB)
	}
	if r.dupCleanupLogRepo == nil {
		r.dupCleanupLogRepo = sqliteRepo.NewDupDeviceCleanupLogRepository(r.appDB)
	}
	if r.dupTaskStateRepo == nil {
		r.dupTaskStateRepo = sqliteRepo.NewDupDeviceTaskStateRepository(r.appDB)
	}
}
