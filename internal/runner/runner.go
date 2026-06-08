package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"sealsuite-operation/internal/api"
	"sealsuite-operation/internal/config"
	"sealsuite-operation/internal/llm"
	"sealsuite-operation/internal/logger"
	"sealsuite-operation/internal/scheduler"
	"sealsuite-operation/internal/sealsuite"
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

type Runner struct {
	cfg *config.Config

	jobsStore       storage.JobsStore
	tplStore        storage.TemplatesStore
	taskDraftsStore storage.TaskDraftsStore
	schedulesStore  storage.JobSchedulesStore
	runStore        storage.JobRunsStore
	webhookStore    *storage.WebhookStore

	client   *sealsuite.Client
	executor *api.Executor

	sched         *scheduler.Scheduler
	jobsFile      *storage.JobsFile
	schedulesFile *storage.JobSchedulesFile
	templates     map[string]storage.Template
	taskDrafts    map[string]storage.TaskDraft

	status         map[string]*JobStatus
	scheduleStatus map[string]*JobStatus
}

func New(cfg *config.Config, client *sealsuite.Client, jobsPath, templatesPath string) *Runner {
	return &Runner{
		cfg:             cfg,
		jobsStore:       storage.JobsStore{Path: jobsPath},
		tplStore:        storage.TemplatesStore{Path: templatesPath},
		taskDraftsStore: storage.TaskDraftsStore{Path: "task-drafts.yaml"},
		schedulesStore:  storage.JobSchedulesStore{Path: "job-schedules.yaml"},
		runStore:        storage.JobRunsStore{Path: "job-runs.json"},
		webhookStore:    storage.NewWebhookStore("webhooks.yaml"),
		client:          client,
		executor:        api.NewExecutor(client, nil),
		status:          map[string]*JobStatus{},
		scheduleStatus:  map[string]*JobStatus{},
		taskDrafts:      map[string]storage.TaskDraft{},
	}
}

func (r *Runner) Executor() *api.Executor { return r.executor }

func (r *Runner) Config() *config.Config { return r.cfg }

const scheduleRunKeyPrefix = "schedule:"

// HotReloadConfig 用于 Web 控制台保存连接信息后在线生效：
// - 更新 cfg（影响 scheduler 时区/启用开关等）
// - 更新 client（影响飞连 API 调用）
// - executor 同步切换 client
func (r *Runner) HotReloadConfig(cfg *config.Config, client *sealsuite.Client) {
	if cfg != nil {
		r.cfg = cfg
	}
	if client != nil {
		r.client = client
		r.executor.SetClient(client)
	}
}

func (r *Runner) LoadSnapshot() (*storage.JobsFile, map[string]storage.Template, map[string]*JobStatus, error) {
	jf, err := r.jobsStore.Load()
	if err != nil {
		return nil, nil, nil, err
	}
	tf, err := r.tplStore.Load()
	if err != nil {
		return nil, nil, nil, err
	}
	tmap := map[string]storage.Template{}
	for _, t := range tf.Templates {
		tmap[t.ID] = t
	}
	// status copy
	smap := map[string]*JobStatus{}
	for k, v := range r.status {
		cp := *v
		smap[k] = &cp
	}

	rf, err := r.runStore.Load()
	if err == nil {
		for name, rec := range rf.Items {
			if len(name) > len(scheduleRunKeyPrefix) && name[:len(scheduleRunKeyPrefix)] == scheduleRunKeyPrefix {
				continue
			}
			st := smap[name]
			if st == nil {
				st = &JobStatus{}
				smap[name] = st
			}
			if rec.LastRun != "" {
				if ts, e := time.Parse(time.RFC3339, rec.LastRun); e == nil {
					st.LastRun = ts
				}
			}
			st.LastOK = rec.OK
			st.DurationMs = rec.DurationMs
			st.LastError = rec.Error
		}
	}
	return jf, tmap, smap, nil
}

func (r *Runner) LoadScheduleSnapshot() (*storage.JobSchedulesFile, map[string]storage.TaskDraft, map[string]*JobStatus, error) {
	sf, err := r.schedulesStore.Load()
	if err != nil {
		return nil, nil, nil, err
	}
	tf, err := r.taskDraftsStore.Load()
	if err != nil {
		return nil, nil, nil, err
	}
	dmap := map[string]storage.TaskDraft{}
	for _, d := range tf.Items {
		dmap[d.ID] = d
	}
	smap := map[string]*JobStatus{}
	for k, v := range r.scheduleStatus {
		cp := *v
		smap[k] = &cp
	}

	rf, err := r.runStore.Load()
	if err == nil {
		for key, rec := range rf.Items {
			if len(key) <= len(scheduleRunKeyPrefix) || key[:len(scheduleRunKeyPrefix)] != scheduleRunKeyPrefix {
				continue
			}
			id := key[len(scheduleRunKeyPrefix):]
			st := smap[id]
			if st == nil {
				st = &JobStatus{}
				smap[id] = st
			}
			if rec.LastRun != "" {
				if ts, e := time.Parse(time.RFC3339, rec.LastRun); e == nil {
					st.LastRun = ts
				}
			}
			st.LastOK = rec.OK
			st.DurationMs = rec.DurationMs
			st.LastError = rec.Error
		}
	}
	return sf, dmap, smap, nil
}

