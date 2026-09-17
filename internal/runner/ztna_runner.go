package runner

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"sealsuite-operation/internal/llm"
	"sealsuite-operation/internal/logger"
	"sealsuite-operation/internal/sealsuite"
	"sealsuite-operation/internal/storage"

	"go.uber.org/zap"
)

const ztnaSystemPrompt = `你是一名资深的零信任安全架构师，负责基于 VPN 访问日志分析内网访问权限的合理性。

## 任务
分析每个用户对每个内网资源（IP:端口）的访问是否合理，给出「继续授权」或「回收权限」的建议。

## 分析维度
1. **角色/部门匹配度**：用户的角色和部门是否与该资源的典型访问者匹配
2. **访问频率**：近 30 天和 90 天的访问天数和次数
3. **访问时段**：工作时间（9:00-18:00）vs 非工作时间 vs 凌晨（0:00-6:00）
4. **最近访问时间**：最近一次访问距今多久
5. **数据传输量**：总流量大小是否合理

## 输出要求
严格输出 JSON 格式，不要输出任何其他文字。格式如下：
{
  "results": [
    {
      "user_id": "用户ID",
      "dest_ip": "目标IP",
      "dest_port": 目标端口,
      "category": "access_should_have | access_should_remove | access_needs_review | unknown",
      "should_revoke": false,
      "confidence": 0.0-1.0,
      "reasons": ["原因1", "原因2"],
      "suggested_policy": "建议的策略描述"
    }
  ]
}

## 分类规则
- **access_should_have**：访问合理，应继续授权。置信度 >= 0.8
- **access_should_remove**：过度授权，建议回收。置信度 >= 0.9（高风险操作，必须高度确认）
- **access_needs_review**：无法自动判断，需要人工审核。置信度 0.5-0.8
- **unknown**：信息不足，无法判断。置信度 < 0.5，should_revoke = false

## 保守原则
- 宁可不回收，也不要误回收
- 信息不足或不确定时，返回 unknown，should_revoke = false
- 只有当有充分证据表明权限确实过度时，才建议回收`

func (r *Runner) SyncZTNALogs(maxItems int) (*storage.ZTNASyncResult, error) {
	if r.ztnaLogRepo == nil {
		return nil, fmt.Errorf("ztna access log repository is nil")
	}
	if r.client == nil {
		return nil, fmt.Errorf("sealsuite client is nil")
	}

	now := time.Now()

	lastTime, _ := r.ztnaLogRepo.GetLatestEventTime()
	var startTime time.Time
	if lastTime != "" {
		if t, err := time.Parse(time.RFC3339, lastTime); err == nil {
			startTime = t
		} else {
			startTime = now.Add(-24 * time.Hour)
		}
	} else {
		startTime = now.Add(-24 * time.Hour)
	}

	if maxItems > 0 {
		sevenDaysAgo := now.Add(-7 * 24 * time.Hour)
		if startTime.Before(sevenDaysAgo) {
			startTime = sevenDaysAgo
		}
	}

	startTimeStr := startTime.Format(time.RFC3339)
	endTimeStr := now.Format(time.RFC3339)

	logger.Info("[ZTNA] 开始同步VPN访问日志",
		zap.String("start_time", startTimeStr),
		zap.String("end_time", endTimeStr),
		zap.Int("max_items", maxItems),
	)

	logs, err := r.client.FetchAllVPNConntrackLogs(startTimeStr, endTimeStr, maxItems)
	if err != nil {
		logger.Error("[ZTNA] 获取VPN日志失败", zap.Error(err))
		_ = r.ztnaTaskStateRepo.UpdateLastSync(now.Format(time.RFC3339), 0, err.Error())
		return nil, err
	}

	logger.Info("[ZTNA] 获取VPN日志完成",
		zap.Int("raw_count", len(logs)),
	)

	storageLogs := sealsuite.VPNLogToStorageLog(logs)
	synced, err := r.ztnaLogRepo.BatchUpsert(storageLogs)
	if err != nil {
		logger.Error("[ZTNA] 保存VPN日志失败", zap.Error(err))
		_ = r.ztnaTaskStateRepo.UpdateLastSync(now.Format(time.RFC3339), 0, err.Error())
		return nil, err
	}

	syncAt := now.Format(time.RFC3339)
	if upErr := r.ztnaTaskStateRepo.UpdateLastSync(syncAt, synced, ""); upErr != nil {
		logger.Warn("[ZTNA] 更新同步状态失败", zap.Error(upErr))
	}

	logger.Info("[ZTNA] VPN日志同步完成",
		zap.Int("synced_count", synced),
	)

	return &storage.ZTNASyncResult{
		SyncedCount: synced,
		SyncAt:      syncAt,
	}, nil
}

