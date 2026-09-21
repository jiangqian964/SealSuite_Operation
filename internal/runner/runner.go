package runner

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"sealsuite-operation/internal/api"
	"sealsuite-operation/internal/config"
	appdb "sealsuite-operation/internal/db"
	"sealsuite-operation/internal/llm"
	"sealsuite-operation/internal/logger"
	"sealsuite-operation/internal/repository"
	sqliteRepo "sealsuite-operation/internal/repository/sqlite"
	"sealsuite-operation/internal/scheduler"
	"sealsuite-operation/internal/sealsuite"
	"sealsuite-operation/internal/service"
	"sealsuite-operation/internal/storage"
	"sealsuite-operation/internal/webhook"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

type JobStatus struct {
	LastRun    time.Time  `json:"last_run"`
	LastOK     bool       `json:"last_ok"`
	LastError  string     `json:"last_error,omitempty"`
	DurationMs int64      `json:"duration_ms"`
	NextRun    *time.Time `json:"next_run,omitempty"`
}

type ComplexTaskStepResult struct {
	StepID string      `json:"step_id"`
	Type   string      `json:"type"`
	Name   string      `json:"name"`
	OK     bool        `json:"ok"`
	Output interface{} `json:"output,omitempty"`
	Error  string      `json:"error,omitempty"`
}

type ComplexTaskRunResult struct {
	TaskID      string                  `json:"task_id"`
	TaskName    string                  `json:"task_name"`
	OK          bool                    `json:"ok"`
	StartedAt   string                  `json:"started_at"`
	FinishedAt  string                  `json:"finished_at"`
	Steps       []ComplexTaskStepResult `json:"steps"`
	FinalOutput interface{}             `json:"final_output,omitempty"`
}

type googlePrefix struct {
	IPv4Prefix string `json:"ipv4Prefix"`
	IPv6Prefix string `json:"ipv6Prefix"`
}

type googleIPRanges struct {
	Prefixes []googlePrefix `json:"prefixes"`
}

type Runner struct {
	cfg *config.Config

	appDB *sql.DB

	legacyJobRepo       repository.LegacyJobRepository
	taskDraftRepo       *sqliteRepo.TaskDraftRepository
	scheduleRepo        *sqliteRepo.ScheduleRepository
	templateRepo        *sqliteRepo.TemplateRepository
	webhookRepo         *sqliteRepo.WebhookRepository
	llmAPIRepo          *sqliteRepo.LLMAPIRepository
	complexTaskRepo     *sqliteRepo.ComplexTaskRepository
	externalIPSyncRepo  repository.ExternalIPSyncTaskRepository
	feishuResourcesRepo *sqliteRepo.FeishuResourcesRepository
	feishuDevicesRepo   *sqliteRepo.FeishuDevicesRepository
	feishuAPIRepo       *sqliteRepo.FeishuAPIRepository
	deviceGroupsRepo    *sqliteRepo.DeviceGroupsRepository
	fieldMappingRepo    *sqliteRepo.FieldMappingRepository
	dlpEventRepo        *sqliteRepo.DLPEventRepository
	dlpAnalysisRepo     *sqliteRepo.DLPAnalysisRepository
	dlpWhitelistRepo    *sqliteRepo.DLPWhitelistRepository
	dupDeviceRepo       *sqliteRepo.DupDeviceRepository
	dupGroupRepo        *sqliteRepo.DupDeviceGroupRepository
	dupGroupMemberRepo  *sqliteRepo.DupDeviceGroupMemberRepository
	dupCleanupLogRepo   *sqliteRepo.DupDeviceCleanupLogRepository
	dupTaskStateRepo    *sqliteRepo.DupDeviceTaskStateRepository
	ztnaLogRepo         *sqliteRepo.ZTNAAccessLogRepository
	ztnaStatsRepo       *sqliteRepo.ZTNAAccessStatsRepository
	ztnaAnalysisRepo    *sqliteRepo.ZTNAAnalysisRepository
	ztnaTaskStateRepo   *sqliteRepo.ZTNATaskStateRepository
	runLogsRepo         *sqliteRepo.JobRunsRepository
	approvalConfigRepo  *sqliteRepo.ApprovalConfigRepository
	approvalTaskRepo    *sqliteRepo.ApprovalTaskRepository
	executionSvc        *service.ExecutionService

	client   *sealsuite.Client
	executor *api.Executor

	// feishuToken 飞书自建应用 tenant_access_token 缓存
	// feishuTokenAppID 记录当前缓存 token 所属的 App ID，切换激活配置后需失效
	feishuTokenMu        sync.Mutex
	feishuToken          string
	feishuTokenExpiresAt time.Time
	feishuTokenAppID     string

	executeExternalIPSync    func(id string) (*service.ExternalIPSyncExecutionSummary, error)
	fetchGoogleCIDRs         func(task storage.ExternalIPSyncTask) ([]string, error)
	loadFeilianResourceCIDRs func(resourceID string) ([]string, error)
	writeFeilianCIDRs        func(task storage.ExternalIPSyncTask, cidrs []string) error

	sched         *scheduler.Scheduler
	jobsFile      *storage.JobsFile
	schedulesFile *storage.JobSchedulesFile
	templates     map[string]storage.Template
	taskDrafts    map[string]storage.TaskDraft

	status         map[string]*JobStatus
	scheduleStatus map[string]*JobStatus
	statusMu       sync.RWMutex
}

func New(cfg *config.Config, client *sealsuite.Client) *Runner {
	r := &Runner{
		cfg:            cfg,
		client:         client,
		executor:       api.NewExecutor(client, nil),
		status:         map[string]*JobStatus{},
		scheduleStatus: map[string]*JobStatus{},
		taskDrafts:     map[string]storage.TaskDraft{},
		executionSvc:   service.NewExecutionService(nil),
	}
	r.fetchGoogleCIDRs = r.defaultFetchGoogleCIDRs
	r.loadFeilianResourceCIDRs = r.defaultLoadFeilianResourceCIDRs
	r.writeFeilianCIDRs = r.defaultWriteFeilianCIDRsV2
	r.executeExternalIPSync = r.runExternalIPSyncTask
	r.initSQLiteRuntime()
	return r
}

func (r *Runner) InitDB(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if r.appDB != nil {
		_ = r.appDB.Close()
	}
	r.appDB = db
	r.setupRepositories(db)
	return nil
}

func (r *Runner) setupRepositories(appDB *sql.DB) {
	r.legacyJobRepo = sqliteRepo.NewLegacyJobRepository(appDB)
	r.taskDraftRepo = sqliteRepo.NewTaskDraftRepository(appDB)
	r.scheduleRepo = sqliteRepo.NewScheduleRepository(appDB)
	r.templateRepo = sqliteRepo.NewTemplateRepository(appDB)
	r.webhookRepo = sqliteRepo.NewWebhookRepository(appDB)
	r.llmAPIRepo = sqliteRepo.NewLLMAPIRepository(appDB)
	r.complexTaskRepo = sqliteRepo.NewComplexTaskRepository(appDB)
	r.externalIPSyncRepo = sqliteRepo.NewExternalIPSyncTaskRepository(appDB)
	r.feishuResourcesRepo = sqliteRepo.NewFeishuResourcesRepository(appDB)
	r.feishuDevicesRepo = sqliteRepo.NewFeishuDevicesRepository(appDB)
	r.feishuAPIRepo = sqliteRepo.NewFeishuAPIRepository(appDB)
	r.deviceGroupsRepo = sqliteRepo.NewDeviceGroupsRepository(appDB)
	r.fieldMappingRepo = sqliteRepo.NewFieldMappingRepository(appDB)
	r.dlpEventRepo = sqliteRepo.NewDLPEventRepository(appDB)
	r.dlpAnalysisRepo = sqliteRepo.NewDLPAnalysisRepository(appDB)
	r.dlpWhitelistRepo = sqliteRepo.NewDLPWhitelistRepository(appDB)
	r.dupDeviceRepo = sqliteRepo.NewDupDeviceRepository(appDB)
	r.dupGroupRepo = sqliteRepo.NewDupDeviceGroupRepository(appDB)
	r.dupGroupMemberRepo = sqliteRepo.NewDupDeviceGroupMemberRepository(appDB)
	r.dupCleanupLogRepo = sqliteRepo.NewDupDeviceCleanupLogRepository(appDB)
	r.dupTaskStateRepo = sqliteRepo.NewDupDeviceTaskStateRepository(appDB)
	r.ztnaLogRepo = sqliteRepo.NewZTNAAccessLogRepository(appDB)
	r.ztnaStatsRepo = sqliteRepo.NewZTNAAccessStatsRepository(appDB)
	r.ztnaAnalysisRepo = sqliteRepo.NewZTNAAnalysisRepository(appDB)
	r.ztnaTaskStateRepo = sqliteRepo.NewZTNATaskStateRepository(appDB)
	r.runLogsRepo = sqliteRepo.NewJobRunsRepository(appDB)
	r.approvalConfigRepo = sqliteRepo.NewApprovalConfigRepository(appDB)
	r.approvalTaskRepo = sqliteRepo.NewApprovalTaskRepository(appDB)
	r.executionSvc = service.NewExecutionService(r.runLogsRepo)
	r.executeExternalIPSync = r.runExternalIPSyncTask
}

func (r *Runner) Executor() *api.Executor { return r.executor }

func (r *Runner) Config() *config.Config { return r.cfg }

const scheduleRunKeyPrefix = "schedule:"

func (r *Runner) loadTemplates() (*storage.TemplatesFile, error) {
	if r.templateRepo == nil {
		return nil, fmt.Errorf("template repository is nil")
	}
	return r.templateRepo.Load()
}

func (r *Runner) loadLegacyJobsFile() (*storage.JobsFile, error) {
	if r.legacyJobRepo == nil {
		return nil, fmt.Errorf("legacy job repository is nil")
	}
	return r.legacyJobRepo.Load()
}

func (r *Runner) initSQLiteRuntime() {
	if r == nil || r.cfg == nil {
		return
	}

	dbPath := strings.TrimSpace(r.cfg.Database.Path)
	if dbPath == "" {
		dbPath = "./data/app.db"
	}

	appDB, err := appdb.OpenSQLite(dbPath)
	if err != nil {
		logger.Warn("[Runner] 打开 SQLite 数据库失败", zap.String("path", dbPath), zap.Error(err))
		return
	}
	if err := appdb.Migrate(appDB); err != nil {
		logger.Warn("[Runner] 数据库迁移失败", zap.String("path", dbPath), zap.Error(err))
		_ = appDB.Close()
		return
	}
	if err := appdb.BootstrapLegacyJobsFromYAML(appDB, "jobs.yaml"); err != nil {
		logger.Warn("[Runner] 从 jobs.yaml 导入旧任务失败", zap.String("path", "jobs.yaml"), zap.Error(err))
	}

	r.appDB = appDB
	r.setupRepositories(appDB)
}

// HotReloadConfig 用于 Web 控制台保存连接信息后在线生效：
// - 更新 cfg（影响 scheduler 时区/启用开关等）
// - 更新 client（影响飞连 API 调用）
// - executor 同步切换 client
func (r *Runner) HotReloadConfig(cfg *config.Config, client *sealsuite.Client) {
	if cfg != nil {
		if r.appDB != nil {
			_ = r.appDB.Close()
			r.appDB = nil
		}
		r.legacyJobRepo = nil
		r.taskDraftRepo = nil
		r.scheduleRepo = nil
		r.templateRepo = nil
		r.webhookRepo = nil
		r.llmAPIRepo = nil
		r.complexTaskRepo = nil
		r.externalIPSyncRepo = nil
		r.runLogsRepo = nil
		r.approvalConfigRepo = nil
		r.approvalTaskRepo = nil
		r.executionSvc = service.NewExecutionService(nil)
		r.executeExternalIPSync = r.runExternalIPSyncTask
		r.cfg = cfg
		r.initSQLiteRuntime()
	}
	if client != nil {
		r.client = client
		r.executor.SetClient(client)
	}
}

func (r *Runner) LoadSnapshot() (*storage.JobsFile, map[string]storage.Template, map[string]*JobStatus, error) {
	jf, err := r.loadLegacyJobsFile()
	if err != nil {
		return nil, nil, nil, err
	}
	tf, err := r.loadTemplates()
	if err != nil {
		return nil, nil, nil, err
	}
	tmap := map[string]storage.Template{}
	for _, t := range tf.Templates {
		tmap[t.ID] = t
	}
	// status copy
	smap := map[string]*JobStatus{}
	r.statusMu.RLock()
	for k, v := range r.status {
		cp := *v
		smap[k] = &cp
	}
	r.statusMu.RUnlock()

	latestRuns, err := r.latestRunLogStatus()
	if err == nil {
		for name, rec := range latestRuns {
			if strings.HasPrefix(name, scheduleRunKeyPrefix) {
				continue
			}
			st := smap[name]
			if st == nil {
				st = &JobStatus{}
				smap[name] = st
			}
			applyJobStatusRecord(st, rec)
		}
	}
	return jf, tmap, smap, nil
}

func (r *Runner) loadTaskDraftsFile() (*storage.TaskDraftsFile, error) {
	if r.taskDraftRepo == nil {
		return nil, fmt.Errorf("task draft repository is nil")
	}
	items, err := r.taskDraftRepo.List()
	if err != nil {
		return nil, err
	}
	tf := &storage.TaskDraftsFile{Version: 1, Items: items}
	storage.EnsureTaskDraftsFileDefaults(tf)
	return tf, nil
}

func (r *Runner) loadSchedulesFile() (*storage.JobSchedulesFile, error) {
	if r.scheduleRepo == nil {
		return nil, fmt.Errorf("schedule repository is nil")
	}
	items, err := r.scheduleRepo.List()
	if err != nil {
		return nil, err
	}
	sf := &storage.JobSchedulesFile{Version: 1, Items: items}
	storage.EnsureJobSchedulesFileDefaults(sf)
	return sf, nil
}

func (r *Runner) LoadScheduleSnapshot() (*storage.JobSchedulesFile, map[string]storage.TaskDraft, map[string]*JobStatus, error) {
	sf, err := r.loadSchedulesFile()
	if err != nil {
		return nil, nil, nil, err
	}
	tf, err := r.loadTaskDraftsFile()
	if err != nil {
		return nil, nil, nil, err
	}
	dmap := map[string]storage.TaskDraft{}
	for _, d := range tf.Items {
		dmap[d.ID] = storage.NormalizeTaskDraft(d)
	}
	smap := map[string]*JobStatus{}
	r.statusMu.RLock()
	for k, v := range r.scheduleStatus {
		cp := *v
		smap[k] = &cp
	}
	r.statusMu.RUnlock()

	latestRuns, err := r.latestRunLogStatus()
	if err == nil {
		for key, rec := range latestRuns {
			if !strings.HasPrefix(key, scheduleRunKeyPrefix) {
				continue
			}
			id := strings.TrimPrefix(key, scheduleRunKeyPrefix)
			st := smap[id]
			if st == nil {
				st = &JobStatus{}
				smap[id] = st
			}
			applyJobStatusRecord(st, rec)
		}
	}
	return sf, dmap, smap, nil
}

func (r *Runner) Reload() error {
	jf, err := r.loadLegacyJobsFile()
	if err != nil {
		return err
	}
	tf, err := r.loadTemplates()
	if err != nil {
		return err
	}

	tmap := map[string]storage.Template{}
	for _, t := range tf.Templates {
		tmap[t.ID] = t
	}
	df, err := r.loadTaskDraftsFile()
	if err != nil {
		return err
	}
	sf, err := r.loadSchedulesFile()
	if err != nil {
		return err
	}
	dmap := map[string]storage.TaskDraft{}
	for _, d := range df.Items {
		dmap[d.ID] = storage.NormalizeTaskDraft(d)
	}
	r.templates = tmap
	r.taskDrafts = dmap
	r.executor.SetTemplates(tmap)
	r.jobsFile = jf
	r.schedulesFile = sf

	loc, err := time.LoadLocation(r.cfg.Scheduler.Timezone)
	if err != nil {
		logger.Warn("invalid timezone, fallback to local", zap.String("timezone", r.cfg.Scheduler.Timezone), zap.Error(err))
		loc = time.Local
	}

	if r.sched != nil {
		r.sched.Stop()
	}
	r.sched = scheduler.New(loc)

	if !r.cfg.Scheduler.Enabled {
		logger.Info("scheduler disabled by config")
		return nil
	}

	for _, j := range jf.Jobs {
		if !j.Enabled {
			continue
		}
		r.ensureStatus(r.status, j.Name)
		jobCopy := j
		if err := r.sched.AddJob(jobCopy.Name, jobCopy.Cron, func() error {
			return r.runJob(jobCopy)
		}); err != nil {
			logger.Error("failed to add job", zap.String("job", jobCopy.Name), zap.Error(err))
		}
	}

	for _, s := range sf.Items {
		if !s.Enabled {
			continue
		}
		spec, err := r.scheduleSpec(s)
		if err != nil {
			logger.Error("failed to build schedule spec", zap.String("schedule", s.ID), zap.Error(err))
			continue
		}
		r.ensureStatus(r.scheduleStatus, s.ID)
		scheduleCopy := s
		if err := r.sched.AddJob(r.scheduleEntryName(scheduleCopy.ID), spec, func() error {
			if ok, err := r.scheduleWindowAllows(scheduleCopy, time.Now()); err != nil {
				return err
			} else if !ok {
				return nil
			}
			return r.runSchedule(scheduleCopy)
		}); err != nil {
			logger.Error("failed to add schedule", zap.String("schedule", scheduleCopy.ID), zap.Error(err))
		}
	}

	r.setupDupDeviceCronJobs()
	r.setupZTNACronJobs()

	r.sched.Start()
	r.refreshNextRun()
	return nil
}