func (r *Runner) Reload() error {
	jf, err := r.jobsStore.Load()
	if err != nil {
		return err
	}
	tf, err := r.tplStore.Load()
	if err != nil {
		return err
	}

	tmap := map[string]storage.Template{}
	for _, t := range tf.Templates {
		tmap[t.ID] = t
	}
	df, err := r.taskDraftsStore.Load()
	if err != nil {
		return err
	}
	sf, err := r.schedulesStore.Load()
	if err != nil {
		return err
	}
	dmap := map[string]storage.TaskDraft{}
	for _, d := range df.Items {
		dmap[d.ID] = d
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

	r.sched.Start()
	r.refreshNextRun()
	return nil
}

func (r *Runner) Stop() {
	if r.sched != nil {
		r.sched.Stop()
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

func (r *Runner) RunScheduleOnce(ctx context.Context, id string) (webhook.DeliveryResult, error) {
	s, ok, err := r.schedulesStore.Get(id)
	if err != nil {
		return webhook.DeliveryResult{Attempted: false}, err
	}
	if !ok {
		return webhook.DeliveryResult{Attempted: false}, fmt.Errorf("job schedule not found: %s", id)
	}
	_, delivery, err := r.executeSchedule(ctx, *s)
	return delivery, err
}

func (r *Runner) RunComplexTask(task storage.ComplexTask) (*ComplexTaskRunResult, error) {
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
			result.FinishedAt = time.Now().Format(time.RFC3339)
			return result, err
		}
		item.OK = true
		item.Output = out
		current = out
		result.Steps = append(result.Steps, item)
	}

	result.OK = true
	result.FinalOutput = current
	result.FinishedAt = time.Now().Format(time.RFC3339)
	return result, nil
}

func (r *Runner) runJob(j storage.Job) error {
	start := time.Now()
	err := r.executeJob(j)
	dur := time.Since(start).Milliseconds()

	st := r.status[j.Name]
	if st == nil {
		st = &JobStatus{}
		r.status[j.Name] = st
	}
	st.LastRun = time.Now()
	st.DurationMs = dur
	st.LastOK = err == nil
	if err != nil {
		st.LastError = err.Error()
	} else {
		st.LastError = ""
	}
	_ = r.runStore.Put(j.Name, storage.JobRunRecord{
		LastRun:    st.LastRun.Format(time.RFC3339),
		OK:         st.LastOK,
		DurationMs: st.DurationMs,
		Error:      st.LastError,
	})
	r.refreshNextRunFor(j.Name)
	return err
}

func (r *Runner) runSchedule(s storage.JobSchedule) error {
	start := time.Now()
	_, _, err := r.executeSchedule(context.Background(), s)
	dur := time.Since(start).Milliseconds()

	st := r.ensureStatus(r.scheduleStatus, s.ID)
	st.LastRun = time.Now()
	st.DurationMs = dur
	st.LastOK = err == nil
	if err != nil {
		st.LastError = err.Error()
	} else {
		st.LastError = ""
	}
	_ = r.runStore.Put(scheduleRunKeyPrefix+s.ID, storage.JobRunRecord{
		LastRun:    st.LastRun.Format(time.RFC3339),
		OK:         st.LastOK,
		DurationMs: st.DurationMs,
		Error:      st.LastError,
	})
	r.refreshNextRunForSchedule(s.ID)
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
			Query:      coerceStringMap(input),
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
			PathParams: coerceStringMap(input), // 允许把变量放在 input 里
			Query:      coerceStringMap(input),
			Body:       extractBody(input),
		})
		return err

	default:
		return fmt.Errorf("unsupported job type: %s", j.Type)
	}
}

func (r *Runner) executeSchedule(ctx context.Context, s storage.JobSchedule) (interface{}, webhook.DeliveryResult, error) {
	targetType, targetID := normalizeTarget(s.TargetType, s.TargetID, s.DraftID)
	result, err := r.executeTargetOutputForSchedule(targetType, targetID)
	if err != nil {
		return nil, webhook.DeliveryResult{Attempted: false}, err
	}
	return result, r.deliverScheduleWebhook(ctx, s, targetType, targetID, result), nil
}

func (r *Runner) executeTaskDraft(d storage.TaskDraft) error {
	_, _, err := r.runTaskDraft(d)
	return err
}

func (r *Runner) runTaskDraft(d storage.TaskDraft) (interface{}, webhook.DeliveryResult, error) {
	return r.runTaskDraftWithDelivery(d, true)
}