func (r *Runner) BuildZTNAAccessStats() (*storage.ZTNAStatsResult, error) {
	if r.ztnaLogRepo == nil || r.ztnaStatsRepo == nil {
		return nil, fmt.Errorf("ztna repository is nil")
	}

	now := time.Now()
	logger.Info("[ZTNA] 开始构建访问统计")

	allLogs, _, err := r.ztnaLogRepo.List(0, 0, storage.ZTNAAccessLogFilter{})
	if err != nil {
		logger.Error("[ZTNA] 获取访问日志失败", zap.Error(err))
		_ = r.ztnaTaskStateRepo.UpdateLastStats(now.Format(time.RFC3339), 0, 0, err.Error())
		return nil, err
	}

	logger.Info("[ZTNA] 获取到日志条数", zap.Int("count", len(allLogs)))

	type statsKey struct {
		UserID   string
		DestIP   string
		DestPort int
		Protocol string
	}

	type statsData struct {
		userName       string
		userRole       string
		userDept       string
		departmentID   string
		totalCount     int
		successCount   int
		failedCount    int
		totalBytes     int64
		totalDuration  int
		firstTime      time.Time
		lastTime       time.Time
		daysSet30      map[string]bool
		daysSet90      map[string]bool
		workHourCount  int
		offHourCount   int
		nightCount     int
	}

	statsMap := make(map[statsKey]*statsData)
	thirtyDaysAgo := now.Add(-30 * 24 * time.Hour)
	ninetyDaysAgo := now.Add(-90 * 24 * time.Hour)

	for _, logItem := range allLogs {
		if logItem.DestIP == "" {
			continue
		}

		var eventTime time.Time
		if logItem.EventTime != "" {
			if t, err := time.Parse(time.RFC3339, logItem.EventTime); err == nil {
				eventTime = t
			}
		}
		if eventTime.IsZero() {
			eventTime = now
		}

		key := statsKey{
			UserID:   logItem.UserID,
			DestIP:   logItem.DestIP,
			DestPort: logItem.DestPort,
			Protocol: logItem.Protocol,
		}

		data, exists := statsMap[key]
		if !exists {
			data = &statsData{
				daysSet30: make(map[string]bool),
				daysSet90: make(map[string]bool),
				firstTime: eventTime,
			}
			statsMap[key] = data
		}

		if logItem.UserFullName != "" && data.userName == "" {
			data.userName = logItem.UserFullName
		}
		if logItem.DepartmentID != "" && data.departmentID == "" {
			data.departmentID = logItem.DepartmentID
		}
		if logItem.DepartmentPath != "" && data.userDept == "" {
			data.userDept = logItem.DepartmentPath
		}
		if logItem.RolesName != "" && data.userRole == "" {
			data.userRole = logItem.RolesName
		}

		data.totalCount++
		if strings.ToLower(logItem.Action) == "accept" || strings.ToLower(logItem.Action) == "allow" {
			data.successCount++
		} else {
			data.failedCount++
		}

		if eventTime.Before(data.firstTime) {
			data.firstTime = eventTime
		}
		if eventTime.After(data.lastTime) {
			data.lastTime = eventTime
		}

		dayKey := eventTime.Format("2006-01-02")
		if !eventTime.Before(thirtyDaysAgo) {
			data.daysSet30[dayKey] = true
		}
		if !eventTime.Before(ninetyDaysAgo) {
			data.daysSet90[dayKey] = true
		}

		hour := eventTime.Hour()
		if hour >= 9 && hour < 18 {
			data.workHourCount++
		} else if hour >= 0 && hour < 6 {
			data.nightCount++
		} else {
			data.offHourCount++
		}
	}

	userCount := 0
	resourceSet := make(map[string]bool)
	updated := 0

	for key, data := range statsMap {
		statsID := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s_%s_%d_%s", key.UserID, key.DestIP, key.DestPort, key.Protocol))))

		var avgDuration int
		if data.totalCount > 0 {
			avgDuration = data.totalDuration / data.totalCount
		}

		statsItem := storage.ZTNAAccessStats{
			ID:               statsID,
			UserID:           key.UserID,
			UserName:         data.userName,
			UserRole:         data.userRole,
			UserDepartment:   data.userDept,
			DepartmentID:     data.departmentID,
			DestIP:           key.DestIP,
			DestPort:         key.DestPort,
			Protocol:         key.Protocol,
			FirstAccessTime:  data.firstTime.Format(time.RFC3339),
			LastAccessTime:   data.lastTime.Format(time.RFC3339),
			AccessDays30:     len(data.daysSet30),
			AccessDays90:     len(data.daysSet90),
			TotalAccessCount: data.totalCount,
			SuccessCount:     data.successCount,
			FailedCount:      data.failedCount,
			AvgDurationSec:   avgDuration,
			TotalBytes:       data.totalBytes,
			WorkHourCount:    data.workHourCount,
			OffHourCount:     data.offHourCount,
			NightCount:       data.nightCount,
			UpdatedAt:        now.Format(time.RFC3339),
		}

		if err := r.ztnaStatsRepo.Upsert(statsItem); err != nil {
			logger.Warn("[ZTNA] 保存统计失败",
				zap.String("user_id", key.UserID),
				zap.String("dest_ip", key.DestIP),
				zap.Error(err),
			)
			continue
		}
		updated++
		resourceSet[fmt.Sprintf("%s:%d", key.DestIP, key.DestPort)] = true

		if key.UserID != "" {
			userCount++
		}
	}

	userCount = 0
	seenUsers := make(map[string]bool)
	for key := range statsMap {
		if !seenUsers[key.UserID] {
			seenUsers[key.UserID] = true
			userCount++
		}
	}

	statsAt := now.Format(time.RFC3339)
	if upErr := r.ztnaTaskStateRepo.UpdateLastStats(statsAt, userCount, len(resourceSet), ""); upErr != nil {
		logger.Warn("[ZTNA] 更新统计状态失败", zap.Error(upErr))
	}

	logger.Info("[ZTNA] 访问统计构建完成",
		zap.Int("user_count", userCount),
		zap.Int("resource_count", len(resourceSet)),
		zap.Int("stats_count", updated),
	)

	return &storage.ZTNAStatsResult{
		UserCount:     userCount,
		ResourceCount: len(resourceSet),
		StatsAt:       statsAt,
	}, nil
}