func (r *Runner) setupDupDeviceCronJobs() {
	if err := r.sched.AddJob("dup_device_sync", "0 0 0 * * *", func() error {
		logger.Info("[重复设备] 定时同步开始")
		_, err := r.SyncDupDevices()
		if err != nil {
			logger.Error("[重复设备] 定时同步失败", zap.Error(err))
		} else {
			logger.Info("[重复设备] 定时同步完成")
		}
		return err
	}); err != nil {
		logger.Error("[重复设备] 添加定时同步任务失败", zap.Error(err))
	}

	if err := r.sched.AddJob("dup_device_detect", "0 0 2 * * *", func() error {
		logger.Info("[重复设备] 定时检测开始")
		_, err := r.RunDupDeviceFullProcess()
		if err != nil {
			logger.Error("[重复设备] 定时检测失败", zap.Error(err))
		} else {
			logger.Info("[重复设备] 定时检测完成")
		}
		return err
	}); err != nil {
		logger.Error("[重复设备] 添加定时检测任务失败", zap.Error(err))
	}
}

func (r *Runner) Stop() {
	if r.sched != nil {
		r.sched.Stop()
	}
	if r.appDB != nil {
		_ = r.appDB.Close()
		r.appDB = nil
	}
}

func (r *Runner) RunOnce(name string) error {
	if r.jobsFile == nil {
		if err := r.Reload(); err != nil {
			return err
		}
	}
	for _, j := range r.jobsFile.Jobs {
		if j.Name == name {
			return r.runJob(j)
		}
	}
	return fmt.Errorf("job not found: %s", name)
}

func (r *Runner) RunTaskDraft(id string) (interface{}, error) {
	result, _, err := r.runTaskDraftByID(id, true, "manual")
	return result, err
}

func (r *Runner) RunSchedule(id string) (interface{}, error) {
	s, err := r.loadSchedule(id)
	if err != nil {
		return nil, err
	}
	result, _, err := r.runScheduleTarget(context.Background(), s, "manual")
	return result, err
}

func (r *Runner) RunScheduleOnce(ctx context.Context, id string) (webhook.DeliveryResult, error) {
	s, err := r.loadSchedule(id)
	if err != nil {
		return webhook.DeliveryResult{Attempted: false}, err
	}
	_, delivery, err := r.runScheduleTarget(ctx, s, "manual")
	return delivery, err
}

func (r *Runner) RunComplexTask(task storage.ComplexTask) (*ComplexTaskRunResult, error) {
	return r.runComplexTask(task, "manual")
}

func (r *Runner) runComplexTask(task storage.ComplexTask, triggerSource string) (*ComplexTaskRunResult, error) {
	startedAt := time.Now()
	result := &ComplexTaskRunResult{
		TaskID:    task.ID,
		TaskName:  task.Name,
		StartedAt: startedAt.Format(time.RFC3339),
		Steps:     make([]ComplexTaskStepResult, 0, len(task.Steps)),
	}

	var current interface{}
	for _, step := range task.Steps {
		item := ComplexTaskStepResult{
			StepID: step.ID,
			Type:   step.Type,
			Name:   step.Name,
		}
		out, err := r.executeComplexTaskStep(step, current)
		if err != nil {
			item.OK = false
			item.Error = err.Error()
			result.Steps = append(result.Steps, item)
			result.OK = false
			result.FinalOutput = current
			finishedAt := time.Now()
			result.FinishedAt = finishedAt.Format(time.RFC3339)
			r.recordComplexTaskRun(task, triggerSource, startedAt, finishedAt, result, err)
			return result, err
		}
		item.OK = true
		item.Output = out
		current = out
		result.Steps = append(result.Steps, item)
	}

	result.OK = true
	result.FinalOutput = current
	finishedAt := time.Now()
	result.FinishedAt = finishedAt.Format(time.RFC3339)
	r.recordComplexTaskRun(task, triggerSource, startedAt, finishedAt, result, nil)
	return result, nil
}

func (r *Runner) runJob(j storage.Job) error {
	start := time.Now()
	err := r.executeJob(j)
	finishedAt := time.Now()
	dur := finishedAt.Sub(start).Milliseconds()

	r.statusMu.Lock()
	st := r.status[j.Name]
	if st == nil {
		st = &JobStatus{}
		r.status[j.Name] = st
	}
	st.LastRun = finishedAt
	st.DurationMs = dur
	st.LastOK = err == nil
	if err != nil {
		st.LastError = err.Error()
	} else {
		st.LastError = ""
	}
	r.statusMu.Unlock()
	r.recordLegacyJobRun(j, "scheduler", start, finishedAt, err)
	r.refreshNextRunFor(j.Name)
	return err
}

func (r *Runner) runSchedule(s storage.JobSchedule) error {
	_, _, err := r.runScheduleTarget(context.Background(), s, "scheduler")
	return err
}

func (r *Runner) executeJob(j storage.Job) error {
	switch j.Type {
	case "api_poll":
		tid, _ := j.Params["template_id"].(string)
		if tid == "" {
			return fmt.Errorf("api_poll requires params.template_id")
		}
		input := map[string]interface{}{}
		if m, ok := j.Params["input"].(map[string]interface{}); ok && m != nil {
			input = m
		}
		_, err := r.executor.Execute(api.ExecuteRequest{
			TemplateID: tid,
			Query:      stringMapToIFMap(coerceStringMap(input)),
			Body:       extractBody(input),
		})
		return err

	case "api_write":
		tid, _ := j.Params["template_id"].(string)
		if tid == "" {
			return fmt.Errorf("api_write requires params.template_id")
		}
		// v0：写入任务默认应为 disabled；若启用则要求 safety.dry_run 已做过预览（这里仅做最小保护）
		if sm, ok := j.Params["safety"].(map[string]interface{}); ok {
			if req, ok := sm["require_preview_token"].(bool); ok && req {
				if confirmed, ok := sm["preview_confirmed"].(bool); !ok || !confirmed {
					return fmt.Errorf("api_write requires preview confirmation before enabling")
				}
			}
		}
		input := map[string]interface{}{}
		if m, ok := j.Params["input"].(map[string]interface{}); ok && m != nil {
			input = m
		}
		_, err := r.executor.Execute(api.ExecuteRequest{
			TemplateID: tid,
			PathParams: stringMapToIFMap(coerceStringMap(input)), // 允许把变量放在 input 里
			Query:      stringMapToIFMap(coerceStringMap(input)),
			Body:       extractBody(input),
		})
		return err

	default:
		return fmt.Errorf("unsupported job type: %s", j.Type)
	}
}

func (r *Runner) executeSchedule(ctx context.Context, s storage.JobSchedule) (interface{}, webhook.DeliveryResult, error) {
	targetType, targetID := NormalizeTarget(s.TargetType, s.TargetID, s.DraftID)
	result, err := r.executeTargetOutputForSchedule(targetType, targetID)
	if err != nil {
		return result, webhook.DeliveryResult{Attempted: false}, err
	}
	return result, r.deliverScheduleWebhook(ctx, s, targetType, targetID, result), nil
}

func (r *Runner) runScheduleTarget(ctx context.Context, s storage.JobSchedule, triggerSource string) (interface{}, webhook.DeliveryResult, error) {
	startedAt := time.Now()
	targetType, targetID := NormalizeTarget(s.TargetType, s.TargetID, s.DraftID)

	logger.Info("[任务] 定时任务开始执行",
		zap.String("schedule_id", s.ID),
		zap.String("schedule_type", s.ScheduleType),
		zap.String("cron", s.Cron),
		zap.String("interval", s.Interval),
		zap.String("target_type", targetType),
		zap.String("target_id", targetID),
		zap.String("trigger_source", triggerSource),
		zap.Bool("enabled", s.Enabled),
	)

	result, delivery, err := r.executeSchedule(ctx, s)
	finishedAt := time.Now()
	duration := finishedAt.Sub(startedAt)

	r.statusMu.Lock()
	st := r.ensureStatus(r.scheduleStatus, s.ID)
	st.LastRun = finishedAt
	st.DurationMs = duration.Milliseconds()
	st.LastOK = err == nil
	if err != nil {
		st.LastError = err.Error()
	} else {
		st.LastError = ""
	}
	r.statusMu.Unlock()

	if err != nil {
		logger.Error("[任务] 定时任务执行失败",
			zap.String("schedule_id", s.ID),
			zap.Duration("duration", duration),
			zap.Error(err),
		)
	} else {
		logger.Info("[任务] 定时任务执行成功",
			zap.String("schedule_id", s.ID),
			zap.Duration("duration", duration),
			zap.Bool("webhook_attempted", delivery.Attempted),
			zap.Bool("webhook_ok", delivery.OK),
			zap.Int("webhook_status_code", delivery.StatusCode),
		)
	}

	r.recordScheduleRun(s, targetType, targetID, triggerSource, startedAt, finishedAt, result, err)
	r.refreshNextRunForSchedule(s.ID)
	return result, delivery, err
}

func (r *Runner) executeTaskDraft(d storage.TaskDraft) error {
	_, _, err := r.runTrackedTaskDraft(d, true, "internal")
	return err
}

func (r *Runner) runTaskDraft(d storage.TaskDraft) (interface{}, webhook.DeliveryResult, error) {
	return r.runTaskDraftWithDelivery(d, true)
}

func (r *Runner) ExecuteTransientTaskDraft(d storage.TaskDraft, enableWebhook bool) (interface{}, webhook.DeliveryResult, error) {
	return r.runTrackedTaskDraft(d, enableWebhook, "manual")
}

func (r *Runner) runTaskDraftByID(id string, enableWebhook bool, triggerSource string) (interface{}, webhook.DeliveryResult, error) {
	draft, err := r.loadTaskDraft(id)
	if err != nil {
		return nil, webhook.DeliveryResult{Attempted: false}, err
	}
	return r.runTrackedTaskDraft(draft, enableWebhook, triggerSource)
}

func (r *Runner) runTrackedTaskDraft(d storage.TaskDraft, enableWebhook bool, triggerSource string) (interface{}, webhook.DeliveryResult, error) {
	startedAt := time.Now()

	logger.Info("[任务] 任务草稿开始执行",
		zap.String("task_id", d.ID),
		zap.String("task_name", d.Name),
		zap.String("mode", d.Mode),
		zap.String("trigger_source", triggerSource),
		zap.Bool("enable_webhook", enableWebhook),
	)

	result, delivery, err := r.runTaskDraftWithDelivery(d, enableWebhook)
	duration := time.Since(startedAt)

	if err != nil {
		logger.Error("[任务] 任务草稿执行失败",
			zap.String("task_id", d.ID),
			zap.String("task_name", d.Name),
			zap.Duration("duration", duration),
			zap.Error(err),
		)
	} else {
		logger.Info("[任务] 任务草稿执行成功",
			zap.String("task_id", d.ID),
			zap.String("task_name", d.Name),
			zap.Duration("duration", duration),
			zap.Bool("webhook_attempted", delivery.Attempted),
			zap.Bool("webhook_ok", delivery.OK),
			zap.Int("webhook_status_code", delivery.StatusCode),
		)
	}

	r.recordTaskDraftRun(d, triggerSource, startedAt, time.Now(), result, err)
	return result, delivery, err
}

func normalizeTaskDraftExecutionMode(d storage.TaskDraft) string {
	mode := strings.TrimSpace(d.Mode)
	hasTransform := len(d.TransformConfig) > 0
	hasLLM := len(d.LLMConfig) > 0
	hasOutput := len(d.OutputConfig) > 0

	switch mode {
	case "workflow":
		return "workflow"
	case "api_plus_llm":
		if hasOutput || hasTransform {
			return "workflow"
		}
		if hasLLM {
			return "api_plus_llm"
		}
	case "api_plus_process":
		if hasLLM || hasOutput {
			return "workflow"
		}
		if hasTransform {
			return "api_plus_process"
		}
	}

	if hasOutput {
		return "workflow"
	}
	if hasLLM {
		return "api_plus_llm"
	}
	if hasTransform {
		return "api_plus_process"
	}
	return "api_only"
}

func (r *Runner) runTaskDraftWithDelivery(d storage.TaskDraft, enableWebhook bool) (interface{}, webhook.DeliveryResult, error) {
	d = storage.NormalizeTaskDraft(d)
	input := d.InputConfig
	if input == nil {
		input = map[string]interface{}{}
	}
	req := api.ExecuteRequest{
		TemplateID: firstNonEmpty(d.SourceTemplateID, asString(input["template_id"])),
		Method:     asString(input["method"]),
		Path:       asString(input["path"]),
		Query:      stringMapToIFMap(coerceStringMap(mapValue(input, "query"))),
		PathParams: stringMapToIFMap(coerceStringMap(mapValue(input, "path_params"))),
		Body:       extractBody(mapValue(input, "body")),
	}
	if len(req.Body) == 0 {
		if bodyMap := mapValue(input, "body"); len(bodyMap) > 0 {
			req.Body = bodyMap
		}
	}
	if req.TemplateID == "" && req.Method == "" && req.Path == "" {
		return nil, webhook.DeliveryResult{Attempted: false}, fmt.Errorf("task draft requires template_id or method/path")
	}
	resp, err := r.executor.Execute(req)
	if err != nil {
		return nil, webhook.DeliveryResult{Attempted: false}, err
	}
	var current interface{} = map[string]interface{}{
		"http_status": resp.HTTPStatus,
		"latency_ms":  resp.LatencyMs,
		"body":        decodeJSONOrRaw(resp.Body),
	}
	mode := normalizeTaskDraftExecutionMode(d)
	if mode == "api_only" {
		return current, r.taskDraftDeliveryResult(d, current, enableWebhook), nil
	}
	if len(d.TransformConfig) > 0 {
		step := storage.ComplexTaskStep{ID: d.ID + "_transform", Type: "data_transform", Name: "transform", Config: d.TransformConfig}
		current, err = r.executeComplexTaskStep(step, current)
		if err != nil {
			return nil, webhook.DeliveryResult{Attempted: false}, err
		}
	}
	if len(d.LLMConfig) > 0 && (mode == "api_plus_llm" || mode == "workflow" || mode == "api_plus_process") {
		step := storage.ComplexTaskStep{ID: d.ID + "_llm", Type: "llm_inference", Name: "llm", Config: d.LLMConfig}
		current, err = r.executeComplexTaskStep(step, current)
		if err != nil {
			return nil, webhook.DeliveryResult{Attempted: false}, err
		}
	}
	if len(d.OutputConfig) > 0 && mode != "api_plus_process" {
		step := storage.ComplexTaskStep{ID: d.ID + "_output", Type: "output", Name: "output", Config: d.OutputConfig}
		current, err = r.executeComplexTaskStep(step, current)
		if err != nil {
			return nil, webhook.DeliveryResult{Attempted: false}, err
		}
	}
	return current, r.taskDraftDeliveryResult(d, current, enableWebhook), nil
}

func (r *Runner) taskDraftDeliveryResult(d storage.TaskDraft, result interface{}, enableWebhook bool) webhook.DeliveryResult {
	if !enableWebhook {
		return webhook.DeliveryResult{Attempted: false}
	}
	return r.deliverTaskDraftWebhook(d, result)
}

func (r *Runner) loadSchedule(id string) (storage.JobSchedule, error) {
	if r.scheduleRepo == nil {
		return storage.JobSchedule{}, fmt.Errorf("schedule repository is nil")
	}
	got, found, err := r.scheduleRepo.Get(id)
	if err != nil {
		return storage.JobSchedule{}, err
	}
	if !found {
		return storage.JobSchedule{}, fmt.Errorf("job schedule not found: %s", id)
	}
	return storage.NormalizeJobSchedule(got), nil
}

func (r *Runner) loadTaskDraft(id string) (storage.TaskDraft, error) {
	if draft, ok := r.taskDrafts[id]; ok {
		return storage.NormalizeTaskDraft(draft), nil
	}
	if r.taskDraftRepo == nil {
		return storage.TaskDraft{}, fmt.Errorf("task draft repository is nil")
	}
	got, found, err := r.taskDraftRepo.Get(id)
	if err != nil {
		return storage.TaskDraft{}, err
	}
	if !found {
		return storage.TaskDraft{}, fmt.Errorf("task draft not found: %s", id)
	}
	return storage.NormalizeTaskDraft(got), nil
}

func (r *Runner) loadComplexTask(id string) (storage.ComplexTask, error) {
	if r.complexTaskRepo == nil {
		return storage.ComplexTask{}, fmt.Errorf("complex task repository is nil")
	}
	got, found, err := r.complexTaskRepo.Get(id)
	if err != nil {
		return storage.ComplexTask{}, err
	}
	if !found {
		return storage.ComplexTask{}, fmt.Errorf("complex task not found: %s", id)
	}
	return got, nil
}

func (r *Runner) loadExternalIPSyncTask(id string) (storage.ExternalIPSyncTask, error) {
	if r.externalIPSyncRepo == nil {
		return storage.ExternalIPSyncTask{}, fmt.Errorf("external ip sync repository is nil")
	}
	got, found, err := r.externalIPSyncRepo.Get(id)
	if err != nil {
		return storage.ExternalIPSyncTask{}, err
	}
	if !found {
		return storage.ExternalIPSyncTask{}, fmt.Errorf("external ip sync task not found: %s", id)
	}
	return storage.NormalizeExternalIPSyncTask(got), nil
}