func (r *Runner) ExecuteTransientTaskDraft(d storage.TaskDraft, enableWebhook bool) (interface{}, webhook.DeliveryResult, error) {
	return r.runTaskDraftWithDelivery(d, enableWebhook)
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
	input := d.InputConfig
	if input == nil {
		input = map[string]interface{}{}
	}
	req := api.ExecuteRequest{
		TemplateID: firstNonEmpty(d.SourceTemplateID, asString(input["template_id"])),
		Method:     asString(input["method"]),
		Path:       asString(input["path"]),
		Query:      coerceStringMap(mapValue(input, "query")),
		PathParams: coerceStringMap(mapValue(input, "path_params")),
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

func (r *Runner) loadTaskDraft(id string) (storage.TaskDraft, error) {
	if draft, ok := r.taskDrafts[id]; ok {
		return draft, nil
	}
	got, found, err := r.taskDraftsStore.Get(id)
	if err != nil {
		return storage.TaskDraft{}, err
	}
	if !found {
		return storage.TaskDraft{}, fmt.Errorf("task draft not found: %s", id)
	}
	return *got, nil
}

func (r *Runner) deliverTaskDraftWebhook(d storage.TaskDraft, result interface{}) webhook.DeliveryResult {
	webhookID := strings.TrimSpace(d.WebhookConfigID)
	delivery := webhook.DeliveryResult{Attempted: false}
	switch {
	case !d.WebhookEnabled || webhookID == "":
	case r.webhookStore == nil:
		delivery = webhook.DeliveryResult{Attempted: false, Error: "webhook store is nil"}
	default:
		item, ok, err := r.webhookStore.Get(webhookID)
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
	case r.webhookStore == nil:
		delivery = webhook.DeliveryResult{Attempted: false, Error: "webhook store is nil"}
	default:
		item, ok, err := r.webhookStore.Get(webhookID)
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

func (r *Runner) executeComplexTaskStep(step storage.ComplexTaskStep, current interface{}) (interface{}, error) {
	switch step.Type {
	case "api_call":
		targetType, targetID := normalizeTarget(asString(step.Config["target_type"]), asString(step.Config["target_id"]), asString(step.Config["draft_id"]))
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
			Model:        llmCfg.Model,
			SystemPrompt: llmCfg.SystemPrompt,
			Prompt:       prompt,
			Input:        current,
			Temperature:  floatPtrIfPositive(llmCfg.Temperature),
			MaxTokens:    intPtrIfPositive(llmCfg.MaxTokens),
			Thinking:     boolPtrIfTrue(llmCfg.Thinking),
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
	for name := range r.status {
		r.refreshNextRunFor(name)
	}
	for id := range r.scheduleStatus {
		r.refreshNextRunForSchedule(id)
	}
}

func (r *Runner) refreshNextRunFor(name string) {
	r.refreshNextRunWith(name, r.status, name)
}

func (r *Runner) refreshNextRunForSchedule(id string) {
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
		Query:      coerceStringMap(mapValue(input, "query")),
		PathParams: coerceStringMap(mapValue(input, "path_params")),
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
		return config.LLMRoleConfig{}, true, fmt.Errorf("指定的 LLM_API 不存在或已删除: %s", id)
	}
	if !selectedCfg.Enabled {
		return config.LLMRoleConfig{}, true, fmt.Errorf("指定的 LLM_API 已禁用: %s", id)
	}
	return applyLLMRoleOverrides(selectedCfg, overrides), true, nil
}

func (r *Runner) resolveLLMConfigFromAPIID(id string) (config.LLMRoleConfig, error) {
	store := storage.NewLLMAPIStore("llm-apis.yaml")
	file, err := store.Load()
	if err != nil {
		return config.LLMRoleConfig{}, err
	}
	id = strings.TrimSpace(id)
	for _, item := range file.Items {
		if item.ID != id {
			continue
		}
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
		}, nil
	}
	return config.LLMRoleConfig{}, fmt.Errorf("llm api not found: %s", id)
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
		store := storage.ComplexTasksStore{Path: "complex-tasks.yaml"}
		task, found, err := store.Get(targetID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("complex task not found: %s", targetID)
		}
		result, err := r.RunComplexTask(*task)
		if err != nil {
			return nil, err
		}
		return result.FinalOutput, nil
	case "scheduled_task", "job_schedule":
		store := storage.JobSchedulesStore{Path: "job-schedules.yaml"}
		s, found, err := store.Get(targetID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("job schedule not found: %s", targetID)
		}
		nextType, nextID := normalizeTarget(s.TargetType, s.TargetID, s.DraftID)
		return r.executeTargetOutput(nextType, nextID)
	default:
		return nil, fmt.Errorf("unsupported target_type: %s", targetType)
	}
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
		store := storage.JobSchedulesStore{Path: "job-schedules.yaml"}
		s, found, err := store.Get(targetID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("job schedule not found: %s", targetID)
		}
		nextType, nextID := normalizeTarget(s.TargetType, s.TargetID, s.DraftID)
		return r.executeTargetOutputForSchedule(nextType, nextID)
	default:
		return r.executeTargetOutput(targetType, targetID)
	}
}

func normalizeTarget(targetType, targetID, draftID string) (string, string) {
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