func (r *Runner) AnalyzeZTNAPolicies(maxItems int, reanalyzeAll bool) (*storage.ZTNAAnalyzeResult, error) {
	if r.ztnaStatsRepo == nil || r.ztnaAnalysisRepo == nil {
		return nil, fmt.Errorf("ztna repository is nil")
	}

	now := time.Now()
	logger.Info("[ZTNA] 开始策略分析",
		zap.Int("max_items", maxItems),
		zap.Bool("reanalyze_all", reanalyzeAll),
	)

	var allStats []storage.ZTNAAccessStats
	allStats, _, err := r.ztnaStatsRepo.List(0, 0, storage.ZTNAStatsFilter{})
	if err != nil {
		logger.Error("[ZTNA] 获取统计数据失败", zap.Error(err))
		return nil, err
	}

	var toAnalyze []storage.ZTNAAccessStats
	for _, s := range allStats {
		if !reanalyzeAll {
			_, exists, _ := r.ztnaAnalysisRepo.GetByStatsID(s.ID)
			if exists {
				continue
			}
		}
		toAnalyze = append(toAnalyze, s)
	}

	if maxItems > 0 && len(toAnalyze) > maxItems {
		toAnalyze = toAnalyze[:maxItems]
	}

	logger.Info("[ZTNA] 待分析条目数", zap.Int("count", len(toAnalyze)))

	if len(toAnalyze) == 0 {
		summary, _ := r.ztnaAnalysisRepo.Summary()
		return &storage.ZTNAAnalyzeResult{
			AnalyzedCount:     0,
			ShouldHaveCount:   summary.ShouldHaveCount,
			ShouldRemoveCount: summary.ShouldRemoveCount,
			NeedsReviewCount:  summary.NeedsReviewCount,
			UnknownCount:      summary.UnknownCount,
			AnalyzeAt:         now.Format(time.RFC3339),
		}, nil
	}

	results := make([]storage.ZTNAPolicyAnalysis, 0, len(toAnalyze))
	batchSize := 20
	llmClient := r.getLLMClientForZTNA()

	for i := 0; i < len(toAnalyze); i += batchSize {
		end := i + batchSize
		if end > len(toAnalyze) {
			end = len(toAnalyze)
		}
		batch := toAnalyze[i:end]

		logger.Info("[ZTNA] 分析批次",
			zap.Int("batch_start", i),
			zap.Int("batch_end", end),
			zap.Int("batch_size", len(batch)),
		)

		var batchResults []storage.ZTNAPolicyAnalysis
		var useLLM bool

		if llmClient != nil {
			llmResults, llmErr := r.analyzeZTNBatchWithLLM(llmClient, batch)
			if llmErr != nil {
				logger.Warn("[ZTNA] LLM分析失败，回退到规则模式", zap.Error(llmErr))
				batchResults = r.analyzeZTNBatchWithHeuristic(batch)
				useLLM = false
			} else {
				batchResults = llmResults
				useLLM = true
			}
		} else {
			batchResults = r.analyzeZTNBatchWithHeuristic(batch)
			useLLM = false
		}

		for _, res := range batchResults {
			if !useLLM {
				res.Method = "heuristic"
			}
			results = append(results, res)
		}
	}

	shouldHaveCount := 0
	shouldRemoveCount := 0
	needsReviewCount := 0
	unknownCount := 0

	for _, res := range results {
		if err := r.ztnaAnalysisRepo.Upsert(res); err != nil {
			logger.Warn("[ZTNA] 保存分析结果失败",
				zap.String("stats_id", res.StatsID),
				zap.Error(err),
			)
			continue
		}
		switch res.Category {
		case storage.ZTNAnalysisCategoryShouldHave:
			shouldHaveCount++
		case storage.ZTNAnalysisCategoryShouldRemove:
			shouldRemoveCount++
		case storage.ZTNAnalysisCategoryNeedsReview:
			needsReviewCount++
		default:
			unknownCount++
		}
	}

	analyzeAt := now.Format(time.RFC3339)
	if upErr := r.ztnaTaskStateRepo.UpdateLastAnalysis(analyzeAt, len(results), ""); upErr != nil {
		logger.Warn("[ZTNA] 更新分析状态失败", zap.Error(upErr))
	}

	logger.Info("[ZTNA] 策略分析完成",
		zap.Int("analyzed_count", len(results)),
		zap.Int("should_have", shouldHaveCount),
		zap.Int("should_remove", shouldRemoveCount),
		zap.Int("needs_review", needsReviewCount),
		zap.Int("unknown", unknownCount),
	)

	return &storage.ZTNAAnalyzeResult{
		AnalyzedCount:     len(results),
		ShouldHaveCount:   shouldHaveCount,
		ShouldRemoveCount: shouldRemoveCount,
		NeedsReviewCount:  needsReviewCount,
		UnknownCount:      unknownCount,
		AnalyzeAt:         analyzeAt,
	}, nil
}