func (r *Runner) RunExternalIPSyncTask(id string) (*service.ExternalIPSyncExecutionSummary, error) {
	return r.runExternalIPSyncTask(id)
}

func (r *Runner) runExternalIPSyncTask(id string) (*service.ExternalIPSyncExecutionSummary, error) {
	task, err := r.loadExternalIPSyncTask(id)
	if err != nil {
		return nil, err
	}
	return r.runExternalIPSyncTaskWithTask(task)
}

func (r *Runner) runExternalIPSyncTaskWithTask(task storage.ExternalIPSyncTask) (*service.ExternalIPSyncExecutionSummary, error) {
	task = storage.NormalizeExternalIPSyncTask(task)
	r.ensureExternalIPSyncDeps()

	createMode := strings.ToLower(strings.TrimSpace(task.CreateMode))
	logger.Info("[外部IP同步] 任务开始执行",
		zap.String("task_id", task.ID),
		zap.String("task_name", task.Name),
		zap.String("create_mode", createMode),
		zap.String("resource_id", task.ResourceID),
		zap.String("ip_version", task.IPVersion),
		zap.String("source_url", task.SourceURL),
		zap.String("feilian_api_path", task.FeilianAPIPath),
		zap.Bool("dry_run", task.DryRun),
		zap.Bool("skip_when_empty", task.SkipWhenEmpty),
	)

	summary := &service.ExternalIPSyncExecutionSummary{
		TaskID:       task.ID,
		ResourceID:   task.ResourceID,
		IPVersion:    task.IPVersion,
		WriteAPIPath: task.FeilianAPIPath,
		DryRun:       task.DryRun,
		Status:       "success",
	}

	logger.Info("[外部IP同步] 开始获取Google IP列表", zap.String("source_url", task.SourceURL))
	sourceCIDRs, err := r.fetchGoogleCIDRs(task)
	if err != nil {
		logger.Error("[外部IP同步] 获取Google IP列表失败",
			zap.String("task_id", task.ID),
			zap.Error(err),
		)
		summary.Status = "failed"
		summary.ErrorMessage = err.Error()
		return summary, err
	}
	summary.SourceTotal = len(sourceCIDRs)
	summary.FilteredTotal = len(sourceCIDRs)
	logger.Info("[外部IP同步] Google IP列表获取成功",
		zap.String("task_id", task.ID),
		zap.Int("cidr_count", len(sourceCIDRs)),
	)

	var existingCIDRs []string
	if createMode == storage.FeishuResourceCreateModeAdd {
		logger.Info("[外部IP同步] 新增模式，跳过获取现有资源CIDR", zap.String("task_id", task.ID))
		existingCIDRs = []string{}
	} else {
		logger.Info("[外部IP同步] 选择模式，开始获取飞连资源现有CIDR",
			zap.String("task_id", task.ID),
			zap.String("resource_id", task.ResourceID),
		)
		existingCIDRs, err = r.loadFeilianResourceCIDRs(task.ResourceID)
		if err != nil {
			logger.Error("[外部IP同步] 获取飞连资源CIDR失败",
				zap.String("task_id", task.ID),
				zap.String("resource_id", task.ResourceID),
				zap.Error(err),
			)
			summary.Status = "failed"
			summary.ErrorMessage = err.Error()
			return summary, err
		}
		logger.Info("[外部IP同步] 飞连资源CIDR获取成功",
			zap.String("task_id", task.ID),
			zap.String("resource_id", task.ResourceID),
			zap.Int("existing_cidr_count", len(existingCIDRs)),
		)
	}
	summary.ExistingTotal = len(existingCIDRs)

	toAdd := diffCIDRs(sourceCIDRs, existingCIDRs)
	summary.ToAddTotal = len(toAdd)
	logger.Info("[外部IP同步] CIDR差异计算完成",
		zap.String("task_id", task.ID),
		zap.Int("source_total", len(sourceCIDRs)),
		zap.Int("existing_total", len(existingCIDRs)),
		zap.Int("to_add_total", len(toAdd)),
	)

	if len(toAdd) == 0 && task.SkipWhenEmpty {
		logger.Info("[外部IP同步] 无增量CIDR且配置跳过空更新，任务结束",
			zap.String("task_id", task.ID),
		)
		summary.Status = "skipped"
		return summary, nil
	}
	if task.DryRun {
		logger.Info("[外部IP同步] Dry-run模式，仅预览不写入，任务结束",
			zap.String("task_id", task.ID),
			zap.Int("to_add_total", len(toAdd)),
		)
		summary.Status = "dry_run"
		return summary, nil
	}

	logger.Info("[外部IP同步] 开始写入飞连",
		zap.String("task_id", task.ID),
		zap.String("create_mode", createMode),
		zap.String("api_path", task.FeilianAPIPath),
		zap.Int("cidr_count", len(toAdd)),
	)
	if err := r.writeFeilianCIDRs(task, toAdd); err != nil {
		logger.Error("[外部IP同步] 写入飞连失败",
			zap.String("task_id", task.ID),
			zap.String("create_mode", createMode),
			zap.String("api_path", task.FeilianAPIPath),
			zap.Error(err),
		)
		summary.Status = "failed"
		summary.ErrorMessage = err.Error()
		return summary, err
	}
	summary.AddedTotal = len(toAdd)

	logger.Info("[外部IP同步] 任务执行完成",
		zap.String("task_id", task.ID),
		zap.String("status", "success"),
		zap.Int("added_total", len(toAdd)),
	)
	return summary, nil
}

func (r *Runner) ensureExternalIPSyncDeps() {
	if r.fetchGoogleCIDRs == nil {
		r.fetchGoogleCIDRs = r.defaultFetchGoogleCIDRs
	}
	if r.loadFeilianResourceCIDRs == nil {
		r.loadFeilianResourceCIDRs = r.defaultLoadFeilianResourceCIDRs
	}
	if r.writeFeilianCIDRs == nil {
		r.writeFeilianCIDRs = r.defaultWriteFeilianCIDRs
	}
}

func (r *Runner) defaultFetchGoogleCIDRs(task storage.ExternalIPSyncTask) ([]string, error) {
	task = storage.NormalizeExternalIPSyncTask(task)
	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Get(task.SourceURL)
	if err != nil {
		return nil, fmt.Errorf("fetch google cidrs: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch google cidrs: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read google cidrs response: %w", err)
	}

	var payload googleIPRanges
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode google cidrs response: %w", err)
	}
	return filterGoogleCIDRs(payload, task.IPVersion), nil
}

func (r *Runner) defaultLoadFeilianResourceCIDRs(resourceID string) ([]string, error) {
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" {
		return nil, fmt.Errorf("load feilian resource cidrs: resource_id is required")
	}
	if r.client == nil {
		return nil, fmt.Errorf("load feilian resource cidrs: sealsuite client is nil")
	}

	logger.Debug("[外部IP同步-加载资源] 开始获取飞连资源详情",
		zap.String("resource_id", resourceID),
	)

	statusCode, body, err := r.client.DoRaw(http.MethodGet, "/api/open/v1/addr/management/detail", map[string]string{
		"resource_id": resourceID,
	}, nil)
	if err != nil {
		logger.Warn("[外部IP同步-加载资源] 获取飞连资源详情失败，降级为全量追加",
			zap.String("resource_id", resourceID),
			zap.Error(err),
		)
		return []string{}, nil
	}

	if statusCode < 200 || statusCode >= 300 {
		logger.Warn("[外部IP同步-加载资源] 飞连资源详情返回非200状态，降级为全量追加",
			zap.String("resource_id", resourceID),
			zap.Int("status_code", statusCode),
			zap.String("response_body", truncateString(string(body), 500)),
		)
		return []string{}, nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		logger.Warn("[外部IP同步-加载资源] 解析飞连资源详情JSON失败，降级为全量追加",
			zap.String("resource_id", resourceID),
			zap.Int("status_code", statusCode),
			zap.String("response_body", truncateString(string(body), 500)),
			zap.Error(err),
		)
		return []string{}, nil
	}

	cidrs := extractCIDRsFromPayload(payload)
	logger.Info("[外部IP同步-加载资源] 飞连资源详情获取成功",
		zap.String("resource_id", resourceID),
		zap.Int("cidr_count", len(cidrs)),
	)
	return cidrs, nil
}