func (r *Runner) analyzeZTNBatchWithLLM(client *llm.Client, batch []storage.ZTNAAccessStats) ([]storage.ZTNAPolicyAnalysis, error) {
	logger.Info("[ZTNA-LLM] 开始LLM批量分析",
		zap.Int("batch_size", len(batch)),
	)

	type inputItem struct {
		UserID         string `json:"user_id"`
		UserName       string `json:"user_name"`
		UserRole       string `json:"user_role"`
		UserDepartment string `json:"user_department"`
		DestIP         string `json:"dest_ip"`
		DestPort       int    `json:"dest_port"`
		Protocol       string `json:"protocol"`
		FirstAccess    string `json:"first_access_time"`
		LastAccess     string `json:"last_access_time"`
		AccessDays30   int    `json:"access_days_30"`
		AccessDays90   int    `json:"access_days_90"`
		TotalCount     int    `json:"total_access_count"`
		SuccessCount   int    `json:"success_count"`
		FailedCount    int    `json:"failed_count"`
		TotalBytes     int64  `json:"total_bytes"`
		WorkHourCount  int    `json:"work_hour_count"`
		OffHourCount   int    `json:"off_hour_count"`
		NightCount     int    `json:"night_count"`
	}

	inputItems := make([]inputItem, 0, len(batch))
	for _, s := range batch {
		inputItems = append(inputItems, inputItem{
			UserID:         s.UserID,
			UserName:       s.UserName,
			UserRole:       s.UserRole,
			UserDepartment: s.UserDepartment,
			DestIP:         s.DestIP,
			DestPort:       s.DestPort,
			Protocol:       s.Protocol,
			FirstAccess:    s.FirstAccessTime,
			LastAccess:     s.LastAccessTime,
			AccessDays30:   s.AccessDays30,
			AccessDays90:   s.AccessDays90,
			TotalCount:     s.TotalAccessCount,
			SuccessCount:   s.SuccessCount,
			FailedCount:    s.FailedCount,
			TotalBytes:     s.TotalBytes,
			WorkHourCount:  s.WorkHourCount,
			OffHourCount:   s.OffHourCount,
			NightCount:     s.NightCount,
		})
	}

	resp, err := client.Chat(llm.ChatRequest{
		SystemPrompt:   ztnaSystemPrompt,
		Prompt:         "请分析以下VPN访问统计数据，给出权限建议：",
		Input:          inputItems,
		ResponseFormat: map[string]interface{}{"type": "json_object"},
	})
	if err != nil {
		return nil, fmt.Errorf("llm chat: %w", err)
	}

	logger.Info("[ZTNA-LLM] LLM响应获取成功",
		zap.Int("response_len", len(resp.Content)),
		zap.String("model", resp.Model),
	)

	var output struct {
		Results []struct {
			UserID         string   `json:"user_id"`
			DestIP         string   `json:"dest_ip"`
			DestPort       int      `json:"dest_port"`
			Category       string   `json:"category"`
			ShouldRevoke   bool     `json:"should_revoke"`
			Confidence     float64  `json:"confidence"`
			Reasons        []string `json:"reasons"`
			SuggestedPolicy string  `json:"suggested_policy"`
		} `json:"results"`
	}

	if err := json.Unmarshal([]byte(resp.Content), &output); err != nil {
		logger.Warn("[ZTNA-LLM] 解析LLM响应失败",
			zap.Error(err),
			zap.String("content", truncateStr(resp.Content, 500)),
		)
		return nil, fmt.Errorf("parse llm response: %w", err)
	}

	statsMap := make(map[string]storage.ZTNAAccessStats)
	for _, s := range batch {
		key := fmt.Sprintf("%s_%s_%d", s.UserID, s.DestIP, s.DestPort)
		statsMap[key] = s
	}

	results := make([]storage.ZTNAPolicyAnalysis, 0, len(output.Results))
	now := time.Now().Format(time.RFC3339)

	for _, llmRes := range output.Results {
		key := fmt.Sprintf("%s_%s_%d", llmRes.UserID, llmRes.DestIP, llmRes.DestPort)
		statsItem, ok := statsMap[key]
		if !ok {
			continue
		}

		analysisID := fmt.Sprintf("ana_%x", md5.Sum([]byte(statsItem.ID)))

		result := storage.ZTNAPolicyAnalysis{
			ID:              analysisID,
			StatsID:         statsItem.ID,
			UserID:          statsItem.UserID,
			UserName:        statsItem.UserName,
			UserRole:        statsItem.UserRole,
			UserDepartment:  statsItem.UserDepartment,
			DestIP:          statsItem.DestIP,
			DestPort:        statsItem.DestPort,
			Protocol:        statsItem.Protocol,
			ResourceTag:     statsItem.ResourceTag,
			Category:        llmRes.Category,
			ShouldRevoke:    llmRes.ShouldRevoke,
			Confidence:      llmRes.Confidence,
			Reasons:         llmRes.Reasons,
			SuggestedPolicy: llmRes.SuggestedPolicy,
			Method:          "llm",
			Status:          storage.ZTNAnalysisStatusAnalyzed,
			AnalyzedAt:      now,
		}
		results = append(results, result)
	}

	logger.Info("[ZTNA-LLM] 批次分析完成",
		zap.Int("result_count", len(results)),
	)

	return results, nil
}