func (r *Runner) defaultWriteFeilianCIDRs(task storage.ExternalIPSyncTask, cidrs []string) error {
	task = storage.NormalizeExternalIPSyncTask(task)
	if task.ResourceID == "" {
		return fmt.Errorf("write feilian cidrs: resource_id is required")
	}
	if r.client == nil {
		return fmt.Errorf("write feilian cidrs: sealsuite client is nil")
	}
	if len(cidrs) == 0 {
		return nil
	}

	resp, err := r.client.Post(task.FeilianAPIPath, map[string]interface{}{
		"resource_id": task.ResourceID,
		"cidrs":       cidrs,
	})
	if err != nil {
		return fmt.Errorf("write feilian cidrs: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("write feilian cidrs: empty response")
	}
	return nil
}

func (r *Runner) RefreshFeishuResources() ([]storage.FeishuResourceItem, error) {
	if r.client == nil {
		return nil, fmt.Errorf("refresh feishu resources: sealsuite client is nil")
	}
	if r.feishuResourcesRepo == nil {
		return nil, fmt.Errorf("refresh feishu resources: repository is nil")
	}

	allItems := make([]storage.FeishuResourceItem, 0)
	for _, resourceType := range []string{"ip", "domain"} {
		query := map[string]string{
			"type": resourceType,
		}
		_, body, err := r.client.DoRaw(http.MethodGet, "/api/open/v1/addr/management/list", query, nil)
		if err != nil {
			logger.Warn("[资源清单] 获取资源列表失败，跳过该类型",
				zap.String("type", resourceType),
				zap.Error(err),
			)
			continue
		}

		var resp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				Count int                      `json:"count"`
				Items []map[string]interface{} `json:"items"`
				Apps  []map[string]interface{} `json:"apps"`
			} `json:"data"`
			Items []map[string]interface{} `json:"items"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			logger.Warn("[资源清单] 解析响应失败，跳过该类型",
				zap.String("type", resourceType),
				zap.Error(err),
			)
			continue
		}
		if resp.Code != 0 {
			logger.Warn("[资源清单] API返回错误，跳过该类型",
				zap.String("type", resourceType),
				zap.Int("code", resp.Code),
				zap.String("message", resp.Message),
			)
			continue
		}

		rawItems := resp.Data.Items
		if len(rawItems) == 0 {
			rawItems = resp.Data.Apps
		}
		if len(rawItems) == 0 {
			rawItems = resp.Items
		}

		for _, raw := range rawItems {
			item := storage.FeishuResourceItem{}
			if v, ok := raw["id"]; ok && v != nil {
				item.ID = fmt.Sprintf("%v", v)
			}
			if v, ok := raw["resource_id"]; ok && v != nil {
				item.ID = fmt.Sprintf("%v", v)
			}
			if v, ok := raw["name"]; ok && v != nil {
				item.Name = fmt.Sprintf("%v", v)
			}
			if v, ok := raw["resource_name"]; ok && v != nil {
				item.Name = fmt.Sprintf("%v", v)
			}
			item.Type = resourceType
			if v, ok := raw["type"]; ok && v != nil {
				typeStr := fmt.Sprintf("%v", v)
				if typeStr != "" {
					item.Type = typeStr
				}
			}
			if v, ok := raw["tag_ids"]; ok && v != nil {
				item.TagIDs = fmt.Sprintf("%v", v)
			}
			if v, ok := raw["tag_names"]; ok && v != nil {
				item.TagNames = fmt.Sprintf("%v", v)
			}
			if tags, ok := raw["tags"].([]interface{}); ok && len(tags) > 0 {
				var tagIDs, tagNames []string
				for _, t := range tags {
					if tagMap, ok := t.(map[string]interface{}); ok {
						if id, ok := tagMap["id"]; ok && id != nil {
							tagIDs = append(tagIDs, fmt.Sprintf("%v", id))
						}
						if name, ok := tagMap["name"]; ok && name != nil {
							tagNames = append(tagNames, fmt.Sprintf("%v", name))
						}
					}
				}
				if len(tagIDs) > 0 {
					item.TagIDs = strings.Join(tagIDs, ",")
				}
				if len(tagNames) > 0 {
					item.TagNames = strings.Join(tagNames, ",")
				}
			}
			if item.ID == "" {
				continue
			}
			if rawBytes, jsonErr := json.Marshal(raw); jsonErr == nil {
				item.RawJSON = string(rawBytes)
			}
			allItems = append(allItems, item)
		}
	}

	if len(allItems) == 0 {
		logger.Warn("[资源清单] 未获取到任何资源")
		return allItems, nil
	}

	logger.Info("[资源清单] 刷新完成",
		zap.Int("总资源数", len(allItems)),
	)

	count, err := r.feishuResourcesRepo.ReplaceAll(allItems)
	if err != nil {
		return nil, fmt.Errorf("refresh feishu resources: store: %w", err)
	}
	return allItems[:count], nil
}

func safeBool(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case int:
		return val != 0
	case string:
		return val == "1" || val == "true" || val == "True" || val == "TRUE"
	default:
		return fmt.Sprintf("%v", v) == "1"
	}
}

func (r *Runner) SyncFeishuDevices() ([]storage.FeishuDeviceItem, error) {
	if r.client == nil {
		return nil, fmt.Errorf("sync feishu devices: sealsuite client is nil")
	}
	if r.feishuDevicesRepo == nil {
		return nil, fmt.Errorf("sync feishu devices: repository is nil")
	}

	allDevices := make([]storage.FeishuDeviceItem, 0)
	maxPages := 500
	offset := 0
	limit := 100

	for i := 0; i < maxPages; i++ {
		params := map[string]string{
			"limit":  fmt.Sprintf("%d", limit),
			"offset": fmt.Sprintf("%d", offset),
		}

		_, body, err := r.client.DoRaw(http.MethodGet, "/api/open/v1/device/search", params, nil)
		if err != nil {
			return allDevices, fmt.Errorf("sync feishu devices: offset %d: %w", offset, err)
		}

		var resp struct {
			Data struct {
				Items     []map[string]interface{} `json:"items"`
				Devices   []map[string]interface{} `json:"devices"`
				PageToken string                   `json:"page_token"`
			} `json:"data"`
			Items     []map[string]interface{} `json:"items"`
			Devices   []map[string]interface{} `json:"devices"`
			PageToken string                   `json:"page_token"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return allDevices, fmt.Errorf("sync feishu devices: parse offset %d: %w", offset, err)
		}

		rawItems := resp.Data.Items
		if len(rawItems) == 0 {
			rawItems = resp.Data.Devices
		}
		if len(rawItems) == 0 {
			rawItems = resp.Items
		}
		if len(rawItems) == 0 {
			rawItems = resp.Devices
		}

		for _, raw := range rawItems {
			device := storage.FeishuDeviceItem{}
			deviceInfo, _ := raw["device_info"].(map[string]interface{})
			getField := func(key string) (interface{}, bool) {
				if v, ok := raw[key]; ok && v != nil {
					return v, true
				}
				if deviceInfo != nil {
					if v, ok := deviceInfo[key]; ok && v != nil {
						return v, true
					}
				}
				return nil, false
			}
			if v, ok := getField("did"); ok {
				device.DID = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("user_id"); ok {
				device.UserID = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("os"); ok {
				device.OS = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("client_ip"); ok {
				device.ClientIP = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("client_ip_location"); ok {
				device.ClientIPLocation = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("device_status"); ok {
				device.DeviceStatus = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("nic_type"); ok {
				device.NICType = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("mac_addrs"); ok {
				device.MacAddrs = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("is_vm"); ok {
				device.IsVM = safeBool(v)
			}
			if v, ok := getField("serial_number"); ok {
				device.SerialNumber = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("mac_addr"); ok {
				device.MacAddr = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("is_virtual"); ok {
				device.IsVirtual = safeBool(v)
			}
			if v, ok := getField("is_default"); ok {
				device.IsDefault = safeBool(v)
			}
			if v, ok := getField("hdd_serial_numbers"); ok {
				device.HDDSerialNumbers = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("ssd_serial_numbers"); ok {
				device.SSDSerialNumbers = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("cpu_serial_number"); ok {
				device.CPUSerialNumber = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("windows_ad_domain_name"); ok {
				device.WindowsADDomainName = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("groups_id"); ok {
				device.GroupsID = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("groups_name"); ok {
				device.GroupsName = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("groups_mode"); ok {
				device.GroupsMode = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("device_type"); ok {
				device.DeviceType = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("device_name"); ok {
				device.DeviceName = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("name"); ok {
				if device.DeviceName == "" {
					device.DeviceName = fmt.Sprintf("%v", v)
				}
			}
			if v, ok := getField("full_name"); ok {
				device.FullName = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("user_name"); ok {
				if device.FullName == "" {
					device.FullName = fmt.Sprintf("%v", v)
				}
			}
			if v, ok := getField("department_name"); ok {
				device.DepartmentName = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("mem_serial_numbers"); ok {
				device.MemSerialNumbers = fmt.Sprintf("%v", v)
			}
			if device.DID != "" {
				allDevices = append(allDevices, device)
			}
		}

		if len(rawItems) == 0 {
			break
		}
		offset += limit
	}

	count, err := r.feishuDevicesRepo.ReplaceAll(allDevices)
	if err != nil {
		return allDevices, fmt.Errorf("sync feishu devices: store: %w", err)
	}

	for i, device := range allDevices[:count] {
		trustedStatus, _ := r.FetchDeviceTrustedStatus(device.DID)
		if trustedStatus != "" {
			allDevices[i].TrustedStatus = trustedStatus
			_ = r.feishuDevicesRepo.UpdateTrustedStatus(device.DID, trustedStatus)
		}
	}

	return allDevices[:count], nil
}

func (r *Runner) RefreshDeviceGroups() ([]storage.DeviceGroupItem, error) {
	if r.client == nil {
		return nil, fmt.Errorf("refresh device groups: sealsuite client is nil")
	}
	if r.deviceGroupsRepo == nil {
		return nil, fmt.Errorf("refresh device groups: repository is nil")
	}

	_, body, err := r.client.DoRaw(http.MethodGet, "/api/open/v1/device/group/list", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("refresh device groups: %w", err)
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Groups []map[string]interface{} `json:"groups"`
			Total  int                      `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("refresh device groups: parse response: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("refresh device groups: api error code=%d message=%s", resp.Code, resp.Message)
	}

	rawItems := resp.Data.Groups

	items := make([]storage.DeviceGroupItem, 0, len(rawItems))
	for _, raw := range rawItems {
		item := storage.DeviceGroupItem{}
		if v, ok := raw["id"]; ok && v != nil {
			item.ID = fmt.Sprintf("%v", v)
		}
		if v, ok := raw["group_id"]; ok && v != nil {
			item.ID = fmt.Sprintf("%v", v)
		}
		if v, ok := raw["name"]; ok && v != nil {
			item.Name = fmt.Sprintf("%v", v)
		}
		if v, ok := raw["group_name"]; ok && v != nil {
			item.Name = fmt.Sprintf("%v", v)
		}
		if item.ID == "" {
			continue
		}
		if rawBytes, jsonErr := json.Marshal(raw); jsonErr == nil {
			item.RawJSON = string(rawBytes)
		}
		items = append(items, item)
	}

	logger.Info("[设备分组] 刷新完成",
		zap.Int("总分组数", len(items)),
	)

	count, err := r.deviceGroupsRepo.ReplaceAll(items)
	if err != nil {
		return nil, fmt.Errorf("refresh device groups: store: %w", err)
	}
	return items[:count], nil
}

func (r *Runner) GetDevicesByGroup(groupID string) (*storage.DeviceGroupDetail, error) {
	if r.client == nil {
		return nil, fmt.Errorf("get devices by group: sealsuite client is nil")
	}
	if r.feishuDevicesRepo == nil {
		return nil, fmt.Errorf("get devices by group: repository is nil")
	}

	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, fmt.Errorf("get devices by group: group_id is required")
	}

	allDevices := make([]storage.DeviceDetailItem, 0)
	maxPages := 100
	offset := 0
	limit := 100
	totalCount := 0

	for i := 0; i < maxPages; i++ {
		params := map[string]string{
			"group_id": groupID,
			"limit":    fmt.Sprintf("%d", limit),
			"offset":   fmt.Sprintf("%d", offset),
		}

		_, respBody, err := r.client.DoRaw(http.MethodGet, "/api/open/v1/device/search", params, nil)
		if err != nil {
			return nil, fmt.Errorf("get devices by group: offset %d: %w", offset, err)
		}

		var resp struct {
			Data struct {
				Items   []map[string]interface{} `json:"items"`
				Devices []map[string]interface{} `json:"devices"`
				Count   int                      `json:"count"`
				Total   int                      `json:"total"`
			} `json:"data"`
			Items   []map[string]interface{} `json:"items"`
			Devices []map[string]interface{} `json:"devices"`
			Count   int                      `json:"count"`
			Total   int                      `json:"total"`
		}
		if err := json.Unmarshal(respBody, &resp); err != nil {
			return nil, fmt.Errorf("get devices by group: parse offset %d: %w", offset, err)
		}

		rawItems := resp.Data.Items
		if len(rawItems) == 0 {
			rawItems = resp.Data.Devices
		}
		if len(rawItems) == 0 {
			rawItems = resp.Items
		}
		if len(rawItems) == 0 {
			rawItems = resp.Devices
		}

		if totalCount == 0 {
			if resp.Data.Count > 0 {
				totalCount = resp.Data.Count
			} else if resp.Count > 0 {
				totalCount = resp.Count
			} else if resp.Data.Total > 0 {
				totalCount = resp.Data.Total
			} else if resp.Total > 0 {
				totalCount = resp.Total
			}
		}

		for _, raw := range rawItems {
			device := storage.DeviceDetailItem{
				GroupID: groupID,
			}
			deviceInfo, _ := raw["device_info"].(map[string]interface{})
			getField := func(key string) (interface{}, bool) {
				if v, ok := raw[key]; ok && v != nil {
					return v, true
				}
				if deviceInfo != nil {
					if v, ok := deviceInfo[key]; ok && v != nil {
						return v, true
					}
				}
				return nil, false
			}
			if v, ok := getField("did"); ok {
				device.DID = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("user_id"); ok {
				device.UserID = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("device_name"); ok {
				device.DeviceName = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("name"); ok {
				if device.DeviceName == "" {
					device.DeviceName = fmt.Sprintf("%v", v)
				}
			}
			if v, ok := getField("os"); ok {
				device.OS = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("full_name"); ok {
				device.FullName = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("user_name"); ok {
				if device.FullName == "" {
					device.FullName = fmt.Sprintf("%v", v)
				}
			}
			if v, ok := getField("department_name"); ok {
				device.DepartmentName = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("device_status"); ok {
				device.DeviceStatus = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("mac_addrs"); ok {
				device.MacAddrs = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("serial_number"); ok {
				device.SerialNumber = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("hdd_serial_numbers"); ok {
				device.HDDSerialNumbers = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("ssd_serial_numbers"); ok {
				device.SSDSerialNumbers = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("cpu_serial_number"); ok {
				device.CPUSerialNumber = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("mem_serial_numbers"); ok {
				device.MemSerialNumbers = fmt.Sprintf("%v", v)
			}
			if v, ok := getField("groups_name"); ok {
				device.GroupName = fmt.Sprintf("%v", v)
			}
			if device.DID != "" {
				allDevices = append(allDevices, device)
			}
		}

		pageItems := len(rawItems)
		if pageItems == 0 {
			break
		}

		if totalCount > 0 && len(allDevices) >= totalCount {
			break
		}
		offset += limit
	}

	if totalCount == 0 {
		totalCount = len(allDevices)
	}

	detail := &storage.DeviceGroupDetail{
		GroupID: groupID,
		Count:   totalCount,
		Items:   allDevices,
	}

	go func() {
		for _, d := range allDevices {
			item := storage.FeishuDeviceItem{
				DID:              d.DID,
				UserID:           d.UserID,
				DeviceName:       d.DeviceName,
				FullName:         d.FullName,
				DepartmentName:   d.DepartmentName,
				OS:               d.OS,
				DeviceStatus:     d.DeviceStatus,
				MacAddrs:         d.MacAddrs,
				SerialNumber:     d.SerialNumber,
				HDDSerialNumbers: d.HDDSerialNumbers,
				SSDSerialNumbers: d.SSDSerialNumbers,
				CPUSerialNumber:  d.CPUSerialNumber,
				MemSerialNumbers: d.MemSerialNumbers,
				GroupsID:         d.GroupID,
				GroupsName:       d.GroupName,
			}
			_ = r.feishuDevicesRepo.Upsert(item)
		}
		logger.Info("[可信设备] 分组设备已存入数据库",
			zap.String("分组ID", groupID),
			zap.Int("设备数", len(allDevices)),
		)
	}()

	return detail, nil
}

func (r *Runner) FetchDeviceTrustedStatus(did string) (string, error) {
	if r.client == nil {
		return "", fmt.Errorf("fetch device trusted status: sealsuite client is nil")
	}

	_, body, err := r.client.DoRaw(http.MethodGet, "/api/open/v1/device/detail", map[string]string{
		"did": did,
	}, nil)
	if err != nil {
		return "", fmt.Errorf("fetch device trusted status: %w", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode device detail: %w", err)
	}

	if data, ok := resp["data"]; ok {
		if dataMap, ok := data.(map[string]interface{}); ok {
			if v, ok := dataMap["trusted_status"]; ok && v != nil {
				return fmt.Sprintf("%v", v), nil
			}
		}
	}
	if v, ok := resp["trusted_status"]; ok && v != nil {
		return fmt.Sprintf("%v", v), nil
	}
	return "", nil
}

// getActiveFeishuAPI 获取当前激活的飞书自建应用配置
func (r *Runner) getActiveFeishuAPI() (*storage.FeishuAPIItem, error) {
	if r.feishuAPIRepo == nil {
		return nil, fmt.Errorf("feishu api repository is nil")
	}
	file, err := r.feishuAPIRepo.Load()
	if err != nil {
		return nil, fmt.Errorf("load feishu api config: %w", err)
	}
	if file == nil || file.ActiveID == "" {
		return nil, fmt.Errorf("no active feishu api config, please configure and activate in 设置 -> 飞书API")
	}
	for i := range file.Items {
		if file.Items[i].ID == file.ActiveID {
			return &file.Items[i], nil
		}
	}
	return nil, fmt.Errorf("active feishu api config not found: %s", file.ActiveID)
}

// getFeishuTenantAccessToken 获取飞书 tenant_access_token（带缓存）
func (r *Runner) getFeishuTenantAccessToken() (string, error) {
	r.feishuTokenMu.Lock()
	defer r.feishuTokenMu.Unlock()

	api, err := r.getActiveFeishuAPI()
	if err != nil {
		return "", err
	}
	if api.AppID == "" || api.AppSecret == "" {
		return "", fmt.Errorf("feishu app_id/app_secret is empty")
	}

	// 缓存未过期且属于同一个 App ID 则直接返回（提前 60 秒刷新）
	if r.feishuToken != "" && r.feishuTokenAppID == api.AppID &&
		time.Now().Before(r.feishuTokenExpiresAt.Add(-60*time.Second)) {
		return r.feishuToken, nil
	}

	baseURL := strings.TrimRight(api.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://open.feishu.cn"
	}
	tokenURL := baseURL + "/open-apis/auth/v3/tenant_access_token/internal"

	payload, _ := json.Marshal(map[string]string{
		"app_id":     api.AppID,
		"app_secret": api.AppSecret,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request tenant_access_token: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}

	var parsed struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parse token response: %w (raw: %s)", err, string(body))
	}
	if parsed.Code != 0 {
		return "", fmt.Errorf("get tenant_access_token failed: code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	if parsed.TenantAccessToken == "" {
		return "", fmt.Errorf("tenant_access_token is empty in response")
	}

	r.feishuToken = parsed.TenantAccessToken
	r.feishuTokenAppID = api.AppID
	expireSec := parsed.Expire
	if expireSec <= 0 {
		expireSec = 7200 // 默认 2 小时
	}
	r.feishuTokenExpiresAt = time.Now().Add(time.Duration(expireSec) * time.Second)

	logger.Info("[飞书] tenant_access_token 获取成功",
		zap.Int("有效期秒", expireSec),
	)
	return r.feishuToken, nil
}

// feishuDoRequest 向飞书开放平台发起带 Bearer 认证的请求
func (r *Runner) feishuDoRequest(method, url string, body interface{}) (int, []byte, error) {
	token, err := r.getFeishuTenantAccessToken()
	if err != nil {
		return 0, nil, err
	}

	var bodyReader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return 0, nil, fmt.Errorf("build feishu request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("feishu request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read feishu response: %w", err)
	}
	return resp.StatusCode, respBody, nil
}

// osToFeishuDeviceSystem 将飞连 os 字符串转换为飞书 device_system int
// 飞书：1=Windows 2=macOS 3=Linux 4=Android 5=iOS
func osToFeishuDeviceSystem(os string) int {
	switch strings.ToLower(strings.TrimSpace(os)) {
	case "windows", "win":
		return 1
	case "macos", "mac os", "mac", "darwin":
		return 2
	case "linux":
		return 3
	case "android":
		return 4
	case "ios", "iphone", "ipad":
		return 5
	default:
		return 0
	}
}

// trustedStatusToFeishuDeviceStatus 将飞连信任状态转换为飞书 device_status int
// 飞书：0=Unknown 1=Trusted 2=Untrusted
func trustedStatusToFeishuDeviceStatus(trustedStatus string) int {
	switch strings.TrimSpace(trustedStatus) {
	case "可信", "trusted", "Trusted":
		return 1
	case "不可信", "untrusted", "Untrusted":
		return 2
	default:
		return 0
	}
}

// pickDiskSerialNumber 优先取硬盘序列号（HDD 或 SSD），作为飞书 disk_serial_number
func pickDiskSerialNumber(device storage.FeishuDeviceItem) string {
	if v := strings.TrimSpace(device.HDDSerialNumbers); v != "" {
		return v
	}
	if v := strings.TrimSpace(device.SSDSerialNumbers); v != "" {
		return v
	}
	return ""
}

// pickMacAddress 取设备 MAC 地址，优先 mac_addr，其次从 mac_addrs 取第一个
func pickMacAddress(device storage.FeishuDeviceItem) string {
	if v := strings.TrimSpace(device.MacAddr); v != "" {
		return v
	}
	if v := strings.TrimSpace(device.MacAddrs); v != "" {
		// mac_addrs 可能是逗号/分号分隔的多个，取第一个
		for _, sep := range []string{",", ";", "|"} {
			if idx := strings.Index(v, sep); idx > 0 {
				return strings.TrimSpace(v[:idx])
			}
		}
		return v
	}
	return ""
}

// feishuValidCreateFields 飞书「新增设备」接口支持的字段白名单
var feishuValidCreateFields = map[string]bool{
	"device_system":      true,
	"serial_number":      true,
	"disk_serial_number": true,
	"uuid":               true,
	"mac_address":        true,
	"android_id":         true,
	"idfv":               true,
	"aaid":               true,
	"device_ownership":   true,
	"device_status":      true,
}

// buildFeishuCreatePayload 构建飞书「新增设备」请求体
func buildFeishuCreatePayload(device storage.FeishuDeviceItem, mappings []storage.FieldMappingItem) map[string]interface{} {
	payload := map[string]interface{}{
		"device_system":    osToFeishuDeviceSystem(device.OS),
		"device_ownership": 2, // 企业设备默认 Company
		"device_status":    trustedStatusToFeishuDeviceStatus(device.TrustedStatus),
	}

	// 基于字段映射填充可选设备标识符（仅允许飞书接口支持的字段）
	for _, m := range mappings {
		if !m.Enabled {
			continue
		}
		if !feishuValidCreateFields[m.TargetField] {
			continue
		}
		val := getDeviceFieldValue(device, m.SourceField)
		if val == "" {
			continue
		}
		payload[m.TargetField] = val
	}

	// 自动兜底：serial_number / disk_serial_number / mac_address
	if _, ok := payload["serial_number"]; !ok {
		if v := strings.TrimSpace(device.SerialNumber); v != "" {
			payload["serial_number"] = v
		}
	}
	if _, ok := payload["disk_serial_number"]; !ok {
		if v := pickDiskSerialNumber(device); v != "" {
			payload["disk_serial_number"] = v
		}
	}
	if _, ok := payload["mac_address"]; !ok {
		if v := pickMacAddress(device); v != "" {
			payload["mac_address"] = v
		}
	}

	return payload
}

// buildFeishuUpdatePayload 构建飞书「更新设备」请求体（仅支持 device_ownership 与 device_status）
func buildFeishuUpdatePayload(device storage.FeishuDeviceItem) map[string]interface{} {
	return map[string]interface{}{
		"device_ownership": 2,
		"device_status":    trustedStatusToFeishuDeviceStatus(device.TrustedStatus),
	}
}

func (r *Runner) ImportDevicesToFeishu() (int, error) {
	return r.ImportDevicesToFeishuWithFilters([]string{}, []string{}, []string{}, "AND")
}

// ImportDevicesToFeishuWithFilters 将筛选后的飞连设备导入/更新到飞书设备记录
// 已存在 feishu_device_record_id 的设备走 PUT 更新，否则走 POST 新增
func (r *Runner) ImportDevicesToFeishuWithFilters(osList, trustedStatusList, groupList []string, logicMode string) (int, error) {
	if r.feishuDevicesRepo == nil {
		return 0, fmt.Errorf("import devices: repository is nil")
	}

	api, err := r.getActiveFeishuAPI()
	if err != nil {
		return 0, err
	}
	baseURL := strings.TrimRight(api.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://open.feishu.cn"
	}
	createURL := baseURL + "/open-apis/security_and_compliance/v2/device_records"

	// 获取启用的字段映射（仅用于可选标识符字段）
	mappings, err := r.fieldMappingRepo.GetEnabledMappings()
	if err != nil {
		return 0, fmt.Errorf("import devices: get mappings: %w", err)
	}

	devices, err := r.feishuDevicesRepo.ListMulti(osList, trustedStatusList, groupList, logicMode)
	if err != nil {
		return 0, fmt.Errorf("import devices: list devices: %w", err)
	}

	if len(devices) == 0 {
		return 0, nil
	}

	successCount := 0
	for _, device := range devices {
		recordID := strings.TrimSpace(device.FeishuDeviceRecordID)

		if recordID != "" {
			// 已导入：调用更新接口（PUT）
			updateURL := createURL + "/" + recordID + "?version=0"
			payload := buildFeishuUpdatePayload(device)
			statusCode, body, err := r.feishuDoRequest(http.MethodPut, updateURL, payload)
			if err != nil {
				logger.Warn("[飞书导入] 更新设备失败",
					zap.String("did", device.DID),
					zap.String("device_record_id", recordID),
					zap.Error(err),
				)
				continue
			}
			var resp struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			_ = json.Unmarshal(body, &resp)
			if statusCode >= 200 && statusCode < 300 && resp.Code == 0 {
				successCount++
			} else {
				logger.Warn("[飞书导入] 更新设备返回非成功",
					zap.String("did", device.DID),
					zap.Int("http_status", statusCode),
					zap.Int("code", resp.Code),
					zap.String("msg", resp.Msg),
				)
			}
			continue
		}

		// 未导入：调用新增接口（POST）
		payload := buildFeishuCreatePayload(device, mappings)
		statusCode, body, err := r.feishuDoRequest(http.MethodPost, createURL, payload)
		if err != nil {
			logger.Warn("[飞书导入] 新增设备失败",
				zap.String("did", device.DID),
				zap.Error(err),
			)
			continue
		}
		var resp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				DeviceRecordID string `json:"device_record_id"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &resp)
		if statusCode >= 200 && statusCode < 300 && resp.Code == 0 {
			successCount++
			if resp.Data.DeviceRecordID != "" {
				_ = r.feishuDevicesRepo.UpdateFeishuDeviceRecordID(device.DID, resp.Data.DeviceRecordID)
			}
		} else if resp.Code == 1785010 {
			// 1785010: 设备已存在，无法通过 identifiers 自动关联，记录日志跳过
			logger.Warn("[飞书导入] 设备已存在，跳过",
				zap.String("did", device.DID),
				zap.String("msg", resp.Msg),
			)
		} else {
			logger.Warn("[飞书导入] 新增设备返回非成功",
				zap.String("did", device.DID),
				zap.Int("http_status", statusCode),
				zap.Int("code", resp.Code),
				zap.String("msg", resp.Msg),
			)
		}
	}

	logger.Info("[飞书导入] 导入完成",
		zap.Int("总数", len(devices)),
		zap.Int("成功", successCount),
	)
	return successCount, nil
}

func getDeviceFieldValue(device storage.FeishuDeviceItem, fieldName string) string {
	switch fieldName {
	case "did":
		return device.DID
	case "user_id":
		return device.UserID
	case "os":
		return device.OS
	case "client_ip":
		return device.ClientIP
	case "client_ip_location":
		return device.ClientIPLocation
	case "device_status":
		return device.DeviceStatus
	case "nic_type":
		return device.NICType
	case "serial_number":
		return strings.TrimSpace(device.SerialNumber)
	case "mac_addr":
		return pickMacAddress(device)
	case "device_type":
		return device.DeviceType
	case "trusted_status":
		return device.TrustedStatus
	case "groups_name":
		return device.GroupsName
	case "hdd_serial_numbers", "ssd_serial_numbers":
		return pickDiskSerialNumber(device)
	case "cpu_serial_number":
		return strings.TrimSpace(device.CPUSerialNumber)
	default:
		return ""
	}
}

func (r *Runner) defaultWriteFeilianCIDRsV2(task storage.ExternalIPSyncTask, cidrs []string) error {
	task = storage.NormalizeExternalIPSyncTask(task)

	if r.client == nil {
		logger.Error("[外部IP同步-写入] sealsuite client 未初始化",
			zap.String("task_id", task.ID),
		)
		return fmt.Errorf("write feilian cidrs v2: sealsuite client is nil")
	}
	if len(cidrs) == 0 {
		logger.Info("[外部IP同步-写入] CIDR列表为空，跳过写入",
			zap.String("task_id", task.ID),
		)
		return nil
	}

	createMode := strings.ToLower(strings.TrimSpace(task.CreateMode))
	logger.Info("[外部IP同步-写入] 准备调用飞连API",
		zap.String("task_id", task.ID),
		zap.String("create_mode", createMode),
		zap.Int("cidr_count", len(cidrs)),
	)

	if createMode == storage.FeishuResourceCreateModeAdd {
		newResourceName := strings.TrimSpace(task.NewResourceName)
		if newResourceName == "" {
			logger.Error("[外部IP同步-写入] 新增模式下new_resource_name为空",
				zap.String("task_id", task.ID),
			)
			return fmt.Errorf("write feilian cidrs v2: new_resource_name is required for add mode")
		}

		logger.Info("[外部IP同步-写入] 调用新增资源接口",
			zap.String("task_id", task.ID),
			zap.String("api_path", "/api/open/v1/addr/management/add"),
			zap.String("new_resource_name", newResourceName),
			zap.Int("cidr_count", len(cidrs)),
		)

		_, body, err := r.client.DoRaw(http.MethodPost, "/api/open/v1/addr/management/add", nil, map[string]interface{}{
			"name": newResourceName,
			"type": "ip",
			"ips":  cidrs,
		})
		if err != nil {
			logger.Error("[外部IP同步-写入] 调用新增资源接口失败",
				zap.String("task_id", task.ID),
				zap.String("api_path", "/api/open/v1/addr/management/add"),
				zap.Error(err),
			)
			return fmt.Errorf("write feilian cidrs v2: create resource: %w", err)
		}

		var resp map[string]interface{}
		if jsonErr := json.Unmarshal(body, &resp); jsonErr != nil {
			logger.Warn("[外部IP同步-写入] 解析响应JSON失败",
				zap.String("task_id", task.ID),
				zap.Error(jsonErr),
			)
			return nil
		}

		var newResourceID string
		if data, ok := resp["data"]; ok {
			if dataMap, ok := data.(map[string]interface{}); ok {
				if v, ok := dataMap["resource_id"]; ok && v != nil {
					newResourceID = fmt.Sprintf("%v", v)
				}
				if v, ok := dataMap["id"]; ok && v != nil {
					newResourceID = fmt.Sprintf("%v", v)
				}
			}
		}
		if v, ok := resp["resource_id"]; ok && v != nil {
			newResourceID = fmt.Sprintf("%v", v)
		}

		if newResourceID != "" {
			logger.Info("[外部IP同步-写入] 新增资源成功，获取到resource_id",
				zap.String("task_id", task.ID),
				zap.String("new_resource_id", newResourceID),
			)

			if r.externalIPSyncRepo != nil {
				existing, ok, getErr := r.externalIPSyncRepo.Get(task.ID)
				if getErr != nil {
					logger.Error("[外部IP同步-写入] 查询任务失败",
						zap.String("task_id", task.ID),
						zap.Error(getErr),
					)
				} else if !ok {
					logger.Warn("[外部IP同步-写入] 任务不存在",
						zap.String("task_id", task.ID),
					)
				} else {
					existing.ResourceID = newResourceID
					existing.CreateMode = storage.FeishuResourceCreateModeSelect
					if upsertErr := r.externalIPSyncRepo.Upsert(existing); upsertErr != nil {
						logger.Error("[外部IP同步-写入] 更新任务失败",
							zap.String("task_id", task.ID),
							zap.Error(upsertErr),
						)
					} else {
						logger.Info("[外部IP同步-写入] 任务已更新为选择模式",
							zap.String("task_id", task.ID),
							zap.String("resource_id", newResourceID),
						)
					}
				}
			}
		} else {
			logger.Warn("[外部IP同步-写入] 新增资源成功但未获取到resource_id",
				zap.String("task_id", task.ID),
			)
		}
		return nil
	}

	if task.ResourceID == "" {
		logger.Error("[外部IP同步-写入] 选择模式下resource_id为空",
			zap.String("task_id", task.ID),
		)
		return fmt.Errorf("write feilian cidrs v2: resource_id is required for select mode")
	}

	apiPath := "/api/open/v1/addr/management/update"
	if strings.TrimSpace(task.FeilianAPIPath) != "" && strings.TrimSpace(task.FeilianAPIPath) != "/api/open/v1/addr/management/add" {
		apiPath = task.FeilianAPIPath
	}

	logger.Info("[外部IP同步-写入] 调用更新资源接口",
		zap.String("task_id", task.ID),
		zap.String("api_path", apiPath),
		zap.String("resource_id", task.ResourceID),
		zap.Int("cidr_count", len(cidrs)),
	)

	var resourceID interface{} = task.ResourceID
	if idInt, err := strconv.Atoi(task.ResourceID); err == nil {
		resourceID = idInt
	}

	_, err := r.client.Post(apiPath, map[string]interface{}{
		"id":   resourceID,
		"type": "ip",
		"ips":  cidrs,
	})
	if err != nil {
		logger.Error("[外部IP同步-写入] 调用更新资源接口失败",
			zap.String("task_id", task.ID),
			zap.String("api_path", apiPath),
			zap.String("resource_id", task.ResourceID),
			zap.Error(err),
		)
	} else {
		logger.Info("[外部IP同步-写入] 更新资源成功",
			zap.String("task_id", task.ID),
			zap.String("resource_id", task.ResourceID),
			zap.Int("cidr_count", len(cidrs)),
		)
	}
	return err
}

func filterGoogleCIDRs(in googleIPRanges, version string) []string {
	version = strings.ToLower(strings.TrimSpace(version))
	set := make(map[string]struct{}, len(in.Prefixes))
	for _, prefix := range in.Prefixes {
		if (version == "ipv4" || version == "all") && strings.TrimSpace(prefix.IPv4Prefix) != "" {
			set[strings.TrimSpace(prefix.IPv4Prefix)] = struct{}{}
		}
		if (version == "ipv6" || version == "all") && strings.TrimSpace(prefix.IPv6Prefix) != "" {
			set[strings.TrimSpace(prefix.IPv6Prefix)] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for cidr := range set {
		out = append(out, cidr)
	}
	sort.Strings(out)
	return out
}

func diffCIDRs(source, existing []string) []string {
	exists := make(map[string]struct{}, len(existing))
	for _, cidr := range existing {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		exists[cidr] = struct{}{}
	}

	out := make([]string, 0, len(source))
	added := make(map[string]struct{}, len(source))
	for _, cidr := range source {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		if _, ok := exists[cidr]; ok {
			continue
		}
		if _, ok := added[cidr]; ok {
			continue
		}
		out = append(out, cidr)
		added[cidr] = struct{}{}
	}
	return out
}

func (r *Runner) deliverTaskDraftWebhook(d storage.TaskDraft, result interface{}) webhook.DeliveryResult {
	webhookID := strings.TrimSpace(d.WebhookConfigID)
	delivery := webhook.DeliveryResult{Attempted: false}
	switch {
	case !d.WebhookEnabled || webhookID == "":
	case r.webhookRepo == nil:
		delivery = webhook.DeliveryResult{Attempted: false, Error: "webhook repository is nil"}
	default:
		item, ok, err := r.loadWebhookItem(webhookID)
		switch {
		case err != nil:
			delivery = webhook.DeliveryResult{Attempted: false, Error: err.Error()}
		case !ok:
			delivery = webhook.DeliveryResult{Attempted: false, Error: fmt.Sprintf("webhook config not found: %s", webhookID)}
		default:
			delivery = webhook.DeliverWebhookForSuccess(context.Background(), item, webhook.Envelope{
				Event:      "task.completed",
				SourceType: "api_task",
				SourceID:   d.ID,
				SourceName: firstNonEmpty(d.Name, d.ID, "飞连API任务"),
				Timestamp:  time.Now().Format(time.RFC3339),
				Data:       result,
			})
		}
	}
	r.logWebhookDelivery("api_task", d.ID, delivery)
	return delivery
}

func (r *Runner) deliverScheduleWebhook(ctx context.Context, s storage.JobSchedule, targetType, targetID string, result interface{}) webhook.DeliveryResult {
	webhookID := strings.TrimSpace(s.WebhookConfigID)
	if ctx == nil {
		ctx = context.Background()
	}
	delivery := webhook.DeliveryResult{Attempted: false}
	switch {
	case !s.WebhookEnabled || webhookID == "":
	case r.webhookRepo == nil:
		delivery = webhook.DeliveryResult{Attempted: false, Error: "webhook repository is nil"}
	default:
		item, ok, err := r.loadWebhookItem(webhookID)
		switch {
		case err != nil:
			delivery = webhook.DeliveryResult{Attempted: false, Error: err.Error()}
		case !ok:
			delivery = webhook.DeliveryResult{Attempted: false, Error: fmt.Sprintf("webhook config not found: %s", webhookID)}
		default:
			delivery = webhook.DeliverWebhookForSuccess(ctx, item, webhook.Envelope{
				Event:      "task.completed",
				SourceType: "job_schedule",
				SourceID:   s.ID,
				SourceName: firstNonEmpty(s.ID, "定时任务"),
				Timestamp:  time.Now().Format(time.RFC3339),
				Data: map[string]interface{}{
					"schedule_id": s.ID,
					"target_type": targetType,
					"target_id":   targetID,
					"result":      result,
				},
			})
		}
	}
	r.logWebhookDelivery("job_schedule", s.ID, delivery)
	return delivery
}

func (r *Runner) logWebhookDelivery(sourceType, sourceID string, delivery webhook.DeliveryResult) {
	logger.Info("webhook delivery",
		zap.String("source_type", sourceType),
		zap.String("source_id", sourceID),
		zap.Bool("attempted", delivery.Attempted),
		zap.Bool("ok", delivery.OK),
		zap.Int("status_code", delivery.StatusCode),
		zap.String("error", delivery.Error),
	)
}

func (r *Runner) recordLegacyJobRun(j storage.Job, triggerSource string, startedAt, finishedAt time.Time, runErr error) {
	if r.executionSvc == nil {
		return
	}
	if err := r.executionSvc.RecordRun(service.RunLog{
		SourceType:    "job",
		SourceID:      j.Name,
		TargetType:    "legacy_job",
		TargetID:      j.Name,
		Status:        statusFromRunError(runErr),
		TriggerSource: firstNonEmpty(triggerSource, "scheduler"),
		StartedAt:     startedAt.Format(time.RFC3339),
		FinishedAt:    finishedAt.Format(time.RFC3339),
		DurationMS:    finishedAt.Sub(startedAt).Milliseconds(),
		ErrorMessage:  errorString(runErr),
		ResultJSON:    "null",
	}); err != nil {
		logger.Warn("failed to persist legacy job run", zap.String("job", j.Name), zap.Error(err))
	}
}

func (r *Runner) recordTaskDraftRun(d storage.TaskDraft, triggerSource string, startedAt, finishedAt time.Time, result interface{}, runErr error) {
	if r.executionSvc == nil {
		return
	}
	if err := r.executionSvc.RecordTaskDraftRun(d.ID, triggerSource, startedAt, finishedAt, result, runErr); err != nil {
		logger.Warn("failed to persist task draft run", zap.String("draft_id", d.ID), zap.Error(err))
	}
}

func (r *Runner) recordScheduleRun(s storage.JobSchedule, targetType, targetID, triggerSource string, startedAt, finishedAt time.Time, result interface{}, runErr error) {
	if r.executionSvc == nil {
		return
	}
	if err := r.executionSvc.RecordScheduleRun(s.ID, targetType, targetID, triggerSource, startedAt, finishedAt, result, runErr); err != nil {
		logger.Warn("failed to persist schedule run", zap.String("schedule_id", s.ID), zap.Error(err))
	}
}

func (r *Runner) recordComplexTaskRun(task storage.ComplexTask, triggerSource string, startedAt, finishedAt time.Time, result interface{}, runErr error) {
	if r.executionSvc == nil {
		return
	}
	if err := r.executionSvc.RecordComplexTaskRun(task.ID, triggerSource, startedAt, finishedAt, result, runErr); err != nil {
		logger.Warn("failed to persist complex task run", zap.String("complex_task_id", task.ID), zap.Error(err))
	}
}

func statusFromRunError(err error) string {
	if err != nil {
		return "failed"
	}
	return "success"
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (r *Runner) latestRunLogStatus() (map[string]service.RunLog, error) {
	if r.runLogsRepo == nil {
		return map[string]service.RunLog{}, nil
	}
	runs, err := r.runLogsRepo.List(0)
	if err != nil {
		return nil, err
	}
	out := make(map[string]service.RunLog, len(runs))
	for _, run := range runs {
		key := latestRunStatusKey(run)
		if key == "" {
			continue
		}
		if _, exists := out[key]; exists {
			continue
		}
		out[key] = run
	}
	return out, nil
}

func latestRunStatusKey(run service.RunLog) string {
	switch strings.TrimSpace(run.SourceType) {
	case "job":
		return strings.TrimSpace(run.SourceID)
	case "job_schedule":
		return scheduleRunKeyPrefix + strings.TrimSpace(run.SourceID)
	default:
		return ""
	}
}

func applyJobStatusRecord(target *JobStatus, run service.RunLog) {
	if target == nil {
		return
	}
	lastRun := firstNonEmpty(strings.TrimSpace(run.FinishedAt), strings.TrimSpace(run.StartedAt))
	if lastRun != "" {
		if ts, err := time.Parse(time.RFC3339, lastRun); err == nil {
			target.LastRun = ts
		}
	}
	target.LastOK = run.Status == "success"
	target.DurationMs = run.DurationMS
	target.LastError = run.ErrorMessage
}

func (r *Runner) executeComplexTaskStep(step storage.ComplexTaskStep, current interface{}) (interface{}, error) {
	switch step.Type {
	case "api_call":
		targetType, targetID := NormalizeTarget(asString(step.Config["target_type"]), asString(step.Config["target_id"]), asString(step.Config["draft_id"]))
		if targetType != "" && targetID != "" {
			return r.executeTargetOutput(targetType, targetID)
		}
		req := buildExecuteRequestFromMap(step.Config)
		if req.TemplateID == "" && req.Method == "" && req.Path == "" {
			return nil, fmt.Errorf("api_call requires template_id or method/path")
		}
		resp, err := r.executor.Execute(req)
		if err != nil {
			return nil, err
		}
		body := decodeJSONOrRaw(resp.Body)
		out := map[string]interface{}{
			"http_status": resp.HTTPStatus,
			"latency_ms":  resp.LatencyMs,
			"body":        body,
		}
		if resp.BusinessCode != nil {
			out["business_code"] = *resp.BusinessCode
		}
		if resp.BusinessMessage != "" {
			out["business_message"] = resp.BusinessMessage
		}
		return out, nil

	case "data_transform":
		cfg := step.Config
		if cfg == nil {
			cfg = map[string]interface{}{}
		}
		transformType := firstNonEmpty(asString(cfg["type"]), "passthrough")
		switch transformType {
		case "passthrough", "json_map":
			return current, nil
		case "wrap":
			key := firstNonEmpty(asString(cfg["key"]), "result")
			return map[string]interface{}{key: current}, nil
		default:
			return map[string]interface{}{
				"type":  transformType,
				"note":  "当前未实现该 transform，已返回原始输入",
				"input": current,
			}, nil
		}

	case "llm_inference":
		cfg := step.Config
		if cfg == nil {
			cfg = map[string]interface{}{}
		}
		role := firstNonEmpty(asString(cfg["role"]), "formatter")
		llmCfg, hasSelectedLLMAPI, err := r.resolveSelectedLLMConfig(cfg)
		if err != nil {
			return nil, err
		}
		if !hasSelectedLLMAPI {
			llmCfg = r.resolveLLMRoleConfig(role, cfg)
		}
		prompt := buildLLMPrompt(cfg)
		if !llmCfg.Enabled || llmCfg.BaseURL == "" || llmCfg.APIKey == "" || llmCfg.Model == "" {
			previewContent := strings.TrimSpace("当前未真正调用大模型，因为 LLM 配置不完整。\n\n请检查连接配置中的 LLM_API 配置是否已启用、保存并热更新。\n\nPrompt:\n" + prompt + "\n\n输入预览:\n" + truncateString(prettyJSON(current), 1200))
			result := map[string]interface{}{
				"provider":      firstNonEmpty(llmCfg.Provider, "preview"),
				"model":         llmCfg.Model,
				"role":          role,
				"prompt":        prompt,
				"note":          "LLM 未完整配置，已返回预览结果",
				"input_preview": truncateString(prettyJSON(current), 1200),
				"content":       previewContent,
			}
			if id := strings.TrimSpace(asString(cfg["llm_api_id"])); id != "" {
				result["llm_api_id"] = id
			}
			return result, nil
		}
		client := llm.NewClient(llmCfg)
		resp, err := client.Chat(llm.ChatRequest{
			Model:           llmCfg.Model,
			SystemPrompt:    llmCfg.SystemPrompt,
			Prompt:          prompt,
			Input:           current,
			Temperature:     floatPtrIfPositive(llmCfg.Temperature),
			MaxTokens:       intPtrIfPositive(llmCfg.MaxTokens),
			Thinking:        boolPtrIfTrue(llmCfg.Thinking),
			ReasoningEffort: llmCfg.ReasoningEffort,
			ResponseFormat:  llmCfg.ResponseFormat,
		})
		if err != nil {
			return nil, err
		}
		result := map[string]interface{}{
			"role":     role,
			"provider": llmCfg.Provider,
			"model":    resp.Model,
			"prompt":   prompt,
			"content":  resp.Content,
			"raw":      resp.Raw,
		}
		if id := strings.TrimSpace(asString(cfg["llm_api_id"])); id != "" {
			result["llm_api_id"] = id
		}
		return result, nil

	case "output":
		cfg := step.Config
		if cfg == nil {
			cfg = map[string]interface{}{}
		}
		format := firstNonEmpty(asString(cfg["format"]), "json")
		title := firstNonEmpty(asString(cfg["title"]), step.Name, "复杂任务输出")
		contentTemplate := asString(cfg["content_template"])
		if contentTemplate != "" {
			return applyOutputContentTemplate(contentTemplate, current), nil
		}
		switch format {
		case "markdown":
			return renderMarkdownOutput(title, current), nil
		case "text":
			if content, ok := extractPreferredOutputContent(current); ok {
				return content, nil
			}
			return truncateString(prettyJSON(current), 4000), nil
		default:
			return current, nil
		}

	default:
		return nil, fmt.Errorf("unsupported complex task step type: %s", step.Type)
	}
}

func (r *Runner) refreshNextRun() {
	if r.sched == nil {
		return
	}
	r.statusMu.Lock()
	defer r.statusMu.Unlock()
	for name := range r.status {
		r.refreshNextRunWith(name, r.status, name)
	}
	for id := range r.scheduleStatus {
		r.refreshNextRunWith(r.scheduleEntryName(id), r.scheduleStatus, id)
	}
}

func (r *Runner) refreshNextRunFor(name string) {
	r.statusMu.Lock()
	defer r.statusMu.Unlock()
	r.refreshNextRunWith(name, r.status, name)
}

func (r *Runner) refreshNextRunForSchedule(id string) {
	r.statusMu.Lock()
	defer r.statusMu.Unlock()
	r.refreshNextRunWith(r.scheduleEntryName(id), r.scheduleStatus, id)
}

func (r *Runner) refreshNextRunWith(entryName string, target map[string]*JobStatus, statusKey string) {
	if r.sched == nil {
		return
	}
	id, ok := r.sched.EntryID(entryName)
	if !ok {
		return
	}
	for _, e := range r.sched.Entries() {
		if e.ID == id {
			st := r.ensureStatus(target, statusKey)
			if !e.Next.IsZero() {
				t := e.Next
				st.NextRun = &t
			}
			return
		}
	}
}

func (r *Runner) ensureStatus(target map[string]*JobStatus, key string) *JobStatus {
	st := target[key]
	if st == nil {
		st = &JobStatus{}
		target[key] = st
	}
	return st
}

func (r *Runner) scheduleEntryName(id string) string {
	return scheduleRunKeyPrefix + id
}

func (r *Runner) scheduleSpec(s storage.JobSchedule) (string, error) {
	switch s.ScheduleType {
	case "cron":
		if s.Cron == "" {
			return "", fmt.Errorf("cron schedule requires cron")
		}
		return s.Cron, nil
	case "interval":
		if s.Interval == "" {
			return "", fmt.Errorf("interval schedule requires interval")
		}
		if _, err := time.ParseDuration(s.Interval); err != nil {
			return "", fmt.Errorf("invalid interval %q: %w", s.Interval, err)
		}
		return "@every " + s.Interval, nil
	default:
		return "", fmt.Errorf("unsupported schedule type: %s", s.ScheduleType)
	}
}

func (r *Runner) scheduleWindowAllows(s storage.JobSchedule, now time.Time) (bool, error) {
	if s.StartAt != "" {
		startAt, err := time.Parse(time.RFC3339, s.StartAt)
		if err != nil {
			return false, fmt.Errorf("invalid start_at for schedule %s: %w", s.ID, err)
		}
		if now.Before(startAt) {
			return false, nil
		}
	}
	if s.EndAt != "" {
		endAt, err := time.Parse(time.RFC3339, s.EndAt)
		if err != nil {
			return false, fmt.Errorf("invalid end_at for schedule %s: %w", s.ID, err)
		}
		if now.After(endAt) {
			return false, nil
		}
	}
	return true, nil
}

func coerceStringMap(m map[string]interface{}) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		switch vv := v.(type) {
		case string:
			out[k] = vv
		}
	}
	return out
}

// stringMapToIFMap 将 map[string]string 转为 map[string]interface{} 用于
// api.ExecuteRequest 的 Query/PathParams 字段（该字段现在是 map[string]interface{}）。
func stringMapToIFMap(m map[string]string) map[string]interface{} {
	if m == nil {
		return nil
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func mapValue(input map[string]interface{}, key string) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	if v, ok := input[key].(map[string]interface{}); ok && v != nil {
		return v
	}
	return map[string]interface{}{}
}

func buildExecuteRequestFromMap(input map[string]interface{}) api.ExecuteRequest {
	if input == nil {
		input = map[string]interface{}{}
	}
	req := api.ExecuteRequest{
		TemplateID: firstNonEmpty(asString(input["template_id"])),
		Method:     asString(input["method"]),
		Path:       asString(input["path"]),
		Query:      stringMapToIFMap(coerceStringMap(mapValue(input, "query"))),
		PathParams: stringMapToIFMap(coerceStringMap(mapValue(input, "path_params"))),
		Body:       extractBody(mapValue(input, "body")),
	}
	if len(req.Body) == 0 {
		if bodyMap := mapValue(input, "body"); len(bodyMap) > 0 {
			req.Body = bodyMap
		}
	}
	return req
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func asFloat(v interface{}) (float64, bool) {
	switch vv := v.(type) {
	case float64:
		return vv, true
	case float32:
		return float64(vv), true
	case int:
		return float64(vv), true
	case int64:
		return float64(vv), true
	default:
		return 0, false
	}
}

func asInt(v interface{}) (int, bool) {
	switch vv := v.(type) {
	case int:
		return vv, true
	case int64:
		return int(vv), true
	case float64:
		return int(vv), true
	case float32:
		return int(vv), true
	default:
		return 0, false
	}
}

func asBool(v interface{}) (bool, bool) {
	b, ok := v.(bool)
	return b, ok
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// extractBody 支持两种写法：
// 1) input.__raw_json__ = { ... }（推荐）
// 2) input.body = { ... }
func extractBody(input map[string]interface{}) map[string]interface{} {
	if v, ok := input["__raw_json__"].(map[string]interface{}); ok {
		return map[string]interface{}{"__raw_json__": v}
	}
	if v, ok := input["body"].(map[string]interface{}); ok {
		return v
	}
	return nil
}

func decodeJSONOrRaw(raw []byte) interface{} {
	if len(raw) == 0 {
		return nil
	}
	var out interface{}
	if err := json.Unmarshal(raw, &out); err == nil {
		return out
	}
	return string(raw)
}

func prettyJSON(v interface{}) string {
	if v == nil {
		return "null"
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func truncateString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n...(truncated)"
}

func renderMarkdownOutput(title string, current interface{}) string {
	if content, ok := extractPreferredOutputContent(current); ok {
		return "# " + title + "\n\n" + strings.TrimSpace(content)
	}
	return "# " + title + "\n\n```json\n" + prettyJSON(current) + "\n```"
}

func applyOutputContentTemplate(tpl string, current interface{}) string {
	out := strings.ReplaceAll(tpl, "{{result}}", prettyJSON(current))
	out = strings.ReplaceAll(out, "{{json}}", prettyJSON(current))
	if content, ok := extractPreferredOutputContent(current); ok {
		out = strings.ReplaceAll(out, "{{content}}", content)
	}
	return out
}

func extractPreferredOutputContent(current interface{}) (string, bool) {
	switch v := current.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			return v, true
		}
	case map[string]interface{}:
		if content := strings.TrimSpace(asString(v["content"])); content != "" {
			return content, true
		}
		if note := strings.TrimSpace(asString(v["note"])); note != "" {
			parts := []string{note}
			if prompt := strings.TrimSpace(asString(v["prompt"])); prompt != "" {
				parts = append(parts, "Prompt:\n"+prompt)
			}
			if preview := strings.TrimSpace(asString(v["input_preview"])); preview != "" {
				parts = append(parts, "输入预览:\n"+preview)
			}
			return strings.Join(parts, "\n\n"), true
		}
	}
	return "", false
}

func buildLLMPrompt(cfg map[string]interface{}) string {
	parts := make([]string, 0, 4)
	for _, key := range []string{"skill_text", "soul_text", "prompt_text", "prompt"} {
		if v := strings.TrimSpace(asString(cfg[key])); v != "" {
			parts = append(parts, v)
		}
	}
	for _, key := range []string{"skill_file", "soul_file"} {
		if path := strings.TrimSpace(asString(cfg[key])); path != "" {
			if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
				parts = append(parts, string(b))
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func (r *Runner) resolveLLMRoleConfig(role string, overrides map[string]interface{}) config.LLMRoleConfig {
	var llmCfg config.LLMRoleConfig
	switch role {
	case "planner":
		llmCfg = r.cfg.LLM.Planner
	default:
		llmCfg = r.cfg.LLM.Formatter
	}
	return applyLLMRoleOverrides(llmCfg, overrides)
}

func (r *Runner) resolveSelectedLLMConfig(overrides map[string]interface{}) (config.LLMRoleConfig, bool, error) {
	id := strings.TrimSpace(asString(overrides["llm_api_id"]))
	if id == "" {
		return config.LLMRoleConfig{}, false, nil
	}
	selectedCfg, err := r.resolveLLMConfigFromAPIID(id)
	if err != nil {
		return config.LLMRoleConfig{}, true, storage.NewValidationError(fmt.Sprintf("指定的 LLM_API 不存在或已删除: %s", id))
	}
	if !selectedCfg.Enabled {
		return config.LLMRoleConfig{}, true, storage.NewValidationError(fmt.Sprintf("指定的 LLM_API 已禁用: %s", id))
	}
	return applyLLMRoleOverrides(selectedCfg, overrides), true, nil
}

func (r *Runner) resolveLLMConfigFromAPIID(id string) (config.LLMRoleConfig, error) {
	if r.llmAPIRepo == nil {
		return config.LLMRoleConfig{}, fmt.Errorf("llm api repository is nil")
	}
	item, ok, err := r.llmAPIRepo.Get(id)
	if err != nil {
		logger.Warn("failed to load llm api from sqlite", zap.String("llm_api_id", id), zap.Error(err))
		return config.LLMRoleConfig{}, err
	}
	if !ok {
		return config.LLMRoleConfig{}, fmt.Errorf("llm api not found: %s", strings.TrimSpace(id))
	}
	return r.mapLLMAPIItemToRoleConfig(*item), nil
}

func (r *Runner) mapLLMAPIItemToRoleConfig(item storage.LLMAPIItem) config.LLMRoleConfig {
	return config.LLMRoleConfig{
		Enabled:         item.Enabled,
		Provider:        item.Provider,
		BaseURL:         item.BaseURL,
		APIKey:          item.APIKey,
		Model:           item.Model,
		Timeout:         item.Timeout,
		MockMode:        r.cfg.LLM.MockMode,
		SystemPrompt:    item.SystemPrompt,
		Temperature:     item.Temperature,
		MaxTokens:       item.MaxTokens,
		Thinking:        item.Thinking,
		ReasoningEffort: item.ReasoningEffort,
		ResponseFormat:  cloneMapAny(item.ResponseFormat),
	}
}

func (r *Runner) loadWebhookItem(id string) (*storage.WebhookItem, bool, error) {
	id = strings.TrimSpace(id)
	if r.webhookRepo == nil {
		return nil, false, fmt.Errorf("webhook repository is nil")
	}
	item, ok, err := r.webhookRepo.Get(id)
	if err != nil {
		logger.Warn("failed to load webhook from sqlite", zap.String("webhook_id", id), zap.Error(err))
		return nil, false, err
	}
	return item, ok, nil
}

func applyLLMRoleOverrides(llmCfg config.LLMRoleConfig, overrides map[string]interface{}) config.LLMRoleConfig {
	if overrides == nil {
		return llmCfg
	}
	if v := asString(overrides["provider"]); v != "" {
		llmCfg.Provider = v
	}
	if v := asString(overrides["base_url"]); v != "" {
		llmCfg.BaseURL = v
	}
	if v := asString(overrides["api_key"]); v != "" {
		llmCfg.APIKey = v
	}
	if v := asString(overrides["model"]); v != "" {
		llmCfg.Model = v
	}
	if v := asString(overrides["system_prompt"]); v != "" {
		llmCfg.SystemPrompt = v
	}
	if v, ok := asFloat(overrides["temperature"]); ok {
		llmCfg.Temperature = v
	}
	if v, ok := asInt(overrides["max_tokens"]); ok {
		llmCfg.MaxTokens = v
	}
	if v, ok := asBool(overrides["enabled"]); ok {
		llmCfg.Enabled = v
	}
	if v, ok := asBool(overrides["thinking"]); ok {
		llmCfg.Thinking = v
	}
	if v := asString(overrides["reasoning_effort"]); v != "" {
		llmCfg.ReasoningEffort = v
	}
	if v, ok := overrides["response_format"].(map[string]interface{}); ok && len(v) > 0 {
		llmCfg.ResponseFormat = cloneMapAny(v)
	}
	return llmCfg
}

func cloneMapAny(in map[string]interface{}) map[string]interface{} {
	if len(in) == 0 {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		if child, ok := v.(map[string]interface{}); ok {
			out[k] = cloneMapAny(child)
			continue
		}
		out[k] = v
	}
	return out
}

func (r *Runner) executeTargetOutput(targetType, targetID string) (interface{}, error) {
	switch targetType {
	case "task_draft":
		draft, err := r.loadTaskDraft(targetID)
		if err != nil {
			return nil, err
		}
		out, _, err := r.runTaskDraft(draft)
		return out, err
	case "complex_task":
		task, err := r.loadComplexTask(targetID)
		if err != nil {
			return nil, err
		}
		result, err := r.runComplexTask(task, "internal")
		if err != nil {
			return nil, err
		}
		return result.FinalOutput, nil
	case "scheduled_task", "job_schedule":
		normalized, err := r.loadSchedule(targetID)
		if err != nil {
			return nil, err
		}
		nextType, nextID := NormalizeTarget(normalized.TargetType, normalized.TargetID, normalized.DraftID)
		return r.executeTargetOutput(nextType, nextID)
	case "external_ip_sync":
		if r.executeExternalIPSync == nil {
			r.executeExternalIPSync = r.runExternalIPSyncTask
		}
		return r.executeExternalIPSync(targetID)
	default:
		return nil, fmt.Errorf("unsupported target_type: %s", targetType)
	}
}

func extractCIDRsFromPayload(payload map[string]interface{}) []string {
	set := map[string]struct{}{}
	collectCIDRs(payload, set)
	out := make([]string, 0, len(set))
	for cidr := range set {
		out = append(out, cidr)
	}
	sort.Strings(out)
	return out
}

func collectCIDRs(value interface{}, set map[string]struct{}) {
	switch vv := value.(type) {
	case map[string]interface{}:
		for key, child := range vv {
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "cidr", "ipv4prefix", "ipv6prefix":
				appendCIDRIfValid(set, asString(child))
			case "cidrs", "ips", "ip_list", "items", "data", "list":
				collectCIDRs(child, set)
			default:
				collectCIDRs(child, set)
			}
		}
	case []interface{}:
		for _, child := range vv {
			collectCIDRs(child, set)
		}
	case string:
		appendCIDRIfValid(set, vv)
	}
}

func appendCIDRIfValid(set map[string]struct{}, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if _, _, err := net.ParseCIDR(value); err != nil {
		return
	}
	set[value] = struct{}{}
}

func (r *Runner) executeTargetOutputForSchedule(targetType, targetID string) (interface{}, error) {
	switch targetType {
	case "task_draft":
		draft, err := r.loadTaskDraft(targetID)
		if err != nil {
			return nil, err
		}
		out, _, err := r.runTaskDraftWithDelivery(draft, false)
		return out, err
	case "scheduled_task", "job_schedule":
		normalized, err := r.loadSchedule(targetID)
		if err != nil {
			return nil, err
		}
		nextType, nextID := NormalizeTarget(normalized.TargetType, normalized.TargetID, normalized.DraftID)
		return r.executeTargetOutputForSchedule(nextType, nextID)
	default:
		return r.executeTargetOutput(targetType, targetID)
	}
}

func NormalizeTarget(targetType, targetID, draftID string) (string, string) {
	if targetType == "" && draftID != "" {
		return "task_draft", draftID
	}
	return targetType, targetID
}

func floatPtrIfPositive(v float64) *float64 {
	if v <= 0 {
		return nil
	}
	return &v
}

func intPtrIfPositive(v int) *int {
	if v <= 0 {
		return nil
	}
	return &v
}

func boolPtrIfTrue(v bool) *bool {
	if !v {
		return nil
	}
	return &v
}

// 用于让 go compiler 保留 cron import（当前未直接使用，但未来用于更精细 next_run）
var _ cron.EntryID

// ===================== DLP 文件误报分析 =====================

type DLPSyncResult struct {
	SyncedCount int    `json:"synced_count"`
	SyncAt      string `json:"sync_at"`
}

type DLPAnalyzeResult struct {
	AnalyzedCount int    `json:"analyzed_count"`
	ExcludedCount int    `json:"excluded_count"`
	KeptCount     int    `json:"kept_count"`
	AnalyzedAt    string `json:"analyzed_at"`
}

func (r *Runner) SyncDLPEvents(maxItems int) (*DLPSyncResult, error) {
	if r.dlpEventRepo == nil {
		return nil, fmt.Errorf("dlp event repository is nil")
	}

	now := time.Now()

	var startTime time.Time
	if maxItems > 0 {
		startTime = now.Add(-7 * 24 * time.Hour)
	} else {
		startTime = now.Add(-24 * time.Hour)
	}

	startTimeStr := startTime.Format(time.RFC3339)

	if maxItems <= 0 {
		maxItems = 0
	}

	items, err := r.fetchDLPEventsFromAPI(startTimeStr, maxItems)
	if err != nil {
		logger.Error("[DLP] 获取DLP事件失败", zap.Error(err))
		return nil, err
	}

	synced := 0
	for _, item := range items {
		if err := r.dlpEventRepo.Upsert(item); err != nil {
			logger.Warn("[DLP] 保存DLP事件失败", zap.String("id", item.ID), zap.Error(err))
			continue
		}
		synced++
	}

	syncAt := now.Format(time.RFC3339)
	if upErr := r.dlpEventRepo.UpdateSyncState(syncAt, synced); upErr != nil {
		logger.Warn("[DLP] 更新同步状态失败", zap.Error(upErr))
	}

	return &DLPSyncResult{
		SyncedCount: synced,
		SyncAt:      syncAt,
	}, nil
}

func (r *Runner) fetchDLPEventsFromAPI(startTime string, maxItems int) ([]storage.DLPEvent, error) {
	results := make([]storage.DLPEvent, 0)
	pageSize := 50
	seenIDs := make(map[string]bool)

	currentEndTime := time.Now().Format(time.RFC3339)
	startTimeUnix := toUnixTime(startTime)

	for {
		endTimeUnix := toUnixTime(currentEndTime)

		if endTimeUnix <= startTimeUnix {
			break
		}

		query := map[string]string{
			"start_time": fmt.Sprintf("%d", startTimeUnix),
			"end_time":   fmt.Sprintf("%d", endTimeUnix),
			"page_size":  fmt.Sprintf("%d", pageSize),
		}

		_, body, err := r.client.DoRaw(http.MethodGet, "/api/open/v1/security/edlp/events/list", query, nil)
		if err != nil {
			logger.Warn("[DLP] 调用飞连DLP事件API失败", zap.Error(err))
			return results, nil
		}

		var resp struct {
			Code    int                      `json:"code"`
			Message string                   `json:"message"`
			Data    interface{}              `json:"data"`
			Items   []map[string]interface{} `json:"items"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			logger.Warn("[DLP] 解析DLP事件响应失败", zap.Error(err))
			return results, nil
		}

		if resp.Code != 0 && resp.Code != 40000 {
			logger.Warn("[DLP] API返回错误", zap.Int("code", resp.Code), zap.String("message", resp.Message))
			return results, nil
		}

		items := resp.Items
		if len(items) == 0 {
			if resp.Data != nil {
				if dataMap, ok := resp.Data.(map[string]interface{}); ok {
					if itemsArr, ok2 := dataMap["items"].([]interface{}); ok2 {
						for _, it := range itemsArr {
							if m, ok3 := it.(map[string]interface{}); ok3 {
								items = append(items, m)
							}
						}
					}
				}
			}
		}

		logger.Info("[DLP] 时间分段获取数据", zap.Int("item_count", len(items)), zap.String("end_time", currentEndTime))

		if len(items) == 0 {
			break
		}

		var earliestTime int64 = endTimeUnix

		for _, item := range items {
			event := dlpEventFromMap(item)
			if event.ID != "" && !seenIDs[event.ID] {
				seenIDs[event.ID] = true
				results = append(results, event)

				if event.EventTime != "" {
					if t, err := time.Parse(time.RFC3339, event.EventTime); err == nil && t.Unix() < earliestTime {
						earliestTime = t.Unix()
					}
				}
				if eventUnix, ok := item["event_unix_time"].(float64); ok && int64(eventUnix) < earliestTime {
					earliestTime = int64(eventUnix)
				}
			}
		}

		if maxItems > 0 && len(results) >= maxItems {
			results = results[:maxItems]
			break
		}

		if earliestTime >= endTimeUnix {
			break
		}

		currentEndTime = time.Unix(earliestTime, 0).Format(time.RFC3339)
	}

	return results, nil
}

func dlpEventFromMap(m map[string]interface{}) storage.DLPEvent {
	getStr := func(key string) string {
		v, ok := m[key]
		if !ok {
			return ""
		}
		s, ok := v.(string)
		if ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	}

	id := getStr("id")
	if id == "" {
		id = getStr("event_id")
	}
	if id == "" {
		return storage.DLPEvent{}
	}

	rawJSON, _ := json.Marshal(m)

	fileInfoName := getStr("file_info_name")
	fileInfoPath := getStr("file_info_path")
	fileInfoType := getStr("file_info_type")
	leakWayAppName := getStr("leak_way_app_name")
	eventTime := getStr("event_time")
	userName := getStr("user_name")

	if fileInfoName == "" || fileInfoPath == "" {
		if fileInfo, ok := m["file_info"].(map[string]interface{}); ok {
			if name, ok := fileInfo["name"].(string); ok {
				fileInfoName = name
			}
			if path, ok := fileInfo["path"].(string); ok {
				fileInfoPath = path
			}
			if fileType, ok := fileInfo["type"].(string); ok {
				fileInfoType = fileType
			}
		}
	}

	if leakWayAppName == "" {
		if leakWay, ok := m["leak_way"].(map[string]interface{}); ok {
			if appName, ok := leakWay["app_name"].(string); ok {
				leakWayAppName = appName
			}
		}
	}

	if eventTime == "" {
		if eventUnixTime, ok := m["event_unix_time"].(float64); ok {
			eventTime = time.Unix(int64(eventUnixTime), 0).Format(time.RFC3339)
		}
	}

	if userName == "" {
		if userInfo, ok := m["user_info"].(map[string]interface{}); ok {
			if fullName, ok := userInfo["full_name"].(string); ok {
				userName = fullName
			}
			if userId, ok := userInfo["user_id"].(string); ok {
				if userName != "" {
					userName += " (" + userId + ")"
				} else {
					userName = userId
				}
			}
		}
	}

	return storage.DLPEvent{
		ID:             id,
		FileInfoName:   fileInfoName,
		FileInfoPath:   fileInfoPath,
		FileInfoType:   fileInfoType,
		LeakWayAppName: leakWayAppName,
		EventType:      getStr("event_type"),
		UserID:         getStr("user_id"),
		UserName:       userName,
		DeviceID:       getStr("device_id"),
		EventTime:      eventTime,
		RawJSON:        string(rawJSON),
	}
}

func (r *Runner) AnalyzeDLPEvents(maxItems int, reanalyzeRetained bool) (*DLPAnalyzeResult, error) {
	if r.dlpEventRepo == nil || r.dlpAnalysisRepo == nil || r.dlpWhitelistRepo == nil {
		return nil, fmt.Errorf("dlp repositories not initialized")
	}

	if maxItems <= 0 {
		maxItems = 100
	}

	logger.Info("[DLP] 开始分析DLP事件", zap.Int("max_items", maxItems), zap.Bool("reanalyze_retained", reanalyzeRetained))

	var events []storage.DLPEvent
	var err error

	if reanalyzeRetained {
		events, err = r.dlpEventRepo.ListRetainedAlerts(maxItems)
		if err != nil {
			return nil, fmt.Errorf("获取保留告警事件失败: %w", err)
		}
		logger.Info("[DLP] 获取到保留告警事件", zap.Int("count", len(events)))
	} else {
		events, err = r.dlpEventRepo.ListUnafelyzed(maxItems)
		if err != nil {
			return nil, fmt.Errorf("获取未分析事件失败: %w", err)
		}
		logger.Info("[DLP] 获取到未分析事件", zap.Int("count", len(events)))
	}

	if len(events) == 0 {
		return &DLPAnalyzeResult{AnalyzedAt: time.Now().Format(time.RFC3339)}, nil
	}

	llmCfg := r.cfg.LLM.Planner
	logger.Info("[DLP] 使用LLM配置", zap.String("provider", llmCfg.Provider), zap.String("model", llmCfg.Model))
	llmClient := llm.NewClient(llmCfg)

	analyzed := 0
	excluded := 0
	kept := 0

	for _, event := range events {
		whitelistItem, whitelisted, _ := r.dlpWhitelistRepo.MatchFile(event.FileInfoPath, event.FileInfoName)
		if whitelisted {
			result := storage.DLPAnalysisResult{
				ID:            "ana_" + event.ID,
				EventID:       event.ID,
				Category:      "whitelisted",
				ShouldExclude: true,
				Confidence:    1.0,
				Reasoning:     "命中白名单规则，直接跳过分析",
				Whitelisted:   true,
				MatchType:     whitelistItem.MatchType,
				MatchValue:    whitelistItem.MatchValue,
			}
			if err := r.dlpAnalysisRepo.Upsert(result); err != nil {
				logger.Warn("[DLP] 保存分析结果失败", zap.String("event_id", event.ID), zap.Error(err))
				continue
			}
			if err := r.dlpEventRepo.MarkAnalyzed(event.ID); err != nil {
				logger.Warn("[DLP] 标记事件已分析失败", zap.String("event_id", event.ID), zap.Error(err))
			}
			analyzed++
			excluded++
			continue
		}

		result, err := r.analyzeSingleDLPEvent(llmClient, event)
		if err != nil {
			logger.Warn("[DLP] LLM分析事件失败", zap.String("event_id", event.ID), zap.Error(err))
			result = storage.DLPAnalysisResult{
				ID:            "ana_" + event.ID,
				EventID:       event.ID,
				Category:      "unknown",
				ShouldExclude: false,
				Confidence:    0.0,
				Reasoning:     "LLM分析失败: " + err.Error(),
			}
		}

		result.EventID = event.ID
		result.ID = "ana_" + event.ID

		if err := r.dlpAnalysisRepo.Upsert(result); err != nil {
			logger.Warn("[DLP] 保存分析结果失败", zap.String("event_id", event.ID), zap.Error(err))
			continue
		}

		if err := r.dlpEventRepo.MarkAnalyzed(event.ID); err != nil {
			logger.Warn("[DLP] 标记事件已分析失败", zap.String("event_id", event.ID), zap.Error(err))
		}

		analyzed++
		if result.ShouldExclude {
			excluded++
		} else {
			kept++
		}
	}

	analyzedAt := time.Now().Format(time.RFC3339)
	if upErr := r.dlpEventRepo.UpdateAnalysisState(analyzedAt, analyzed); upErr != nil {
		logger.Warn("[DLP] 更新分析状态失败", zap.Error(upErr))
	}

	return &DLPAnalyzeResult{
		AnalyzedCount: analyzed,
		ExcludedCount: excluded,
		KeptCount:     kept,
		AnalyzedAt:    analyzedAt,
	}, nil
}

func (r *Runner) analyzeSingleDLPEvent(client *llm.Client, event storage.DLPEvent) (storage.DLPAnalysisResult, error) {
	systemPrompt := `你是一个文件分类助手。请根据给定的文件信息，判断该文件是否属于用户个人文件，还是属于操作系统、软件配置、日志、缓存等非用户个人文件。

分类标签：
- user_personal: 用户个人文件（文档、照片、项目资料等）
- os_system: 操作系统文件
- app_config: 软件配置文件
- app_log: 软件日志文件
- cache_temp: 缓存/临时文件
- app_data: 软件运行数据（非直接面向用户的内部数据）
- unknown: 无法判断

判断原则：宁可错判为用户文件（should_exclude=false），也不要误判为系统/软件文件。不确定时返回 unknown 且 should_exclude=false。

请严格以 JSON 格式输出，包含以下字段：
- category: 分类标签
- should_exclude: 是否应排除（不属于用户个人文件）
- confidence: 置信度（0-1之间的浮点数）
- reasoning: 推理依据（简短中文说明）`

	userPrompt := fmt.Sprintf(`请分析以下文件信息：

文件名: %s
文件路径: %s
文件类型: %s
泄露途径应用: %s

请判断该文件是否属于用户个人文件。`,
		event.FileInfoName, event.FileInfoPath, event.FileInfoType, event.LeakWayAppName)

	resp, err := client.Chat(llm.ChatRequest{
		SystemPrompt: systemPrompt,
		Prompt:       userPrompt,
	})
	if err != nil {
		return storage.DLPAnalysisResult{}, err
	}

	content := resp.Content
	content = extractJSONFromContent(content)

	var result struct {
		Category      string  `json:"category"`
		ShouldExclude bool    `json:"should_exclude"`
		Confidence    float64 `json:"confidence"`
		Reasoning     string  `json:"reasoning"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		logger.Warn("[DLP] 解析LLM响应失败", zap.String("content", content), zap.Error(err))
		return storage.DLPAnalysisResult{
			Category:      "unknown",
			ShouldExclude: false,
			Confidence:    0.0,
			Reasoning:     "LLM响应解析失败，默认视为用户文件",
		}, nil
	}

	return storage.DLPAnalysisResult{
		Category:      result.Category,
		ShouldExclude: result.ShouldExclude,
		Confidence:    result.Confidence,
		Reasoning:     result.Reasoning,
	}, nil
}

func toUnixTime(timeStr string) int64 {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		t, err = time.Parse("2006-01-02 15:04:05", timeStr)
		if err != nil {
			return time.Now().Unix()
		}
	}
	return t.Unix()
}

func extractJSONFromContent(content string) string {
	content = strings.TrimSpace(content)

	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}

	return content
}

// ===================== 自动化审批 =====================

// approvalEventPayload 飞书设备申报事件的通用结构（schema 2.0）。
// 实际事件体位于 event 字段中，不同租户字段可能略有差异，因此 event 保留为原始 map 以便灵活取值。
type approvalEventPayload struct {
	Schema string                 `json:"schema"`
	Header map[string]interface{} `json:"header"`
	Event  map[string]interface{} `json:"event"`
}

// ExtractApprovalEventID 从飞书事件回调体中提取事件 ID（用于幂等）。
func ExtractApprovalEventID(body map[string]interface{}) string {
	if header, ok := body["header"].(map[string]interface{}); ok {
		if v, ok := header["event_id"].(string); ok && v != "" {
			return v
		}
	}
	if v, ok := body["event_id"].(string); ok && v != "" {
		return v
	}
	return ""
}

// extractDeviceIdentifier 从设备记录体中提取设备标识，按优先级返回 (标识值, 标识类型)。
// 飞书 device_apply_event_v2 的设备字段位于 event.device_record 下，
// 可用标识：serial_number / mac_address / uuid / disk_serial_number / device_name。
// 用于在飞连中匹配设备的优先级：序列号 → MAC → UUID → 磁盘序列号 → 设备名。
func extractDeviceIdentifier(event map[string]interface{}) (string, string) {
	candidates := []struct {
		field string
		typ   string
	}{
		{"serial_number", "serial_number"},
		{"mac_address", "mac_address"},
		{"mac", "mac_address"},
		{"mac_addr", "mac_address"},
		{"uuid", "uuid"},
		{"disk_serial_number", "disk_serial_number"},
		{"device_name", "device_name"},
		{"device_id", "device_id"},
	}
	for _, c := range candidates {
		if v, ok := event[c.field]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprintf("%v", v))
			if s != "" {
				return s, c.typ
			}
		}
	}
	return "", ""
}

// findEventMap 从回调体中定位设备字段所在的子结构。
// 飞书 device_apply_event_v2 的设备字段位于 event.device_record 下；
// 若不存在则回退到 event，再回退到根 body，以兼容不同版本/自定义事件体。
func findEventMap(body map[string]interface{}) map[string]interface{} {
	if event, ok := body["event"].(map[string]interface{}); ok && len(event) > 0 {
		if record, ok := event["device_record"].(map[string]interface{}); ok && len(record) > 0 {
			return record
		}
		return event
	}
	return body
}

// ProcessDeviceApplyEvent 处理飞书设备申报事件：
//  1. 提取事件 ID 与设备标识；
//  2. 幂等检查（同 EventID 不重复处理）；
//  3. 在飞连中查询设备；
//  4. 命中白名单分组 → 调用飞书新增设备接口（公司设备+信任设备），状态=auto_approved；
//  5. 未命中 → 状态=pending，并通过配置的 Webhook 推送设备信息。
func (r *Runner) ProcessDeviceApplyEvent(ctx context.Context, body map[string]interface{}) error {
	if r.approvalTaskRepo == nil {
		return fmt.Errorf("approval task repository is nil")
	}

	eventID := ExtractApprovalEventID(body)
	eventMap := findEventMap(body)
	deviceIdentifier, identifierType := extractDeviceIdentifier(eventMap)

	logger.Info("[自动化审批] 收到设备申报事件",
		zap.String("event_id", eventID),
		zap.String("device_identifier", deviceIdentifier),
		zap.String("identifier_type", identifierType),
	)

	// 幂等检查：同一事件 ID 已处理过则直接返回
	if eventID != "" {
		if existing, ok, err := r.approvalTaskRepo.GetByEventID(eventID); err == nil && ok {
			logger.Info("[自动化审批] 事件已处理，跳过",
				zap.String("event_id", eventID),
				zap.String("status", string(existing.Status)),
			)
			return nil
		}
	}

	rawPayload, _ := json.Marshal(body)
	taskID := "appr_" + time.Now().Format("20060102_150405.000")
	task := storage.ApprovalTask{
		ID:               taskID,
		EventID:          eventID,
		DeviceIdentifier: deviceIdentifier,
		RawPayload:       string(rawPayload),
		Status:           storage.ApprovalStatusPending,
	}
	if err := r.approvalTaskRepo.Create(task); err != nil {
		return fmt.Errorf("create approval task: %w", err)
	}

	// 若未提取到设备标识，直接留存为 pending 并推送 Webhook
	if deviceIdentifier == "" {
		logger.Warn("[自动化审批] 未提取到设备标识，留存待审批",
			zap.String("task_id", taskID),
		)
		r.notifyPendingTask(ctx, taskID, body)
		return nil
	}

	// 在飞连中查询设备
	device, err := r.findDeviceInFeilian(deviceIdentifier, identifierType)
	if err != nil {
		logger.Warn("[自动化审批] 飞连设备查询失败，留存待审批",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		_ = r.approvalTaskRepo.UpdateError(taskID, "feilian query failed: "+err.Error())
		r.notifyPendingTask(ctx, taskID, body)
		return nil
	}
	if device == nil {
		logger.Info("[自动化审批] 飞连未找到该设备，留存待审批",
			zap.String("task_id", taskID),
			zap.String("device_identifier", deviceIdentifier),
		)
		r.notifyPendingTask(ctx, taskID, body)
		return nil
	}

	// 加载白名单配置，判断分组是否命中
	cfg, err := r.getApprovalConfig()
	if err != nil {
		_ = r.approvalTaskRepo.UpdateError(taskID, "load config failed: "+err.Error())
		r.notifyPendingTask(ctx, taskID, body)
		return nil
	}
	if !deviceGroupsHitWhitelist(*device, cfg.GroupIDs) {
		logger.Info("[自动化审批] 设备不在白名单分组，留存待审批",
			zap.String("task_id", taskID),
			zap.String("device_did", device.DID),
			zap.String("groups_name", device.GroupsName),
		)
		r.notifyPendingTask(ctx, taskID, body)
		return nil
	}

	// 命中白名单：调用飞书新增设备接口
	recordID, err := r.createFeishuDeviceRecord(*device)
	if err != nil {
		logger.Error("[自动化审批] 飞书新增设备失败",
			zap.String("task_id", taskID),
			zap.String("device_did", device.DID),
			zap.Error(err),
		)
		_ = r.approvalTaskRepo.UpdateError(taskID, "feishu create failed: "+err.Error())
		r.notifyPendingTask(ctx, taskID, body)
		return nil
	}

	_ = r.approvalTaskRepo.UpdateFeishuRecordID(taskID, recordID)
	_ = r.approvalTaskRepo.UpdateStatus(taskID, storage.ApprovalStatusAutoApproved)
	logger.Info("[自动化审批] 设备自动审批通过",
		zap.String("task_id", taskID),
		zap.String("device_did", device.DID),
		zap.String("feishu_device_record_id", recordID),
	)
	return nil
}

// getApprovalConfig 获取审批配置，若仓储未初始化则返回默认配置。
func (r *Runner) getApprovalConfig() (*storage.ApprovalConfig, error) {
	if r.approvalConfigRepo == nil {
		return &storage.ApprovalConfig{GroupIDs: []string{}}, nil
	}
	return r.approvalConfigRepo.Get()
}

// deviceGroupsHitWhitelist 判断设备所属分组是否与白名单分组有交集。
func deviceGroupsHitWhitelist(device storage.FeishuDeviceItem, whitelistGroupIDs []string) bool {
	if len(whitelistGroupIDs) == 0 {
		return false
	}
	whitelist := make(map[string]bool, len(whitelistGroupIDs))
	for _, id := range whitelistGroupIDs {
		whitelist[strings.TrimSpace(id)] = true
	}
	// 飞连设备的 groups_id 可能是逗号分隔的多个分组 ID
	for _, gid := range strings.Split(device.GroupsID, ",") {
		gid = strings.TrimSpace(gid)
		if gid != "" && whitelist[gid] {
			return true
		}
	}
	// 兜底：按分组名称匹配（白名单可能存的是名称）
	for _, name := range strings.Split(device.GroupsName, ",") {
		name = strings.TrimSpace(name)
		if name != "" && whitelist[name] {
			return true
		}
	}
	return false
}

// findDeviceInFeilian 在飞连中按设备标识查询设备。
// 优先尝试带过滤参数的查询；若接口不支持或无结果，则全量拉取后本地匹配。
func (r *Runner) findDeviceInFeilian(identifier, identifierType string) (*storage.FeishuDeviceItem, error) {
	if r.client == nil {
		return nil, fmt.Errorf("sealsuite client is nil")
	}
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return nil, nil
	}

	// 尝试带过滤参数的查询（不同飞连版本参数名可能不同，尝试多个）
	queryKeys := map[string][]string{
		"device_id":     {"device_id", "did"},
		"serial_number": {"serial_number"},
		"mac_address":   {"mac_address", "mac_addr", "mac"},
	}
	keys, ok := queryKeys[identifierType]
	if !ok {
		keys = []string{identifierType}
	}
	for _, key := range keys {
		params := map[string]string{key: identifier, "limit": "100", "offset": "0"}
		_, body, err := r.client.DoRaw(http.MethodGet, "/api/open/v1/device/search", params, nil)
		if err != nil {
			continue
		}
		if device, found := parseFirstDeviceFromSearch(body, identifier, identifierType); found {
			return device, nil
		}
	}

	// 回退：全量拉取后本地匹配
	all, err := r.SyncFeishuDevices()
	if err != nil {
		return nil, fmt.Errorf("fallback sync devices: %w", err)
	}
	for i := range all {
		if matchDeviceByIdentifier(all[i], identifier, identifierType) {
			return &all[i], nil
		}
	}
	return nil, nil
}

// matchDeviceByIdentifier 判断飞连设备是否匹配给定标识。
func matchDeviceByIdentifier(device storage.FeishuDeviceItem, identifier, identifierType string) bool {
	identifier = strings.TrimSpace(identifier)
	switch identifierType {
	case "device_id":
		return strings.TrimSpace(device.DID) == identifier
	case "serial_number":
		return strings.TrimSpace(device.SerialNumber) == identifier
	case "mac_address":
		return strings.EqualFold(strings.TrimSpace(device.MacAddr), identifier) ||
			strings.Contains(strings.ToLower(device.MacAddrs), strings.ToLower(identifier))
	}
	// 未知类型：尝试所有字段
	return strings.TrimSpace(device.DID) == identifier ||
		strings.TrimSpace(device.SerialNumber) == identifier ||
		strings.EqualFold(strings.TrimSpace(device.MacAddr), identifier)
}

// parseFirstDeviceFromSearch 解析飞连 device/search 响应，返回与标识匹配的第一台设备。
func parseFirstDeviceFromSearch(body []byte, identifier, identifierType string) (*storage.FeishuDeviceItem, bool) {
	var resp struct {
		Data struct {
			Items   []map[string]interface{} `json:"items"`
			Devices []map[string]interface{} `json:"devices"`
		} `json:"data"`
		Items   []map[string]interface{} `json:"items"`
		Devices []map[string]interface{} `json:"devices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, false
	}
	rawItems := resp.Data.Items
	if len(rawItems) == 0 {
		rawItems = resp.Data.Devices
	}
	if len(rawItems) == 0 {
		rawItems = resp.Items
	}
	if len(rawItems) == 0 {
		rawItems = resp.Devices
	}
	for _, raw := range rawItems {
		item := parseFeishuDeviceItemForApproval(raw)
		if matchDeviceByIdentifier(item, identifier, identifierType) {
			return &item, true
		}
	}
	return nil, false
}

// parseFeishuDeviceItemForApproval 将飞连设备原始 map 解析为 FeishuDeviceItem（仅提取审批所需字段）。
func parseFeishuDeviceItemForApproval(raw map[string]interface{}) storage.FeishuDeviceItem {
	device := storage.FeishuDeviceItem{}
	deviceInfo, _ := raw["device_info"].(map[string]interface{})
	getField := func(key string) string {
		if v, ok := raw[key]; ok && v != nil {
			return fmt.Sprintf("%v", v)
		}
		if deviceInfo != nil {
			if v, ok := deviceInfo[key]; ok && v != nil {
				return fmt.Sprintf("%v", v)
			}
		}
		return ""
	}
	device.DID = getField("did")
	device.UserID = getField("user_id")
	device.DeviceName = getField("device_name")
	if device.DeviceName == "" {
		device.DeviceName = getField("name")
	}
	device.FullName = getField("full_name")
	device.OS = getField("os")
	device.SerialNumber = getField("serial_number")
	device.MacAddr = getField("mac_addr")
	device.MacAddrs = getField("mac_addrs")
	device.HDDSerialNumbers = getField("hdd_serial_numbers")
	device.SSDSerialNumbers = getField("ssd_serial_numbers")
	device.CPUSerialNumber = getField("cpu_serial_number")
	device.GroupsID = getField("groups_id")
	device.GroupsName = getField("groups_name")
	device.TrustedStatus = getField("trusted_status")
	return device
}

// createFeishuDeviceRecord 调用飞书新增设备接口，将设备作为公司设备+信任设备写入。
// 复用 buildFeishuCreatePayload 构建请求体；返回 device_record_id。
func (r *Runner) createFeishuDeviceRecord(device storage.FeishuDeviceItem) (string, error) {
	api, err := r.getActiveFeishuAPI()
	if err != nil {
		return "", err
	}
	baseURL := strings.TrimRight(api.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://open.feishu.cn"
	}
	createURL := baseURL + "/open-apis/security_and_compliance/v2/device_records"

	mappings, err := r.fieldMappingRepo.GetEnabledMappings()
	if err != nil {
		return "", fmt.Errorf("get field mappings: %w", err)
	}

	// 强制覆盖：公司设备 + 信任设备（审批通过语义）
	payload := buildFeishuCreatePayload(device, mappings)
	payload["device_ownership"] = 2
	payload["device_status"] = 1

	statusCode, body, err := r.feishuDoRequest(http.MethodPost, createURL, payload)
	if err != nil {
		return "", fmt.Errorf("feishu create device record: %w", err)
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			DeviceRecordID string `json:"device_record_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse feishu create response: %w (raw: %s)", err, string(body))
	}
	if statusCode < 200 || statusCode >= 300 || resp.Code != 0 {
		return "", fmt.Errorf("feishu create device record failed: http=%d code=%d msg=%s", statusCode, resp.Code, resp.Msg)
	}
	return resp.Data.DeviceRecordID, nil
}

// notifyPendingTask 对待审批任务推送 Webhook 通知。
func (r *Runner) notifyPendingTask(ctx context.Context, taskID string, event map[string]interface{}) {
	if r.approvalConfigRepo == nil || r.webhookRepo == nil {
		return
	}
	cfg, err := r.approvalConfigRepo.Get()
	if err != nil || cfg.WebhookID == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	env := webhook.Envelope{
		Event:      "approval.pending",
		SourceType: "approval_task",
		SourceID:   taskID,
		SourceName: "设备申报待审批",
		Timestamp:  time.Now().Format(time.RFC3339),
		Data:       event,
	}
	delivery := webhook.DeliveryResult{Attempted: false}
	item, ok, err := getWebhookFromRepo(r.webhookRepo, cfg.WebhookID)
	if err == nil && ok {
		delivery = webhook.DeliverWebhookForSuccess(ctx, item, env)
	}
	deliveryJSON, _ := json.Marshal(delivery)
	_ = r.approvalTaskRepo.UpdateWebhookDelivery(taskID, string(deliveryJSON))
}

// ApproveTask 手动通过待审批任务：调用飞书新增设备接口，状态置为 approved。
func (r *Runner) ApproveTask(ctx context.Context, taskID string) error {
	if r.approvalTaskRepo == nil {
		return fmt.Errorf("approval task repository is nil")
	}
	task, ok, err := r.approvalTaskRepo.Get(taskID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("approval task not found: %s", taskID)
	}
	// 从原始 payload 中提取设备并在飞连中查询
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(task.RawPayload), &body); err != nil {
		return fmt.Errorf("parse task payload: %w", err)
	}
	eventMap := findEventMap(body)
	deviceIdentifier, identifierType := extractDeviceIdentifier(eventMap)
	if deviceIdentifier == "" {
		return fmt.Errorf("no device identifier in task payload")
	}
	device, err := r.findDeviceInFeilian(deviceIdentifier, identifierType)
	if err != nil {
		return fmt.Errorf("find device in feilian: %w", err)
	}
	if device == nil {
		return fmt.Errorf("device not found in feilian: %s", deviceIdentifier)
	}
	recordID, err := r.createFeishuDeviceRecord(*device)
	if err != nil {
		return err
	}
	_ = r.approvalTaskRepo.UpdateFeishuRecordID(taskID, recordID)
	_ = r.approvalTaskRepo.UpdateStatus(taskID, storage.ApprovalStatusApproved)
	return nil
}

// RejectTask 手动驳回待审批任务。
func (r *Runner) RejectTask(taskID string) error {
	if r.approvalTaskRepo == nil {
		return fmt.Errorf("approval task repository is nil")
	}
	_, ok, err := r.approvalTaskRepo.Get(taskID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("approval task not found: %s", taskID)
	}
	return r.approvalTaskRepo.UpdateStatus(taskID, storage.ApprovalStatusRejected)
}

// getWebhookFromRepo 从 Webhook 仓储加载指定配置（与 web 包内同名函数保持一致，避免循环依赖）。
func getWebhookFromRepo(repo *sqliteRepo.WebhookRepository, id string) (*storage.WebhookItem, bool, error) {
	if repo == nil {
		return nil, false, fmt.Errorf("webhook repository is nil")
	}
	return repo.Get(id)
}