func (r *Runner) analyzeZTNBatchWithHeuristic(batch []storage.ZTNAAccessStats) []storage.ZTNAPolicyAnalysis {
	results := make([]storage.ZTNAPolicyAnalysis, 0, len(batch))
	now := time.Now().Format(time.RFC3339)

	for _, s := range batch {
		result := storage.ZTNAPolicyAnalysis{
			ID:             fmt.Sprintf("ana_%x", md5.Sum([]byte(s.ID))),
			StatsID:        s.ID,
			UserID:         s.UserID,
			UserName:       s.UserName,
			UserRole:       s.UserRole,
			UserDepartment: s.UserDepartment,
			DestIP:         s.DestIP,
			DestPort:       s.DestPort,
			Protocol:       s.Protocol,
			ResourceTag:    s.ResourceTag,
			Method:         "heuristic",
			Status:         storage.ZTNAnalysisStatusAnalyzed,
			AnalyzedAt:     now,
		}

		reasons := []string{}
		category := storage.ZTNAnalysisCategoryUnknown
		shouldRevoke := false
		confidence := 0.0

		ninetyDaysAgo := time.Now().Add(-90 * 24 * time.Hour)
		var lastAccess time.Time
		if s.LastAccessTime != "" {
			if t, err := time.Parse(time.RFC3339, s.LastAccessTime); err == nil {
				lastAccess = t
			}
		}

		if !lastAccess.IsZero() && lastAccess.Before(ninetyDaysAgo) {
			category = storage.ZTNAnalysisCategoryShouldRemove
			shouldRevoke = true
			confidence = 0.95
			reasons = append(reasons, fmt.Sprintf("近90天无访问（最后访问：%s）", s.LastAccessTime))
			reasons = append(reasons, "长期未使用的权限建议回收")
			result.SuggestedPolicy = fmt.Sprintf("建议回收用户 %s 对 %s:%d 的访问权限（长期未使用）",
				s.UserName, s.DestIP, s.DestPort)
		} else if s.AccessDays30 >= 10 && s.WorkHourCount > 0 && s.TotalAccessCount > 0 &&
			float64(s.WorkHourCount)/float64(s.TotalAccessCount) > 0.7 {
			category = storage.ZTNAnalysisCategoryShouldHave
			shouldRevoke = false
			confidence = 0.85
			reasons = append(reasons, fmt.Sprintf("近30天访问%d天，频率稳定", s.AccessDays30))
			reasons = append(reasons, fmt.Sprintf("%.0f%%的访问发生在工作时段",
				float64(s.WorkHourCount)/float64(s.TotalAccessCount)*100))
			result.SuggestedPolicy = fmt.Sprintf("建议保留用户 %s 对 %s:%d 的访问权限",
				s.UserName, s.DestIP, s.DestPort)
		} else if s.NightCount > 0 && s.TotalAccessCount > 0 &&
			float64(s.NightCount)/float64(s.TotalAccessCount) > 0.3 {
			category = storage.ZTNAnalysisCategoryNeedsReview
			shouldRevoke = false
			confidence = 0.6
			reasons = append(reasons, fmt.Sprintf("%.0f%%的访问发生在凌晨时段（0:00-6:00）",
				float64(s.NightCount)/float64(s.TotalAccessCount)*100))
			reasons = append(reasons, "非工作时间高频访问需人工确认")
			result.SuggestedPolicy = fmt.Sprintf("需人工审核用户 %s 对 %s:%d 的访问（凌晨访问占比高）",
				s.UserName, s.DestIP, s.DestPort)
		} else {
			category = storage.ZTNAnalysisCategoryUnknown
			shouldRevoke = false
			confidence = 0.4
			reasons = append(reasons, "规则引擎无法明确判断")
			reasons = append(reasons, "建议结合更多上下文信息由人工判断")
			result.SuggestedPolicy = fmt.Sprintf("无法自动判断用户 %s 对 %s:%d 的权限合理性，需人工确认",
				s.UserName, s.DestIP, s.DestPort)
		}

		result.Category = category
		result.ShouldRevoke = shouldRevoke
		result.Confidence = confidence
		result.Reasons = reasons
		results = append(results, result)
	}

	return results
}

func (r *Runner) RunZTNASyncAndAnalyze(maxItems int) (*storage.ZTNAFullResult, error) {
	now := time.Now()
	result := &storage.ZTNAFullResult{}

	logger.Info("[ZTNA] 开始立即同步分析流程",
		zap.Int("max_items", maxItems),
	)

	syncRes, err := r.SyncZTNALogs(maxItems)
	if err != nil {
		logger.Error("[ZTNA] 同步失败", zap.Error(err))
		result.Error = err.Error()
		return result, err
	}
	result.SyncedCount = syncRes.SyncedCount

	statsRes, err := r.BuildZTNAAccessStats()
	if err != nil {
		logger.Error("[ZTNA] 统计失败", zap.Error(err))
		result.Error = err.Error()
		return result, err
	}
	result.UserCount = statsRes.UserCount
	result.ResourceCount = statsRes.ResourceCount

	analyzeRes, err := r.AnalyzeZTNAPolicies(0, false)
	if err != nil {
		logger.Error("[ZTNA] 分析失败", zap.Error(err))
		result.Error = err.Error()
		return result, err
	}
	result.AnalyzedCount = analyzeRes.AnalyzedCount
	result.ShouldHaveCount = analyzeRes.ShouldHaveCount
	result.ShouldRemoveCount = analyzeRes.ShouldRemoveCount

	logger.Info("[ZTNA] 立即同步分析完成",
		zap.Int("synced", syncRes.SyncedCount),
		zap.Int("analyzed", analyzeRes.AnalyzedCount),
		zap.Duration("total_duration", time.Since(now)),
	)

	return result, nil
}

func (r *Runner) setupZTNACronJobs() {
	if r.sched == nil {
		return
	}

	state, _ := r.ztnaTaskStateRepo.Get()

	if state.SyncScheduleEnabled {
		if err := r.sched.AddJob("ztna_sync", "0 0 1 * * *", func() error {
			logger.Info("[ZTNA] 定时同步开始")
			_, err := r.SyncZTNALogs(0)
			if err != nil {
				logger.Error("[ZTNA] 定时同步失败", zap.Error(err))
			} else {
				logger.Info("[ZTNA] 定时同步完成")
			}
			return err
		}); err != nil {
			logger.Error("[ZTNA] 添加定时同步任务失败", zap.Error(err))
		}
	}

	if state.StatsScheduleEnabled {
		if err := r.sched.AddJob("ztna_stats", "0 30 1 * * *", func() error {
			logger.Info("[ZTNA] 定时统计开始")
			_, err := r.BuildZTNAAccessStats()
			if err != nil {
				logger.Error("[ZTNA] 定时统计失败", zap.Error(err))
			} else {
				logger.Info("[ZTNA] 定时统计完成")
			}
			return err
		}); err != nil {
			logger.Error("[ZTNA] 添加定时统计任务失败", zap.Error(err))
		}
	}

	if state.AnalysisScheduleEnabled {
		if err := r.sched.AddJob("ztna_analysis", "0 0 2 * * *", func() error {
			logger.Info("[ZTNA] 定时分析开始")
			_, err := r.AnalyzeZTNAPolicies(0, false)
			if err != nil {
				logger.Error("[ZTNA] 定时分析失败", zap.Error(err))
			} else {
				logger.Info("[ZTNA] 定时分析完成")
			}
			return err
		}); err != nil {
			logger.Error("[ZTNA] 添加定时分析任务失败", zap.Error(err))
		}
	}

	logger.Info("[ZTNA] 定时任务设置完成",
		zap.Bool("sync_enabled", state.SyncScheduleEnabled),
		zap.Bool("stats_enabled", state.StatsScheduleEnabled),
		zap.Bool("analysis_enabled", state.AnalysisScheduleEnabled),
	)
}

func (r *Runner) getLLMClientForZTNA() *llm.Client {
	if r.cfg == nil {
		return nil
	}
	if !r.cfg.LLM.Planner.Enabled && !r.cfg.LLM.Formatter.Enabled {
		return nil
	}
	cfg := r.cfg.LLM.Planner
	if !cfg.Enabled {
		cfg = r.cfg.LLM.Formatter
	}
	client := llm.NewClient(cfg)
	if client == nil {
		return nil
	}
	return client
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}

func sortByAccessCount(stats []storage.ZTNAAccessStats) {
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].TotalAccessCount > stats[j].TotalAccessCount
	})
}
