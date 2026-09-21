package web

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"sealsuite-operation/internal/api"
	"sealsuite-operation/internal/config"
	appdb "sealsuite-operation/internal/db"
	"sealsuite-operation/internal/llm"
	"sealsuite-operation/internal/logger"
	"sealsuite-operation/internal/repository"
	sqliteRepo "sealsuite-operation/internal/repository/sqlite"
	"sealsuite-operation/internal/runner"
	"sealsuite-operation/internal/sealsuite"
	"sealsuite-operation/internal/service"
	"sealsuite-operation/internal/storage"
	"sealsuite-operation/internal/web/assets"
	"sealsuite-operation/internal/webhook"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func NewServer(cfg *config.Config, r *runner.Runner) (*http.Server, error) {
	return NewServerWithServices(cfg, r, nil)
}

func NewServerWithServices(cfg *config.Config, r *runner.Runner, services *service.Services) (*http.Server, error) {
	router, err := newRouter(cfg, r, services)
	if err != nil {
		return nil, err
	}
	addr := cfg.Server.Bind + ":" + strconv.Itoa(cfg.Server.Port)
	return &http.Server{
		Addr:    addr,
		Handler: router,
	}, nil
}

func openSQLiteDB(cfg *config.Config) (*sql.DB, error) {
	dbPath := strings.TrimSpace(cfg.Database.Path)
	if dbPath == "" {
		dbPath = "./data/app.db"
	}

	appDB, err := appdb.OpenSQLite(dbPath)
	if err != nil {
		return nil, err
	}
	if err := appdb.Migrate(appDB); err != nil {
		_ = appDB.Close()
		return nil, err
	}
	return appDB, nil
}

func newServices(cfg *config.Config) (*service.Services, error) {
	appDB, err := openSQLiteDB(cfg)
	if err != nil {
		return nil, err
	}

	taskDraftRepo := sqliteRepo.NewTaskDraftRepository(appDB)
	scheduleRepo := sqliteRepo.NewScheduleRepository(appDB)
	complexTaskRepo := sqliteRepo.NewComplexTaskRepository(appDB)
	externalIPSyncRepo := sqliteRepo.NewExternalIPSyncTaskRepository(appDB)
	return service.NewServices(taskDraftRepo, scheduleRepo, complexTaskRepo, externalIPSyncRepo), nil
}

func NewRouter(cfg *config.Config, r *runner.Runner) (http.Handler, error) {
	return newRouter(cfg, r, nil)
}

func newRouter(cfg *config.Config, r *runner.Runner, services *service.Services) (http.Handler, error) {
	rr := chi.NewRouter()
	configStore := storage.ConfigStore{Path: "config.yaml"}
	appDB, err := openSQLiteDB(cfg)
	if err != nil && services == nil {
		return nil, err
	}
	var (
		taskDraftRepo       *sqliteRepo.TaskDraftRepository
		scheduleRepo        *sqliteRepo.ScheduleRepository
		legacyJobRepo       repository.LegacyJobRepository
		connectionRepo      *sqliteRepo.ConnectionsRepository
		llmAPIRepo          *sqliteRepo.LLMAPIRepository
		feishuAPIRepo       *sqliteRepo.FeishuAPIRepository
		feishuResourcesRepo *sqliteRepo.FeishuResourcesRepository
		feishuDevicesRepo   *sqliteRepo.FeishuDevicesRepository
		deviceGroupsRepo    *sqliteRepo.DeviceGroupsRepository
		fieldMappingRepo    *sqliteRepo.FieldMappingRepository
		dlpEventRepo        *sqliteRepo.DLPEventRepository
		dlpAnalysisRepo     *sqliteRepo.DLPAnalysisRepository
		dlpWhitelistRepo    *sqliteRepo.DLPWhitelistRepository
		ztnaLogRepo         *sqliteRepo.ZTNAAccessLogRepository
		ztnaStatsRepo       *sqliteRepo.ZTNAAccessStatsRepository
		ztnaAnalysisRepo    *sqliteRepo.ZTNAAnalysisRepository
		ztnaTaskStateRepo   *sqliteRepo.ZTNATaskStateRepository
		webhookRepo         *sqliteRepo.WebhookRepository
		templateRepo        *sqliteRepo.TemplateRepository
		outputTemplateRepo  *sqliteRepo.OutputTemplateRepository
		complexTaskRepo     *sqliteRepo.ComplexTaskRepository
		externalIPSyncRepo  *sqliteRepo.ExternalIPSyncTaskRepository
		jobRunsRepo         *sqliteRepo.JobRunsRepository
		approvalConfigRepo  *sqliteRepo.ApprovalConfigRepository
		approvalTaskRepo    *sqliteRepo.ApprovalTaskRepository
	)
	if appDB != nil {
		taskDraftRepo = sqliteRepo.NewTaskDraftRepository(appDB)
		scheduleRepo = sqliteRepo.NewScheduleRepository(appDB)
		legacyJobRepo = sqliteRepo.NewLegacyJobRepository(appDB)
		connectionRepo = sqliteRepo.NewConnectionsRepository(appDB)
		llmAPIRepo = sqliteRepo.NewLLMAPIRepository(appDB)
		feishuAPIRepo = sqliteRepo.NewFeishuAPIRepository(appDB)
		feishuResourcesRepo = sqliteRepo.NewFeishuResourcesRepository(appDB)
		feishuDevicesRepo = sqliteRepo.NewFeishuDevicesRepository(appDB)
		deviceGroupsRepo = sqliteRepo.NewDeviceGroupsRepository(appDB)
		fieldMappingRepo = sqliteRepo.NewFieldMappingRepository(appDB)
		dlpEventRepo = sqliteRepo.NewDLPEventRepository(appDB)
		dlpAnalysisRepo = sqliteRepo.NewDLPAnalysisRepository(appDB)
		dlpWhitelistRepo = sqliteRepo.NewDLPWhitelistRepository(appDB)
		ztnaLogRepo = sqliteRepo.NewZTNAAccessLogRepository(appDB)
		ztnaStatsRepo = sqliteRepo.NewZTNAAccessStatsRepository(appDB)
		ztnaAnalysisRepo = sqliteRepo.NewZTNAAnalysisRepository(appDB)
		ztnaTaskStateRepo = sqliteRepo.NewZTNATaskStateRepository(appDB)
		webhookRepo = sqliteRepo.NewWebhookRepository(appDB)
		templateRepo = sqliteRepo.NewTemplateRepository(appDB)
		outputTemplateRepo = sqliteRepo.NewOutputTemplateRepository(appDB)
		complexTaskRepo = sqliteRepo.NewComplexTaskRepository(appDB)
		externalIPSyncRepo = sqliteRepo.NewExternalIPSyncTaskRepository(appDB)
		jobRunsRepo = sqliteRepo.NewJobRunsRepository(appDB)
		approvalConfigRepo = sqliteRepo.NewApprovalConfigRepository(appDB)
		approvalTaskRepo = sqliteRepo.NewApprovalTaskRepository(appDB)
	}
	if services == nil {
		services = service.NewServices(
			taskDraftRepo,
			scheduleRepo,
			complexTaskRepo,
			externalIPSyncRepo,
		)
	}

	saveTaskDraft := func(draft storage.TaskDraft) error {
		return services.TaskDrafts.Save(draft)
	}

	saveJobSchedule := func(sched storage.JobSchedule) error {
		return services.Schedules.Save(sched)
	}

	saveComplexTask := func(task storage.ComplexTask) error {
		return services.ComplexTasks.Save(task)
	}

	deleteComplexTask := func(id string) error {
		return services.ComplexTasks.Delete(id)
	}

	loadTaskDrafts := func() (*storage.TaskDraftsFile, error) {
		items, err := taskDraftRepo.List()
		if err != nil {
			return nil, err
		}
		tf := &storage.TaskDraftsFile{Version: 1, Items: items}
		storage.EnsureTaskDraftsFileDefaults(tf)
		return tf, nil
	}

	getTaskDraft := func(id string) (storage.TaskDraft, bool, error) {
		return taskDraftRepo.Get(id)
	}

	getJobSchedule := func(id string) (storage.JobSchedule, bool, error) {
		return scheduleRepo.Get(id)
	}

	loadConnections := func() (*storage.ConnectionsFile, error) {
		return loadConnectionsFromRepo(connectionRepo)
	}

	loadLLMAPIs := func() (*storage.LLMAPIFile, error) {
		return loadLLMAPIsFromRepo(llmAPIRepo)
	}

	loadFeishuAPIs := func() (*storage.FeishuAPIFile, error) {
		if feishuAPIRepo == nil {
			return nil, fmt.Errorf("feishu api repository is nil")
		}
		return feishuAPIRepo.Load()
	}

	loadWebhooks := func() (*storage.WebhookFile, error) {
		return loadWebhooksFromRepo(webhookRepo)
	}

	loadTemplates := func() (*storage.TemplatesFile, error) {
		return loadTemplatesFromRepo(templateRepo)
	}

	loadOutputTemplates := func() (*storage.OutputTemplatesFile, error) {
		return loadOutputTemplatesFromRepo(outputTemplateRepo)
	}

	loadComplexTasks := func() (*storage.ComplexTasksFile, error) {
		return loadComplexTasksFromRepo(complexTaskRepo)
	}

	listExternalIPSyncTasks := func() ([]storage.ExternalIPSyncTask, error) {
		if externalIPSyncRepo == nil {
			return nil, fmt.Errorf("external ip sync task repository is nil")
		}
		return externalIPSyncRepo.List()
	}

	getExternalIPSyncTask := func(id string) (storage.ExternalIPSyncTask, bool, error) {
		if externalIPSyncRepo == nil {
			return storage.ExternalIPSyncTask{}, false, fmt.Errorf("external ip sync task repository is nil")
		}
		return externalIPSyncRepo.Get(id)
	}

	saveExternalIPSyncTask := func(task storage.ExternalIPSyncTask) error {
		if services == nil || services.ExternalIPSync == nil {
			return fmt.Errorf("external ip sync task service is nil")
		}
		return services.ExternalIPSync.Save(task)
	}

	deleteExternalIPSyncTask := func(id string) error {
		if externalIPSyncRepo == nil {
			return fmt.Errorf("external ip sync task repository is nil")
		}
		return externalIPSyncRepo.Delete(id)
	}

	listExternalIPSyncResources := func() ([]map[string]interface{}, string, string, error) {
		snapshotItems := []map[string]interface{}{}
		if externalIPSyncRepo != nil {
			tasks, err := externalIPSyncRepo.List()
			if err != nil {
				return nil, "", "", err
			}
			snapshotItems = externalIPSyncResourceItemsFromTasks(tasks)
		}

		liveItems, source, liveErr := loadExternalIPSyncResourceItems(cfg, configStore, loadTemplates)
		items := mergeExternalIPSyncResourceItems(snapshotItems, liveItems)
		if len(items) == 0 && len(snapshotItems) > 0 {
			source = "task_snapshots"
		}
		warning := ""
		if liveErr != nil {
			warning = liveErr.Error()
		}
		return items, source, warning, nil
	}

	getTemplate := func(id string) (*storage.Template, bool, error) {
		return getTemplateFromRepo(templateRepo, id)
	}

	getOutputTemplate := func(id string) (*storage.OutputTemplate, bool, error) {
		return getOutputTemplateFromRepo(outputTemplateRepo, id)
	}

	getComplexTask := func(id string) (storage.ComplexTask, bool, error) {
		return getComplexTaskFromRepo(complexTaskRepo, id)
	}

	hotReloadRuntime := func() error {
		newCfg, err := config.Load("config.yaml")
		if err != nil {
			return err
		}
		newClient := sealsuite.NewClient(&newCfg.SealSuite)
		newClient.SetMockMode(newCfg.SealSuite.MockMode)
		r.HotReloadConfig(newCfg, newClient)
		return r.Reload()
	}

	sub, err := fs.Sub(assets.FS, "ui")
	if err != nil {
		return nil, err
	}

	// NOTE: http.FileServer 在某些路径情况下会返回 301（canonical redirect），
	// 例如自动补齐/清理路径。为确保根路径稳定返回 200（并满足单测），
	// 这里对 index/css/js 采用“直接读 embed 文件”的方式提供。
	serveEmbed := func(name string, contentType string) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			b, err := fs.ReadFile(sub, strings.TrimPrefix(name, "/"))
			if err != nil {
				http.NotFound(w, req)
				return
			}
			if contentType != "" {
				w.Header().Set("Content-Type", contentType)
			}
			// 添加缓存控制头，强制浏览器每次获取最新版本
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
			_, _ = w.Write(b)
		}
	}

	rr.Get("/", serveEmbed("index.html", "text/html; charset=utf-8"))
	rr.Get("/styles.css", serveEmbed("styles.css", "text/css; charset=utf-8"))
	rr.Get("/app.js", serveEmbed("app.js", "application/javascript; charset=utf-8"))

	rr.Route("/api/v1", func(apiR chi.Router) {
		apiR.Get("/health", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})
		apiR.Get("/version", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"version": "0.1.0"})
		})

		// --- task drafts: API 工具箱产出的任务定义草稿 ---
		apiR.Get("/task-drafts", func(w http.ResponseWriter, req *http.Request) {
			tf, err := loadTaskDrafts()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, tf)
		})

		apiR.Get("/task-drafts/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			draft, ok, err := getTaskDraft(id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "task draft not found"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"draft": draft})
		})

		apiR.Post("/task-drafts", func(w http.ResponseWriter, req *http.Request) {
			var draft storage.TaskDraft
			if err := json.NewDecoder(req.Body).Decode(&draft); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			normalizeTaskDraftWebhookReference(&draft)
			if draft.ID == "" || draft.Name == "" {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "id/name required"})
				return
			}
			if err := saveTaskDraft(draft); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Put("/task-drafts/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			var draft storage.TaskDraft
			if err := json.NewDecoder(req.Body).Decode(&draft); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if draft.ID == "" {
				draft.ID = id
			}
			normalizeTaskDraftWebhookReference(&draft)
			if draft.ID != id {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "path id and body id mismatch"})
				return
			}
			if draft.Name == "" {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "name required"})
				return
			}
			if err := saveTaskDraft(draft); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Delete("/task-drafts/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if refs, err := referencedSchedulesByDraft(scheduleRepo, id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			} else if len(refs) > 0 {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{
					"error":         "task draft is referenced by schedules",
					"referenced_by": refs,
				})
				return
			}
			if err := taskDraftRepo.Delete(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		// --- job schedules: Jobs 调度中心 ---
		apiR.Get("/job-schedules", func(w http.ResponseWriter, req *http.Request) {
			jf, drafts, runs, err := r.LoadScheduleSnapshot()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			latestScheduleRuns, _ := latestScheduleRunLogs(jobRunsRepo)
			scheduleStatusPayload := map[string]interface{}{}
			for _, sched := range jf.Items {
				statusItem := jobStatusPayload(runs[sched.ID])
				normalizeScheduleTarget(&sched)
				if sched.TargetType == "external_ip_sync" {
					if latestRun, ok := latestScheduleRuns[sched.ID]; ok {
						if summary := externalIPSyncRunSummaryFromRunLog(latestRun); len(summary) > 0 {
							statusItem["summary"] = summary
							statusItem["external_ip_sync_summary"] = summary
						}
					}
					if task, ok, err := getExternalIPSyncTask(sched.TargetID); err == nil && ok {
						statusItem["task_summary"] = buildExternalIPSyncTaskSummary(task)
					}
				}
				scheduleStatusPayload[sched.ID] = statusItem
			}
			for scheduleID, status := range runs {
				if _, ok := scheduleStatusPayload[scheduleID]; ok {
					continue
				}
				scheduleStatusPayload[scheduleID] = jobStatusPayload(status)
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"version":         jf.Version,
				"items":           jf.Items,
				"drafts":          drafts,
				"schedule_status": scheduleStatusPayload,
			})
		})

		apiR.Get("/job-schedules/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			sched, ok, err := getJobSchedule(id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "job schedule not found"})
				return
			}
			_, drafts, runs, _ := r.LoadScheduleSnapshot()
			targetType := sched.TargetType
			targetID := sched.TargetID
			if targetType == "" && sched.DraftID != "" {
				targetType = "task_draft"
				targetID = sched.DraftID
			}
			targetSummary := map[string]interface{}{
				"type": targetType,
				"id":   targetID,
			}
			if targetType == "task_draft" {
				targetSummary["draft"] = drafts[targetID]
			}
			runPayload := jobStatusPayload(runs[id])
			latestScheduleRuns, _ := latestScheduleRunLogs(jobRunsRepo)
			if targetType == "external_ip_sync" {
				if task, found, err := getExternalIPSyncTask(targetID); err == nil && found {
					targetSummary["external_task"] = task
					targetSummary["external_ip_sync_summary"] = buildExternalIPSyncTaskSummary(task)
				}
			}
			if latestRun, ok := latestScheduleRuns[id]; ok && targetType == "external_ip_sync" {
				if summary := externalIPSyncRunSummaryFromRunLog(latestRun); len(summary) > 0 {
					runPayload["summary"] = summary
					runPayload["external_ip_sync_summary"] = summary
					if existing, ok := targetSummary["external_ip_sync_summary"].(map[string]interface{}); ok {
						targetSummary["external_ip_sync_summary"] = mergeStringAnyMap(existing, summary)
					} else {
						targetSummary["external_ip_sync_summary"] = cloneMapAny(summary)
					}
				}
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"schedule": sched,
				"draft":    drafts[targetID],
				"target":   targetSummary,
				"run":      runPayload,
			})
		})

		apiR.Post("/job-schedules", func(w http.ResponseWriter, req *http.Request) {
			var sched storage.JobSchedule
			if err := json.NewDecoder(req.Body).Decode(&sched); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			normalizeJobScheduleWebhookReference(&sched)
			if sched.ID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "id required"})
				return
			}
			normalizeScheduleTarget(&sched)
			if err := validateScheduleTarget(sched, taskDraftRepo, complexTaskRepo, externalIPSyncRepo); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			if err := saveJobSchedule(sched); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Put("/job-schedules/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			var sched storage.JobSchedule
			if err := json.NewDecoder(req.Body).Decode(&sched); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if sched.ID == "" {
				sched.ID = id
			}
			normalizeJobScheduleWebhookReference(&sched)
			if sched.ID != id {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "path id and body id mismatch"})
				return
			}
			normalizeScheduleTarget(&sched)
			if err := validateScheduleTarget(sched, taskDraftRepo, complexTaskRepo, externalIPSyncRepo); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			if err := saveJobSchedule(sched); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Delete("/job-schedules/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			shouldDeleteExternalTask := false
			externalTaskID := ""
			if sched, ok, err := getJobSchedule(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			} else if ok {
				targetType, targetID := runner.NormalizeTarget(sched.TargetType, sched.TargetID, sched.DraftID)
				if targetType == "external_ip_sync" && targetID != "" {
					refs, err := referencedSchedulesByTarget(scheduleRepo, targetType, targetID, id)
					if err != nil {
						writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
						return
					}
					shouldDeleteExternalTask = len(refs) == 0
					externalTaskID = targetID
				}
			}
			if err := scheduleRepo.Delete(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if shouldDeleteExternalTask {
				if err := deleteExternalIPSyncTask(externalTaskID); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
					return
				}
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Post("/job-schedules/{id}/run", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			delivery, err := r.RunScheduleOnce(req.Context(), id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":               true,
				"webhook_delivery": delivery,
			})
		})

		// --- external ip sync tasks: 可手工配置的 Google IP 同步任务定义 ---
		apiR.Get("/external-ip-sync-tasks", func(w http.ResponseWriter, req *http.Request) {
			items, err := listExternalIPSyncTasks()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
		})

		apiR.Get("/external-ip-sync-resources", func(w http.ResponseWriter, req *http.Request) {
			items, source, warning, err := listExternalIPSyncResources()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			resp := map[string]interface{}{
				"items":  items,
				"source": source,
			}
			if warning != "" {
				resp["warning"] = warning
			}
			writeJSON(w, http.StatusOK, resp)
		})

		apiR.Post("/external-ip-sync-tasks", func(w http.ResponseWriter, req *http.Request) {
			var task storage.ExternalIPSyncTask
			if err := json.NewDecoder(req.Body).Decode(&task); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if err := saveExternalIPSyncTask(task); err != nil {
				writeErrorJSON(w, err, nil)
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Get("/external-ip-sync-tasks/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			task, ok, err := getExternalIPSyncTask(id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "external ip sync task not found"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"task":    task,
				"summary": buildExternalIPSyncTaskSummary(task),
			})
		})

		apiR.Delete("/external-ip-sync-tasks/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if err := deleteExternalIPSyncTask(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Post("/external-ip-sync-tasks/{id}/run", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner is nil"})
				return
			}
			summary, err := r.RunExternalIPSyncTask(id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"summary": summary,
			})
		})

		// --- feishu resources: 飞连资源清单缓存 ---
		apiR.Get("/feishu-resources", func(w http.ResponseWriter, req *http.Request) {
			if feishuResourcesRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			typeFilter := strings.TrimSpace(req.URL.Query().Get("type"))
			items, err := feishuResourcesRepo.List(typeFilter)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			state, _ := feishuResourcesRepo.GetState()
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"items":            items,
				"count":            len(items),
				"last_refresh_at":  state.LastRefreshAt,
				"schedule_enabled": state.ScheduleEnabled,
			})
		})

		apiR.Post("/feishu-resources/refresh", func(w http.ResponseWriter, req *http.Request) {
			if feishuResourcesRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			items, err := r.RefreshFeishuResources()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			now := time.Now().Format(time.RFC3339)
			state, _ := feishuResourcesRepo.GetState()
			if upErr := feishuResourcesRepo.UpdateState(now, len(items), nil); upErr != nil {
				logger.Error("[资源清单] 更新刷新状态失败", zap.Error(upErr))
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":               true,
				"items":            items,
				"count":            len(items),
				"last_refresh_at":  now,
				"schedule_enabled": state.ScheduleEnabled,
			})
		})

		apiR.Post("/feishu-resources/schedule", func(w http.ResponseWriter, req *http.Request) {
			if feishuResourcesRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			var body struct {
				Enabled bool `json:"enabled"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if err := feishuResourcesRepo.UpdateState("", 0, &body.Enabled); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "schedule_enabled": body.Enabled})
		})

		apiR.Get("/feishu-resources/:resource_id", func(w http.ResponseWriter, req *http.Request) {
			if feishuResourcesRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			resourceID := chi.URLParam(req, "resource_id")
			if resourceID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "resource_id is required"})
				return
			}
			item, found, err := feishuResourcesRepo.Get(resourceID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if !found {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "resource not found"})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"item":  item,
				"cidrs": []string{},
			})
		})

		// --- feishu devices: 可信设备 ---
		apiR.Get("/feishu-devices", func(w http.ResponseWriter, req *http.Request) {
			if feishuDevicesRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			osFilter := strings.TrimSpace(req.URL.Query().Get("os"))
			trustedStatusFilter := strings.TrimSpace(req.URL.Query().Get("trusted_status"))
			groupFilter := strings.TrimSpace(req.URL.Query().Get("group"))
			items, err := feishuDevicesRepo.List(osFilter, trustedStatusFilter, groupFilter)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			state, _ := feishuDevicesRepo.GetState()
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"items":                    items,
				"count":                    len(items),
				"last_sync_at":             state.LastSyncAt,
				"last_sync_count":          state.LastSyncCount,
				"schedule_enabled":         state.ScheduleEnabled,
				"schedule_interval":        state.ScheduleInterval,
				"last_import_at":           state.LastImportAt,
				"last_import_count":        state.LastImportCount,
				"import_schedule_enabled":  state.ImportScheduleEnabled,
				"import_schedule_interval": state.ImportScheduleInterval,
			})
		})

		apiR.Get("/feishu-devices/filters", func(w http.ResponseWriter, req *http.Request) {
			if feishuDevicesRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			osList, _ := feishuDevicesRepo.GetDistinctOS()
			groupsList, _ := feishuDevicesRepo.GetDistinctGroups()
			statusList, _ := feishuDevicesRepo.GetDistinctTrustedStatus()
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"os_options":             osList,
				"group_options":          groupsList,
				"trusted_status_options": statusList,
			})
		})

		apiR.Post("/feishu-devices/sync", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			items, err := r.SyncFeishuDevices()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			now := time.Now().Format(time.RFC3339)
			if feishuDevicesRepo != nil {
				state, _ := feishuDevicesRepo.GetState()
				state.LastSyncAt = now
				state.LastSyncCount = len(items)
				_ = feishuDevicesRepo.UpdateState(state)
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"count":   len(items),
				"sync_at": now,
			})
		})

		apiR.Post("/feishu-devices/sync-schedule", func(w http.ResponseWriter, req *http.Request) {
			if feishuDevicesRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			var body struct {
				Enabled  bool   `json:"enabled"`
				Interval string `json:"interval"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			state, _ := feishuDevicesRepo.GetState()
			state.ScheduleEnabled = body.Enabled
			state.ScheduleInterval = body.Interval
			if err := feishuDevicesRepo.UpdateState(state); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "schedule_enabled": body.Enabled, "interval": body.Interval})
		})

		apiR.Post("/feishu-devices/import", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			var body struct {
				OSList            []string `json:"os_list"`
				TrustedStatusList []string `json:"trusted_status_list"`
				GroupList         []string `json:"group_list"`
				LogicMode         string   `json:"logic_mode"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid request body: " + err.Error()})
				return
			}
			cleanStrList := func(list []string) []string {
				out := make([]string, 0, len(list))
				for _, v := range list {
					v = strings.TrimSpace(v)
					if v != "" && v != "*" {
						out = append(out, v)
					}
				}
				return out
			}
			osList := cleanStrList(body.OSList)
			trustedList := cleanStrList(body.TrustedStatusList)
			groupList := cleanStrList(body.GroupList)
			logicMode := strings.ToUpper(strings.TrimSpace(body.LogicMode))
			if logicMode != "OR" {
				logicMode = "AND"
			}
			count, err := r.ImportDevicesToFeishuWithFilters(osList, trustedList, groupList, logicMode)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			now := time.Now().Format(time.RFC3339)
			if feishuDevicesRepo != nil {
				state, _ := feishuDevicesRepo.GetState()
				state.LastImportAt = now
				state.LastImportCount = count
				_ = feishuDevicesRepo.UpdateState(state)
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "imported_count": count, "import_at": now})
		})

		apiR.Post("/feishu-devices/import-schedule", func(w http.ResponseWriter, req *http.Request) {
			if feishuDevicesRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			var body struct {
				Enabled  bool   `json:"enabled"`
				Interval string `json:"interval"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			state, _ := feishuDevicesRepo.GetState()
			state.ImportScheduleEnabled = body.Enabled
			state.ImportScheduleInterval = body.Interval
			if err := feishuDevicesRepo.UpdateState(state); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "import_schedule_enabled": body.Enabled, "interval": body.Interval})
		})

		// --- device groups: 设备分组 ---
		apiR.Get("/device-groups", func(w http.ResponseWriter, req *http.Request) {
			if deviceGroupsRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			items, err := deviceGroupsRepo.List()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			state, _ := deviceGroupsRepo.GetState()
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"items":              items,
				"count":              len(items),
				"last_refresh_at":    state.LastRefreshAt,
				"last_refresh_count": state.LastRefreshCount,
			})
		})

		apiR.Post("/device-groups/refresh", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			items, err := r.RefreshDeviceGroups()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			now := time.Now().Format(time.RFC3339)
			if deviceGroupsRepo != nil {
				_ = deviceGroupsRepo.UpdateState(now, len(items))
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":         true,
				"items":      items,
				"count":      len(items),
				"refresh_at": now,
			})
		})

		apiR.Get("/device-groups/{id}/devices", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			id := chi.URLParam(req, "id")
			detail, err := r.GetDevicesByGroup(id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":       true,
				"group_id": detail.GroupID,
				"count":    detail.Count,
				"items":    detail.Items,
			})
		})

		// --- field mappings: 字段映射 ---
		apiR.Get("/field-mappings", func(w http.ResponseWriter, req *http.Request) {
			if fieldMappingRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			items, err := fieldMappingRepo.List()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"items": items, "count": len(items)})
		})

		apiR.Post("/field-mappings", func(w http.ResponseWriter, req *http.Request) {
			if fieldMappingRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			var item storage.FieldMappingItem
			if err := json.NewDecoder(req.Body).Decode(&item); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if err := fieldMappingRepo.Upsert(item); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Delete("/field-mappings/{id}", func(w http.ResponseWriter, req *http.Request) {
			if fieldMappingRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			id := chi.URLParam(req, "id")
			if err := fieldMappingRepo.Delete(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Post("/field-mappings/bootstrap", func(w http.ResponseWriter, req *http.Request) {
			if fieldMappingRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			if err := fieldMappingRepo.BootstrapDefaults(); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		// --- complex tasks: 工作流编排与未来 Agent 扩展 ---
		apiR.Get("/complex-tasks", func(w http.ResponseWriter, req *http.Request) {
			cf, err := loadComplexTasks()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, cf)
		})

		apiR.Post("/complex-tasks/run", func(w http.ResponseWriter, req *http.Request) {
			var task storage.ComplexTask
			if err := json.NewDecoder(req.Body).Decode(&task); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			normalizeComplexTaskWebhookReference(&task)
			if task.Name == "" {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "name required"})
				return
			}
			result, err := r.RunComplexTask(task)
			delivery := webhook.DeliveryResult{Attempted: false}
			if err == nil {
				delivery = deliverWebhookFromConfig(req.Context(), webhookRepo, task.WebhookEnabled, task.WebhookConfigID, webhook.Envelope{
					Event:      "task.completed",
					SourceType: "complex_task",
					SourceID:   firstNonEmptyString(task.ID, "temporary_complex_task"),
					SourceName: firstNonEmptyString(task.Name, "运营agent"),
					Timestamp:  time.Now().Format(time.RFC3339),
					Data:       complexTaskWebhookData(result),
				})
			}
			resp := map[string]interface{}{
				"ok":               err == nil,
				"task":             task,
				"steps":            result.Steps,
				"final_output":     result.FinalOutput,
				"started_at":       result.StartedAt,
				"finished_at":      result.FinishedAt,
				"webhook_delivery": delivery,
			}
			if err != nil {
				resp["error"] = err.Error()
			}
			writeJSON(w, http.StatusOK, resp)
		})

		apiR.Get("/complex-tasks/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			task, ok, err := getComplexTask(id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "complex task not found"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"task": task})
		})

		apiR.Get("/output-templates", func(w http.ResponseWriter, req *http.Request) {
			tf, err := loadOutputTemplates()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, tf)
		})

		apiR.Get("/output-templates/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			item, ok, err := getOutputTemplate(id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "output template not found"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"template": item})
		})

		apiR.Post("/output-templates", func(w http.ResponseWriter, req *http.Request) {
			var item storage.OutputTemplate
			if err := json.NewDecoder(req.Body).Decode(&item); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if item.ID == "" || item.Name == "" || item.Format == "" {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "id/name/format required"})
				return
			}
			if err := outputTemplateRepo.Upsert(item); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Delete("/output-templates/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if err := outputTemplateRepo.Delete(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Post("/complex-tasks/{id}/run", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			task, ok, err := getComplexTask(id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "complex task not found"})
				return
			}
			result, err := r.RunComplexTask(task)
			delivery := webhook.DeliveryResult{Attempted: false}
			if err == nil {
				delivery = deliverWebhookFromConfig(req.Context(), webhookRepo, task.WebhookEnabled, task.WebhookConfigID, webhook.Envelope{
					Event:      "task.completed",
					SourceType: "complex_task",
					SourceID:   firstNonEmptyString(task.ID, id),
					SourceName: firstNonEmptyString(task.Name, "运营agent"),
					Timestamp:  time.Now().Format(time.RFC3339),
					Data:       complexTaskWebhookData(result),
				})
			}
			resp := map[string]interface{}{
				"ok":               err == nil,
				"task":             task,
				"steps":            result.Steps,
				"final_output":     result.FinalOutput,
				"started_at":       result.StartedAt,
				"finished_at":      result.FinishedAt,
				"webhook_delivery": delivery,
			}
			if err != nil {
				resp["error"] = err.Error()
			}
			writeJSON(w, http.StatusOK, resp)
		})

		apiR.Post("/complex-tasks", func(w http.ResponseWriter, req *http.Request) {
			var task storage.ComplexTask
			if err := json.NewDecoder(req.Body).Decode(&task); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			normalizeComplexTaskWebhookReference(&task)
			if task.ID == "" || task.Name == "" {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "id/name required"})
				return
			}
			if err := saveComplexTask(task); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Put("/complex-tasks/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			var task storage.ComplexTask
			if err := json.NewDecoder(req.Body).Decode(&task); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if task.ID == "" {
				task.ID = id
			}
			normalizeComplexTaskWebhookReference(&task)
			if task.ID != id {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "path id and body id mismatch"})
				return
			}
			if task.Name == "" {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "name required"})
				return
			}
			if err := saveComplexTask(task); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Delete("/complex-tasks/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if err := deleteComplexTask(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		// --- 飞连连接配置中心（最近启用清单，最多 3 条） ---
		apiR.Get("/connections", func(w http.ResponseWriter, req *http.Request) {
			cf, err := loadConnections()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			items := make([]map[string]interface{}, 0, len(cf.Items))
			for _, it := range cf.Items {
				items = append(items, map[string]interface{}{
					"id":            it.ID,
					"name":          it.Name,
					"scheme":        it.Scheme,
					"host":          it.Host,
					"port":          it.Port,
					"access_key_id": mask(it.AccessKeyID),
					"created_at":    it.CreatedAt,
					"active":        it.ID == cf.ActiveID,
				})
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"active_id": cf.ActiveID,
				"items":     items,
			})
		})

		apiR.Post("/connections", func(w http.ResponseWriter, req *http.Request) {
			var in map[string]interface{}
			if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}

			name, _ := in["name"].(string)
			scheme, _ := in["scheme"].(string)
			host, _ := in["host"].(string)
			portF, _ := in["port"].(float64)
			accessKey, _ := in["access_key"].(string)
			secretKey, _ := in["secret_key"].(string)
			baseURL, _ := in["base_url"].(string)

			clean := map[string]interface{}{}
			if scheme != "" {
				clean["scheme"] = scheme
			}
			if host != "" {
				clean["host"] = host
			}
			if portF > 0 {
				clean["port"] = int(portF)
			}
			if baseURL != "" {
				clean["base_url"] = baseURL
			}
			if accessKey != "" {
				clean["access_key"] = accessKey
			}
			// secret_key 允许为空（表示不覆盖）
			if secretKey != "" {
				clean["secret_key"] = secretKey
			} else {
				clean["secret_key"] = ""
			}

			if err := configStore.UpdateSealsuiteConnection(clean); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}

			// 在线热更新：重载 config + 重建 client + runner 切换
			if err := hotReloadRuntime(); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}

			id := "conn_" + time.Now().Format("20060102_150405")
			if name == "" {
				if host != "" {
					name = host
				} else if baseURL != "" {
					name = baseURL
				} else {
					name = id
				}
			}
			createdAt := time.Now().Format(time.RFC3339)
			connItem := storage.ConnectionItem{
				ID:          id,
				Name:        name,
				Scheme:      scheme,
				Host:        host,
				Port:        int(portF),
				AccessKeyID: accessKey,
				SecretRef:   "config",
				CreatedAt:   createdAt,
			}
			if err := connectionRepo.AddAndActivate(connItem); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "id": id})
		})

		apiR.Post("/connections/{id}/activate", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			it, ok, err := getConnectionFromRepo(connectionRepo, id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if !ok {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": fmt.Sprintf("connection not found: %s", id)})
				return
			}
			if err := connectionRepo.AddAndActivate(*it); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}

			// 写回 config.yaml（不覆盖 secret）
			clean := map[string]interface{}{
				"scheme":     it.Scheme,
				"host":       it.Host,
				"port":       it.Port,
				"access_key": it.AccessKeyID,
				"secret_key": "", // keep
			}
			if err := configStore.UpdateSealsuiteConnection(clean); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}

			if err := hotReloadRuntime(); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "active_id": id})
		})

		apiR.Delete("/connections/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			cf, err := loadConnections()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if cf.ActiveID == id {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "cannot delete active connection"})
				return
			}
			if err := connectionRepo.Delete(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		// --- 飞连连接信息（独立页面/接口） ---
		getConnection := func(w http.ResponseWriter, req *http.Request) {
			ss, err := configStore.GetSealsuiteConnection()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			// 脱敏 secret_key
			if _, ok := ss["secret_key"]; ok {
				ss["secret_key"] = "****"
			}
			writeJSON(w, http.StatusOK, ss)
		}
		apiR.Get("/connection", getConnection)
		apiR.Get("/settings/connections/current", getConnection)

		getLLM := func(w http.ResponseWriter, req *http.Request) {
			sec, err := configStore.GetLLMConfig()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			maskRoleConfig(sec, "planner")
			maskRoleConfig(sec, "formatter")
			writeJSON(w, http.StatusOK, sec)
		}
		apiR.Get("/llm/config", getLLM)
		apiR.Get("/settings/llm", getLLM)

		apiR.Get("/settings/llm/apis", func(w http.ResponseWriter, req *http.Request) {
			file, err := loadLLMAPIs()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			items := make([]map[string]interface{}, 0, len(file.Items))
			for _, item := range file.Items {
				items = append(items, maskLLMAPIItem(item, item.ID == file.ActiveID))
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"active_id": file.ActiveID,
				"items":     items,
			})
		})

		apiR.Post("/settings/llm/apis", func(w http.ResponseWriter, req *http.Request) {
			var in map[string]interface{}
			if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			id := strings.TrimSpace(firstNonEmptyString(in["id"], ""))
			name := strings.TrimSpace(firstNonEmptyString(in["name"], ""))
			activate := boolFromAny(in["activate"], false) || boolFromAny(in["active"], false)
			if id == "" || name == "" {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "id/name required"})
				return
			}

			item := storage.LLMAPIItem{
				ID:              id,
				Name:            name,
				Tags:            normalizeStringList(in["tags"]),
				Enabled:         boolFromAny(in["enabled"], true),
				Provider:        strings.TrimSpace(firstNonEmptyString(in["provider"], "")),
				BaseURL:         strings.TrimSpace(firstNonEmptyString(in["base_url"], "")),
				APIKey:          firstNonEmptyString(in["api_key"], ""),
				Model:           strings.TrimSpace(firstNonEmptyString(in["model"], "")),
				Timeout:         intFromAny(in["timeout"]),
				Temperature:     floatFromAny(in["temperature"]),
				MaxTokens:       intFromAny(in["max_tokens"]),
				Thinking:        boolFromAny(in["thinking"], false),
				ReasoningEffort: strings.TrimSpace(firstNonEmptyString(in["reasoning_effort"], "")),
				ResponseFormat:  mapFromAny(in["response_format"]),
				SystemPrompt:    firstNonEmptyString(in["system_prompt"], ""),
			}
			if err := llmAPIRepo.UpsertAndMaybeActivate(item, false); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}

			if activate {
				file, err := loadLLMAPIs()
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
					return
				}
				stored, ok := findLLMAPIItem(file, id)
				if !ok {
					writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "llm api not found"})
					return
				}
				if err := configStore.UpdateLLMConfig(mapSingleLLMAPIToRuntimePayload(*stored)); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
					return
				}
				if err := hotReloadRuntime(); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
					return
				}
				if err := llmAPIRepo.UpsertAndMaybeActivate(*stored, true); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
					return
				}
			}

			file, err := loadLLMAPIs()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			stored, ok := findLLMAPIItem(file, id)
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "llm api not found"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":     true,
				"id":     id,
				"active": file.ActiveID == id,
				"item":   maskLLMAPIItem(*stored, file.ActiveID == id),
			})
		})

		apiR.Post("/settings/llm/apis/{id}/activate", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			file, err := loadLLMAPIs()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			item, ok := findLLMAPIItem(file, id)
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "llm api not found"})
				return
			}
			if err := configStore.UpdateLLMConfig(mapSingleLLMAPIToRuntimePayload(*item)); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if err := hotReloadRuntime(); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if err := llmAPIRepo.UpsertAndMaybeActivate(*item, true); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "active_id": id})
		})

		apiR.Delete("/settings/llm/apis/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if err := llmAPIRepo.Delete(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		// --- feishu apis ---
		apiR.Get("/settings/feishu-apis", func(w http.ResponseWriter, req *http.Request) {
			file, err := loadFeishuAPIs()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			items := make([]map[string]interface{}, 0, len(file.Items))
			for _, item := range file.Items {
				masked := map[string]interface{}{
					"id":         item.ID,
					"name":       item.Name,
					"app_id":     item.AppID,
					"base_url":   item.BaseURL,
					"enabled":    item.Enabled,
					"created_at": item.CreatedAt,
				}
				items = append(items, masked)
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"active_id": file.ActiveID,
				"items":     items,
			})
		})

		apiR.Get("/settings/feishu-apis/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			item, ok, err := feishuAPIRepo.Get(id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "feishu api not found"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"id":         item.ID,
				"name":       item.Name,
				"app_id":     item.AppID,
				"base_url":   item.BaseURL,
				"enabled":    item.Enabled,
				"created_at": item.CreatedAt,
			})
		})

		apiR.Post("/settings/feishu-apis", func(w http.ResponseWriter, req *http.Request) {
			var in map[string]interface{}
			if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			id := strings.TrimSpace(firstNonEmptyString(in["id"], ""))
			name := strings.TrimSpace(firstNonEmptyString(in["name"], ""))
			if id == "" || name == "" {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "id/name required"})
				return
			}
			activate := boolFromAny(in["activate"], false) || boolFromAny(in["active"], false)

			item := storage.FeishuAPIItem{
				ID:        id,
				Name:      name,
				AppID:     strings.TrimSpace(firstNonEmptyString(in["app_id"], "")),
				AppSecret: firstNonEmptyString(in["app_secret"], ""),
				BaseURL:   strings.TrimSpace(firstNonEmptyString(in["base_url"], "")),
				Enabled:   boolFromAny(in["enabled"], true),
			}
			if item.BaseURL == "" {
				item.BaseURL = "https://open.feishu.cn"
			}
			if err := feishuAPIRepo.UpsertAndMaybeActivate(item, activate); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok": true,
				"id": id,
			})
		})

		apiR.Post("/settings/feishu-apis/{id}/activate", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if _, err := feishuAPIRepo.Activate(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "active_id": id})
		})

		apiR.Delete("/settings/feishu-apis/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if err := feishuAPIRepo.Delete(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Post("/settings/feishu-apis/test", func(w http.ResponseWriter, req *http.Request) {
			var in map[string]interface{}
			if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			appID := strings.TrimSpace(firstNonEmptyString(in["app_id"], ""))
			appSecret := firstNonEmptyString(in["app_secret"], "")
			baseURL := strings.TrimSpace(firstNonEmptyString(in["base_url"], ""))
			if appID == "" || appSecret == "" {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "app_id/app_secret required"})
				return
			}
			if baseURL == "" {
				baseURL = "https://open.feishu.cn"
			}

			// 获取 tenant_access_token 用于验证
			tokenURL := strings.TrimRight(baseURL, "/") + "/open-apis/auth/v3/tenant_access_token/internal"
			payload, _ := json.Marshal(map[string]string{
				"app_id":     appID,
				"app_secret": appSecret,
			})
			ctx, cancel := context.WithTimeout(req.Context(), 15*time.Second)
			defer cancel()
			httpReq, err := http.NewRequestWithContext(ctx, "POST", tokenURL, bytes.NewReader(payload))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			httpReq.Header.Set("Content-Type", "application/json")
			client := &http.Client{Timeout: 15 * time.Second}
			resp, err := client.Do(httpReq)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "failed to read response body"})
				return
			}
			var parsed map[string]interface{}
			if err := json.Unmarshal(body, &parsed); err != nil {
				parsed = map[string]interface{}{"raw": string(body)}
			}
			// 根据飞书返回的 code 判断凭证是否有效（code=0 表示成功）
			ok := resp.StatusCode >= 200 && resp.StatusCode < 300
			if codeVal, exists := parsed["code"]; exists {
				if code, isNumber := codeVal.(float64); isNumber {
					ok = ok && code == 0
				}
			}
			// 额外校验 token 字段是否存在，避免 code=0 但 token 为空的异常情况
			if tokenVal, exists := parsed["tenant_access_token"]; !exists || tokenVal == nil || tokenVal == "" {
				ok = false
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":          ok,
				"http_status": resp.StatusCode,
				"response":    parsed,
			})
		})

		apiR.Get("/settings/webhooks", func(w http.ResponseWriter, req *http.Request) {
			file, err := loadWebhooks()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			items := make([]map[string]interface{}, 0, len(file.Items))
			for _, item := range file.Items {
				items = append(items, webhookItemToMap(item))
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
		})

		apiR.Post("/settings/webhooks", func(w http.ResponseWriter, req *http.Request) {
			var in map[string]interface{}
			if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}

			item := storage.WebhookItem{
				ID:         strings.TrimSpace(firstNonEmptyString(in["id"], "")),
				Name:       strings.TrimSpace(firstNonEmptyString(in["name"], "")),
				Provider:   strings.TrimSpace(firstNonEmptyString(in["provider"], "")),
				URL:        strings.TrimSpace(firstNonEmptyString(in["url"], "")),
				Method:     normalizeHTTPRequestMethod(firstNonEmptyString(in["method"], "")),
				Headers:    stringMapFromAny(in["headers"]),
				AuthType:   strings.TrimSpace(firstNonEmptyString(in["auth_type"], "")),
				BodyTmpl:   firstNonEmptyString(in["body_template"], ""),
				TimeoutSec: intFromAny(in["timeout_sec"]),
				RetryCount: intFromAny(in["retry_count"]),
				Enabled:    boolFromAny(in["enabled"], true),
			}
			if item.ID == "" || item.Name == "" || item.URL == "" {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "id/name/url required"})
				return
			}
			if err := webhookRepo.Upsert(item); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "id": item.ID})
		})

		apiR.Delete("/settings/webhooks/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if err := webhookRepo.Delete(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Post("/settings/webhooks/test", func(w http.ResponseWriter, req *http.Request) {
			var in map[string]interface{}
			if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}

			targetURL := strings.TrimSpace(firstNonEmptyString(in["url"], ""))
			if targetURL == "" {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "url required"})
				return
			}

			method := normalizeHTTPRequestMethod(firstNonEmptyString(in["method"], "POST"))
			headers := stringMapFromAny(in["headers"])
			provider := webhook.NormalizeProvider(firstNonEmptyString(in["provider"], ""))
			payload := in["payload"]
			bodyTemplate := firstNonEmptyString(in["body_template"], "")
			// 飞书机器人专属：消息类型与加签密钥
			msgType := webhook.NormalizeFeishuMsgType(firstNonEmptyString(in["msg_type"], ""))
			secret := strings.TrimSpace(firstNonEmptyString(in["secret"], ""))
			timeoutSec := intFromAny(in["timeout_sec"])
			if timeoutSec <= 0 {
				timeoutSec = 10
			}

			bodyBytes, err := webhook.BuildRequestBodyForTest(&storage.WebhookItem{
				Provider: provider,
				BodyTmpl: bodyTemplate,
				MsgType:  msgType,
				Secret:   secret,
			}, payload)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			if len(bodyBytes) > 0 && !hasHeaderInsensitive(headers, "Content-Type") {
				headers["Content-Type"] = "application/json"
			}

			var bodyReader io.Reader
			if len(bodyBytes) > 0 {
				bodyReader = bytes.NewReader(bodyBytes)
			}

			httpReq, err := http.NewRequest(method, targetURL, bodyReader)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			for k, v := range headers {
				httpReq.Header.Set(k, v)
			}

			requestPreview := map[string]interface{}{
				"url":               targetURL,
				"method":            method,
				"provider":          provider,
				"msg_type":          msgType,
				"secret_configured": secret != "",
				"headers":           cloneStringMap(headers),
				"payload":           payload,
				"request_body":      string(bodyBytes),
			}

			client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
			resp, err := client.Do(httpReq)
			if err != nil {
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"ok":              false,
					"provider":        provider,
					"status_code":     0,
					"response_body":   "",
					"request_body":    string(bodyBytes),
					"request_preview": requestPreview,
					"error":           err.Error(),
				})
				return
			}
			defer resp.Body.Close()

			respBody, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"ok":              false,
					"provider":        provider,
					"status_code":     resp.StatusCode,
					"response_body":   "",
					"request_body":    string(bodyBytes),
					"request_preview": requestPreview,
					"error":           readErr.Error(),
				})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":              resp.StatusCode >= 200 && resp.StatusCode < 300,
				"provider":        provider,
				"status_code":     resp.StatusCode,
				"response_body":   string(respBody),
				"request_body":    string(bodyBytes),
				"request_preview": requestPreview,
			})
		})

		putLLM := func(w http.ResponseWriter, req *http.Request) {
			var in map[string]interface{}
			if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			clean := sanitizeLLMConfigPayload(in)
			if err := configStore.UpdateLLMConfig(clean); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if err := hotReloadRuntime(); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		}
		apiR.Put("/llm/config", putLLM)
		apiR.Put("/settings/llm", putLLM)

		postLLMTest := func(w http.ResponseWriter, req *http.Request) {
			var in map[string]interface{}
			if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			role := firstNonEmptyString(in["role"], "planner")
			roleCfg := resolveRoleConfigForTest(cfg.LLM, role, in)
			roleCfg.Enabled = true
			client := llm.NewClient(roleCfg)
			resp, err := client.Chat(llm.ChatRequest{
				Prompt:          "请回答：LLM 连接测试成功",
				SystemPrompt:    roleCfg.SystemPrompt,
				Input:           map[string]interface{}{"ping": "pong"},
				Thinking:        boolPtr(roleCfg.Thinking),
				ReasoningEffort: roleCfg.ReasoningEffort,
				ResponseFormat:  roleCfg.ResponseFormat,
			})
			out := map[string]interface{}{
				"ok":              err == nil,
				"role":            role,
				"provider":        roleCfg.Provider,
				"model":           roleCfg.Model,
				"content":         "",
				"content_preview": "",
			}
			if err != nil {
				out["error"] = err.Error()
			} else {
				out["content"] = resp.Content
				out["content_preview"] = mask(resp.Content)
			}
			writeJSON(w, http.StatusOK, out)
		}
		apiR.Post("/llm/test", postLLMTest)
		apiR.Post("/settings/llm/test", postLLMTest)

		putConnection := func(w http.ResponseWriter, req *http.Request) {
			var in map[string]interface{}
			if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			// 仅允许更新一组明确字段，避免写入垃圾
			allowed := map[string]bool{
				"scheme":     true,
				"host":       true,
				"port":       true,
				"base_url":   true,
				"access_key": true,
				"secret_key": true,
			}
			clean := map[string]interface{}{}
			for k, v := range in {
				if allowed[k] {
					clean[k] = v
				}
			}
			if err := configStore.UpdateSealsuiteConnection(clean); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}

			// 在线热更新：重载 config + 重建 client + runner 切换
			if err := hotReloadRuntime(); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		}
		apiR.Put("/connection", putConnection)
		apiR.Put("/settings/connections/current", putConnection)

		apiR.Post("/connection/test", func(w http.ResponseWriter, req *http.Request) {
			// 支持“未保存就测试”：直接使用请求体组装临时 client
			var in map[string]interface{}
			if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			tmp := cfg.SealSuite
			if v, ok := in["scheme"].(string); ok {
				tmp.Scheme = v
			}
			if v, ok := in["host"].(string); ok {
				tmp.Host = v
			}
			if v, ok := in["port"].(float64); ok { // JSON number
				tmp.Port = int(v)
			}
			if v, ok := in["base_url"].(string); ok {
				tmp.BaseURL = v
			}
			if v, ok := in["access_key"].(string); ok {
				tmp.AccessKey = v
			}
			if v, ok := in["secret_key"].(string); ok && v != "" {
				tmp.SecretKey = v
			}

			// 即使探测不再依赖模板，也在处理流程早期加载一次以暴露数据库问题
			if _, err := loadTemplates(); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}

			tmpClient := sealsuite.NewClient(&tmp)
			tmpClient.SetMockMode(false)

			// 1) 先显式获取 token（用于展示）
			token, exp, tokErr := tmpClient.FetchAccessToken()
			tokenOK := tokErr == nil && token != "" && exp > 0
			tokenPreview := ""
			if tokenOK {
				tokenPreview = mask(token)
			}

			// 2) probe：只有 tokenOK 才进行业务探测。
			// 探测目的：验证"token 有效 + 网络可达 + 飞连服务端可响应业务 API"。
			// 判定标准：
			//   · HTTP 状态码为 2xx 且响应中包含 `code` 字段 → 说明飞连服务端已接受 token 并返回业务响应，
			//     此时 `code` 非 0 只意味着"该 API 缺参数"（飞连对无 token 的请求会返回 HTTP 401，而不是 HTTP 200 + code:40000），
			//     因此不作为连接失败判定。
			//   · HTTP 4xx/5xx、网络超时、DNS 失败 → probe 失败。
			// 探测端点使用飞连资源清单 GET /api/open/v1/addr/management/list——无需额外参数即可返回 code 字段。
			probeOK := false
			var probeResult interface{} = nil
			var probeError string
			if tokenOK {
				status, raw, err := tmpClient.DoRaw(http.MethodGet, "/api/open/v1/addr/management/list", nil, nil)
				if len(raw) > 0 {
					probeResult = json.RawMessage(raw)
				}
				if err != nil {
					probeError = err.Error()
				} else if status < 200 || status >= 300 {
					// HTTP 层面非 2xx —— 通常是 token 失效（401）或端点不可用
					probeError = fmt.Sprintf("http status %d", status)
				} else {
					// HTTP 2xx：只要响应里能解析出 code 字段（无论值），就证明 token 有效、服务端正常
					var parsed map[string]interface{}
					if parseErr := json.Unmarshal(raw, &parsed); parseErr == nil {
						if _, hasCode := parsed["code"]; hasCode {
							probeOK = true
						} else {
							probeError = "unexpected response (missing code field)"
						}
					} else {
						probeError = "response is not valid JSON"
					}
				}
			} else if tokErr != nil {
				probeError = "skip probe because token failed"
			}

			resp := map[string]interface{}{
				"token_ok":              tokenOK,
				"token_preview":         tokenPreview,
				"token_expires_in":      exp,
				"token_request_preview": tokenRequestPreview(tmp, "temporary_form"),
				"probe_ok":              probeOK,
				"probe_result":          probeResult,
				"probe_error":           probeError,
			}
			if tokErr != nil {
				resp["token_error"] = tokErr.Error()
			}
			writeJSON(w, http.StatusOK, resp)
		})

		apiR.Post("/reload", func(w http.ResponseWriter, req *http.Request) {
			if err := r.Reload(); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Get("/jobs", func(w http.ResponseWriter, req *http.Request) {
			jf, tmap, smap, err := r.LoadSnapshot()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"jobs_file":  jf,
				"templates":  tmap,
				"job_status": smap,
			})
		})

		apiR.Get("/jobs/{name}", func(w http.ResponseWriter, req *http.Request) {
			name := chi.URLParam(req, "name")
			job, ok, err := legacyJobRepo.Get(name)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "job not found"})
				return
			}
			_, _, smap, _ := r.LoadSnapshot()
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"job": job,
				"run": smap[name],
			})
		})

		apiR.Post("/jobs", func(w http.ResponseWriter, req *http.Request) {
			var job storage.Job
			if err := json.NewDecoder(req.Body).Decode(&job); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if err := legacyJobRepo.Upsert(job); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Put("/jobs", func(w http.ResponseWriter, req *http.Request) {
			var jf storage.JobsFile
			if err := json.NewDecoder(req.Body).Decode(&jf); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if err := legacyJobRepo.Save(&jf); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Put("/jobs/{name}", func(w http.ResponseWriter, req *http.Request) {
			name := chi.URLParam(req, "name")
			var job storage.Job
			if err := json.NewDecoder(req.Body).Decode(&job); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if job.Name == "" {
				job.Name = name
			}
			if job.Name != name {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "path name and body name mismatch"})
				return
			}
			if err := legacyJobRepo.Upsert(job); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Delete("/jobs/{name}", func(w http.ResponseWriter, req *http.Request) {
			name := chi.URLParam(req, "name")
			if err := legacyJobRepo.Delete(name); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Post("/jobs/{name}/enable", func(w http.ResponseWriter, req *http.Request) {
			name := chi.URLParam(req, "name")
			jf, err := legacyJobRepo.Load()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			for i := range jf.Jobs {
				if jf.Jobs[i].Name == name {
					jf.Jobs[i].Enabled = true
				}
			}
			if err := legacyJobRepo.Save(jf); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Post("/jobs/{name}/disable", func(w http.ResponseWriter, req *http.Request) {
			name := chi.URLParam(req, "name")
			jf, err := legacyJobRepo.Load()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			for i := range jf.Jobs {
				if jf.Jobs[i].Name == name {
					jf.Jobs[i].Enabled = false
				}
			}
			if err := legacyJobRepo.Save(jf); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Post("/jobs/{name}/run", func(w http.ResponseWriter, req *http.Request) {
			name := chi.URLParam(req, "name")
			if err := r.RunOnce(name); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		// --- API 工具箱 ---
		apiR.Get("/api/templates", func(w http.ResponseWriter, req *http.Request) {
			tf, err := loadTemplates()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, tf)
		})

		apiR.Get("/api/templates/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			tpl, ok, err := getTemplate(id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "template not found"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"template": tpl})
		})

		apiR.Post("/api/templates", func(w http.ResponseWriter, req *http.Request) {
			var tpl storage.Template
			if err := json.NewDecoder(req.Body).Decode(&tpl); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if err := templateRepo.Upsert(tpl); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Put("/api/templates/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			var tpl storage.Template
			if err := json.NewDecoder(req.Body).Decode(&tpl); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if tpl.ID == "" {
				tpl.ID = id
			}
			if tpl.ID != id {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "path id and body id mismatch"})
				return
			}
			if err := templateRepo.Upsert(tpl); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Delete("/api/templates/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if refs, err := referencedLegacyJobsByTemplate(legacyJobRepo, id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			} else if len(refs) > 0 {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{
					"error":         "template is referenced by legacy jobs",
					"referenced_by": refs,
				})
				return
			}
			if err := templateRepo.Delete(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Put("/api/templates", func(w http.ResponseWriter, req *http.Request) {
			var tf storage.TemplatesFile
			if err := json.NewDecoder(req.Body).Decode(&tf); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if err := templateRepo.Save(&tf); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Post("/api/templates/{id}/test", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			var in struct {
				PathParams map[string]interface{} `json:"path_params"`
				Query      map[string]interface{} `json:"query"`
				Body       map[string]interface{} `json:"body"`
			}
			if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			out, err := r.Executor().Execute(api.ExecuteRequest{
				TemplateID: id,
				PathParams: in.PathParams,
				Query:      in.Query,
				Body:       in.Body,
			})
			if err != nil {
				writeErrorJSON(w, err, map[string]interface{}{
					"runtime_connection": connectionSummary(r.Config()),
				})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "result": out})
		})

		apiR.Post("/api/execute", func(w http.ResponseWriter, req *http.Request) {
			var inMap map[string]interface{}
			if err := json.NewDecoder(req.Body).Decode(&inMap); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}

			payloadBytes, err := json.Marshal(inMap)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}

			var in api.ExecuteRequest
			if err := json.Unmarshal(payloadBytes, &in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}

			pushOnce := boolFromAny(inMap["webhook_push_once"], false)
			webhookID := strings.TrimSpace(firstNonEmptyString(inMap["webhook_config_id"], ""))
			webhookEnabled := boolFromAny(inMap["webhook_enabled"], false)
			useTaskDraftPipeline := strings.TrimSpace(in.Mode) != "" || len(in.TransformConfig) > 0 || len(in.LLMConfig) > 0 || len(in.OutputConfig) > 0

			var out interface{}
			delivery := webhook.DeliveryResult{Attempted: false}
			if useTaskDraftPipeline {
				draft := storage.TaskDraft{
					ID:               firstNonEmptyString(inMap["draft_id"], "temporary_execute"),
					Name:             firstNonEmptyString(inMap["name"], "飞连任务"),
					Mode:             firstNonEmptyString(in.Mode, "api_only"),
					SourceTemplateID: in.TemplateID,
					InputConfig: map[string]interface{}{
						"template_id": in.TemplateID,
						"method":      in.Method,
						"path":        in.Path,
						"query":       in.Query,
						"path_params": in.PathParams,
						"body":        in.Body,
					},
					TransformConfig: cloneMapAny(in.TransformConfig),
					LLMConfig:       cloneMapAny(in.LLMConfig),
					OutputConfig:    cloneMapAny(in.OutputConfig),
					WebhookConfigID: webhookID,
					WebhookEnabled:  webhookEnabled,
				}
				out, delivery, err = r.ExecuteTransientTaskDraft(draft, pushOnce)
			} else {
				var execOut *api.ExecuteResponse
				execOut, err = r.Executor().Execute(in)
				out = execOut
				if err == nil && pushOnce && webhookEnabled && webhookID != "" {
					delivery = deliverWebhookFromConfig(req.Context(), webhookRepo, webhookEnabled, webhookID, webhook.Envelope{
						Event:      "task.completed",
						SourceType: "api_task",
						SourceID:   firstNonEmptyString(inMap["draft_id"], "temporary_execute"),
						SourceName: firstNonEmptyString(inMap["name"], "飞连API任务"),
						Timestamp:  time.Now().Format(time.RFC3339),
						Data:       out,
					})
				}
			}
			if err != nil {
				writeErrorJSON(w, err, map[string]interface{}{
					"runtime_connection": connectionSummary(r.Config()),
				})
				return
			}

			resp := map[string]interface{}{}
			if encoded, marshalErr := json.Marshal(out); marshalErr == nil {
				_ = json.Unmarshal(encoded, &resp)
			}
			if len(resp) == 0 {
				resp["result"] = out
			}
			resp["webhook_delivery"] = delivery
			writeJSON(w, http.StatusOK, resp)
		})

		apiR.Post("/api/preview", func(w http.ResponseWriter, req *http.Request) {
			var in api.PreviewRequest
			if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			out, err := r.Executor().Preview(in)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, out)
		})

		apiR.Get("/api/history", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"items": r.Executor().History(100)})
		})

		apiR.Post("/api/save-as-job", func(w http.ResponseWriter, req *http.Request) {
			var in struct {
				JobName string                 `json:"job_name"`
				Cron    string                 `json:"cron"`
				Type    string                 `json:"type"` // api_poll | api_write
				Params  map[string]interface{} `json:"params"`
			}
			if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if in.JobName == "" || in.Cron == "" || in.Type == "" {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "job_name/cron/type required"})
				return
			}
			jf, err := legacyJobRepo.Load()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			// 写入类任务：默认禁用
			enabled := true
			if in.Type == "api_write" {
				enabled = false
			}
			newJob := storage.Job{
				Name:    in.JobName,
				Enabled: enabled,
				Cron:    in.Cron,
				Type:    in.Type,
				Params:  in.Params,
			}
			replaced := false
			for i := range jf.Jobs {
				if jf.Jobs[i].Name == in.JobName {
					jf.Jobs[i] = newJob
					replaced = true
					break
				}
			}
			if !replaced {
				jf.Jobs = append(jf.Jobs, newJob)
			}
			if err := legacyJobRepo.Save(jf); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "enabled": enabled})
		})

		// --- Logs ---
		apiR.Get("/logs", func(w http.ResponseWriter, req *http.Request) {
			tailStr := req.URL.Query().Get("tail")
			tail, _ := strconv.Atoi(tailStr)
			if tail <= 0 {
				tail = 2000
			}
			text, err := readTail(cfg.Log.Filename, int64(tail))
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"text": text})
		})
		apiR.Get("/logs/executions", func(w http.ResponseWriter, req *http.Request) {
			if jobRunsRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "job runs repository is nil"})
				return
			}
			runs, err := jobRunsRepo.List(200)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			items := make([]map[string]interface{}, 0, len(runs))
			for _, item := range runs {
				targetType := item.TargetType
				targetID := item.TargetID
				if targetType == "" && item.SourceType == "job" {
					targetType = "legacy_job"
				}
				if targetID == "" {
					targetID = item.SourceID
				}
				lastRun := item.FinishedAt
				if lastRun == "" {
					lastRun = item.StartedAt
				}
				items = append(items, map[string]interface{}{
					"run_id":         item.ID,
					"source_type":    item.SourceType,
					"source_id":      item.SourceID,
					"target_type":    targetType,
					"target_id":      targetID,
					"status":         item.Status,
					"trigger_source": item.TriggerSource,
					"last_run":       lastRun,
					"ok":             item.Status == "success",
					"duration_ms":    item.DurationMS,
					"error":          item.ErrorMessage,
				})
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
		})

		// --- DLP 文件误报分析 ---
		apiR.Get("/dlp/events", func(w http.ResponseWriter, req *http.Request) {
			if dlpEventRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			limit := 50
			offset := 0
			if v := req.URL.Query().Get("limit"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					limit = n
				}
			}
			if v := req.URL.Query().Get("offset"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n >= 0 {
					offset = n
				}
			}
			filter := sqliteRepo.DLPEventFilter{
				Status:   req.URL.Query().Get("status"),
				Category: req.URL.Query().Get("category"),
				Exclude:  req.URL.Query().Get("exclude"),
			}
			items, total, err := dlpEventRepo.ListWithAnalysis(limit, offset, filter)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"items":  items,
				"total":  total,
				"limit":  limit,
				"offset": offset,
			})
		})

		apiR.Get("/dlp/analysis", func(w http.ResponseWriter, req *http.Request) {
			if dlpAnalysisRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			limit := 50
			offset := 0
			if v := req.URL.Query().Get("limit"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					limit = n
				}
			}
			if v := req.URL.Query().Get("offset"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n >= 0 {
					offset = n
				}
			}
			items, total, err := dlpAnalysisRepo.List(limit, offset)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}

			eventMap := make(map[string]storage.DLPEvent)
			for _, item := range items {
				if event, ok, _ := dlpEventRepo.Get(item.EventID); ok {
					eventMap[item.EventID] = event
				}
			}

			resultItems := make([]map[string]interface{}, 0, len(items))
			for _, item := range items {
				event := eventMap[item.EventID]
				resultItems = append(resultItems, map[string]interface{}{
					"id":                item.ID,
					"event_id":          item.EventID,
					"category":          item.Category,
					"should_exclude":    item.ShouldExclude,
					"confidence":        item.Confidence,
					"reasoning":         item.Reasoning,
					"analyzed_at":       item.AnalyzedAt,
					"whitelisted":       item.Whitelisted,
					"match_type":        item.MatchType,
					"match_value":       item.MatchValue,
					"file_info_name":    event.FileInfoName,
					"file_info_path":    event.FileInfoPath,
					"file_info_type":    event.FileInfoType,
					"leak_way_app_name": event.LeakWayAppName,
					"event_time":        event.EventTime,
					"user_name":         event.UserName,
				})
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"items":  resultItems,
				"total":  total,
				"limit":  limit,
				"offset": offset,
			})
		})

		apiR.Get("/dlp/analysis/{id}", func(w http.ResponseWriter, req *http.Request) {
			if dlpAnalysisRepo == nil || dlpEventRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			id := chi.URLParam(req, "id")
			result, ok, err := dlpAnalysisRepo.GetByEventID(id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "analysis not found"})
				return
			}
			event, _, _ := dlpEventRepo.Get(result.EventID)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"result": result,
				"event":  event,
			})
		})

		apiR.Post("/dlp/sync", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			var body struct {
				MaxItems int `json:"max_items"`
			}
			_ = json.NewDecoder(req.Body).Decode(&body)
			if body.MaxItems <= 0 {
				body.MaxItems = 100
			}
			result, err := r.SyncDLPEvents(body.MaxItems)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":           true,
				"synced_count": result.SyncedCount,
				"sync_at":      result.SyncAt,
			})
		})

		apiR.Post("/dlp/analyze", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			var body struct {
				MaxItems          int  `json:"max_items"`
				ReanalyzeRetained bool `json:"reanalyze_retained"`
			}
			_ = json.NewDecoder(req.Body).Decode(&body)
			if body.MaxItems <= 0 {
				body.MaxItems = 100
			}
			result, err := r.AnalyzeDLPEvents(body.MaxItems, body.ReanalyzeRetained)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":             true,
				"analyzed_count": result.AnalyzedCount,
				"excluded_count": result.ExcludedCount,
				"kept_count":     result.KeptCount,
				"analyzed_at":    result.AnalyzedAt,
			})
		})

		apiR.Post("/dlp/sync-and-analyze", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			var body struct {
				MaxItems int `json:"max_items"`
			}
			_ = json.NewDecoder(req.Body).Decode(&body)
			if body.MaxItems <= 0 {
				body.MaxItems = 100
			}
			syncResult, err := r.SyncDLPEvents(body.MaxItems)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			analyzeResult, err := r.AnalyzeDLPEvents(body.MaxItems, false)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":             true,
				"synced_count":   syncResult.SyncedCount,
				"analyzed_count": analyzeResult.AnalyzedCount,
				"excluded_count": analyzeResult.ExcludedCount,
				"kept_count":     analyzeResult.KeptCount,
			})
		})

		apiR.Get("/dlp/whitelist", func(w http.ResponseWriter, req *http.Request) {
			if dlpWhitelistRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			items, err := dlpWhitelistRepo.List()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
		})

		apiR.Post("/dlp/whitelist", func(w http.ResponseWriter, req *http.Request) {
			if dlpWhitelistRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			var body struct {
				ID          string `json:"id"`
				MatchType   string `json:"match_type"`
				MatchValue  string `json:"match_value"`
				Description string `json:"description"`
				EventID     string `json:"event_id"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if body.ID == "" {
				body.ID = "wl_" + fmt.Sprintf("%d", time.Now().UnixNano())
			}
			if body.MatchType == "" || body.MatchValue == "" {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "match_type and match_value are required"})
				return
			}
			item := storage.DLPWhitelistItem{
				ID:          body.ID,
				MatchType:   body.MatchType,
				MatchValue:  body.MatchValue,
				Description: body.Description,
			}
			if err := dlpWhitelistRepo.Upsert(item); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if dlpAnalysisRepo != nil && body.EventID != "" {
				ar, ok, _ := dlpAnalysisRepo.GetByEventID(body.EventID)
				if ok {
					dlpAnalysisRepo.MarkWhitelisted(ar.ID, true, body.MatchType, body.MatchValue)
				}
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "item": item})
		})

		apiR.Delete("/dlp/whitelist/{id}", func(w http.ResponseWriter, req *http.Request) {
			if dlpWhitelistRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			id := chi.URLParam(req, "id")
			if err := dlpWhitelistRepo.Delete(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Get("/dlp/state", func(w http.ResponseWriter, req *http.Request) {
			if dlpEventRepo == nil || dlpAnalysisRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			state, err := dlpEventRepo.GetState()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			summary, _ := dlpAnalysisRepo.Summary()
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"state":   state,
				"summary": summary,
			})
		})

		apiR.Post("/dlp/sync-schedule", func(w http.ResponseWriter, req *http.Request) {
			if dlpEventRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			var body struct {
				Enabled bool `json:"enabled"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if err := dlpEventRepo.UpdateSyncScheduleEnabled(body.Enabled); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "sync_schedule_enabled": body.Enabled})
		})

		apiR.Post("/dlp/analysis-schedule", func(w http.ResponseWriter, req *http.Request) {
			if dlpEventRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			var body struct {
				Enabled bool `json:"enabled"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if err := dlpEventRepo.UpdateAnalysisScheduleEnabled(body.Enabled); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "analysis_schedule_enabled": body.Enabled})
		})

		// --- 重复设备删除 ---

		// 统计概览
		apiR.Get("/dup-devices/summary", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			r.EnsureDupRepos()
			summary, err := r.GetDupDeviceSummary()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, summary)
		})

		// 分组列表
		apiR.Get("/dup-devices/groups", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			r.EnsureDupRepos()

			limit := 20
			offset := 0
			if v := req.URL.Query().Get("limit"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					limit = n
				}
			}
			if v := req.URL.Query().Get("offset"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n >= 0 {
					offset = n
				}
			}
			status := req.URL.Query().Get("status")
			matchLevel := req.URL.Query().Get("match_level")
			distinct := req.URL.Query().Get("distinct") == "true"

			var items []storage.DupDeviceGroup
			var total int
			var err error
			if distinct {
				items, total, err = r.ListDupDeviceGroupsWithDistinct(status, matchLevel, limit, offset, true)
			} else {
				items, total, err = r.ListDupDeviceGroups(status, matchLevel, limit, offset)
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"items":    items,
				"total":    total,
				"limit":    limit,
				"offset":   offset,
				"distinct": distinct,
			})
		})

		// 分组详情
		apiR.Get("/dup-devices/groups/{id}", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			r.EnsureDupRepos()
			groupID := chi.URLParam(req, "id")

			group, members, err := r.GetDupDeviceGroupDetail(groupID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"group":   group,
				"members": members,
			})
		})

		// 分组成员列表
		apiR.Get("/dup-devices/groups/{id}/members", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			r.EnsureDupRepos()
			groupID := chi.URLParam(req, "id")

			members, err := r.ListDupDeviceGroupMembers(groupID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"items": members,
			})
		})

		// 手动处理分组
		apiR.Post("/dup-devices/groups/{id}/resolve", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			r.EnsureDupRepos()
			groupID := chi.URLParam(req, "id")

			var body struct {
				RetainedDID string `json:"retained_did"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid request body"})
				return
			}
			if body.RetainedDID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "retained_did is required"})
				return
			}

			result, err := r.ManualResolveDupGroup(groupID, body.RetainedDID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":            true,
				"cleaned_count": result.CleanedCount,
				"failed_count":  result.FailedCount,
				"cleanup_at":    result.CleanupAt,
			})
		})

		// 清理日志
		apiR.Get("/dup-devices/cleanup-logs", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			r.EnsureDupRepos()

			limit := 50
			offset := 0
			if v := req.URL.Query().Get("limit"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					limit = n
				}
			}
			if v := req.URL.Query().Get("offset"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n >= 0 {
					offset = n
				}
			}
			status := req.URL.Query().Get("status")

			items, total, err := r.ListDupDeviceCleanupLogs(status, limit, offset)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"items":  items,
				"total":  total,
				"limit":  limit,
				"offset": offset,
			})
		})

		// 任务状态
		apiR.Get("/dup-devices/task-state", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			r.EnsureDupRepos()
			state, err := r.GetDupDeviceTaskState()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, state)
		})

		// 立即同步
		apiR.Post("/dup-devices/sync", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			r.EnsureDupRepos()

			go func() {
				_, _ = r.SyncDupDevices()
			}()

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"message": "sync started",
			})
		})

		// 立即检测
		apiR.Post("/dup-devices/detect", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			r.EnsureDupRepos()

			go func() {
				_, _ = r.DetectDupDevices()
				_, _ = r.AutoCleanupHighMatchDups()
			}()

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"message": "detect started",
			})
		})

		// 立即执行完整流程
		apiR.Post("/dup-devices/sync-and-detect", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			r.EnsureDupRepos()

			go func() {
				_, _ = r.RunDupDeviceFullProcess()
			}()

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"message": "full process started",
			})
		})

		// 立即删除（重试失败的删除任务）
		apiR.Post("/dup-devices/retry-cleanup", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			r.EnsureDupRepos()

			go func() {
				_, _ = r.RetryFailedCleanup()
			}()

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"message": "retry cleanup started",
			})
		})

		apiR.Post("/dup-devices/sync-schedule", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			r.EnsureDupRepos()
			var body struct {
				Enabled bool `json:"enabled"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if err := r.UpdateDupDeviceSyncScheduleEnabled(body.Enabled); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "sync_enabled": body.Enabled})
		})

		apiR.Post("/dup-devices/detect-schedule", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			var body struct {
				Enabled bool `json:"enabled"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if err := r.UpdateDupDeviceDetectScheduleEnabled(body.Enabled); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "detect_enabled": body.Enabled})
		})

		// --- ZTNA 访问控制策略推荐 ---

		// 状态总览
		apiR.Get("/ztna/state", func(w http.ResponseWriter, req *http.Request) {
			if ztnaLogRepo == nil || ztnaStatsRepo == nil || ztnaAnalysisRepo == nil || ztnaTaskStateRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			totalLogs, _ := ztnaLogRepo.Count()
			_, totalStats := ztnaStatsRepo.Count()
			totalUsers, _ := ztnaStatsRepo.UserCount()
			totalResources, _ := ztnaStatsRepo.ResourceCount()
			summary, _ := ztnaAnalysisRepo.Summary()
			taskState, _ := ztnaTaskStateRepo.Get()

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"total_logs":       totalLogs,
				"total_stats":      totalStats,
				"total_users":      totalUsers,
				"total_resources":  totalResources,
				"analysis_summary": summary,
				"task_state":       taskState,
			})
		})

		// 分析结果列表
		apiR.Get("/ztna/analysis", func(w http.ResponseWriter, req *http.Request) {
			if ztnaAnalysisRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			limit := 50
			offset := 0
			if v := req.URL.Query().Get("limit"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					limit = n
				}
			}
			if v := req.URL.Query().Get("offset"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n >= 0 {
					offset = n
				}
			}
			filter := storage.ZTNAStatsFilter{
				Category: req.URL.Query().Get("category"),
				UserID:   req.URL.Query().Get("user_id"),
				DestIP:   req.URL.Query().Get("dest_ip"),
			}
			items, total, err := ztnaAnalysisRepo.List(limit, offset, filter)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"items":  items,
				"total":  total,
				"limit":  limit,
				"offset": offset,
			})
		})

		// 分析结果详情
		apiR.Get("/ztna/analysis/{id}", func(w http.ResponseWriter, req *http.Request) {
			if ztnaAnalysisRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			id := chi.URLParam(req, "id")
			item, found, err := ztnaAnalysisRepo.Get(id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if !found {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "not found"})
				return
			}
			writeJSON(w, http.StatusOK, item)
		})

		// 标记已审核
		apiR.Post("/ztna/analysis/{id}/review", func(w http.ResponseWriter, req *http.Request) {
			if ztnaAnalysisRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			id := chi.URLParam(req, "id")
			var body struct {
				ReviewedBy string `json:"reviewed_by"`
				Comment    string `json:"comment"`
				ShouldKeep bool   `json:"should_keep"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if err := ztnaAnalysisRepo.MarkReviewed(id, body.ReviewedBy, body.Comment); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		// 立即同步分析
		apiR.Post("/ztna/sync-and-analyze", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			var body struct {
				MaxItems int `json:"max_items"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if body.MaxItems <= 0 {
				body.MaxItems = 10000
			}

			go func() {
				logger.Info("[ZTNA API] 后台执行立即同步分析")
				_, _ = r.RunZTNASyncAndAnalyze(body.MaxItems)
			}()

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"message": "sync and analyze started",
			})
		})

		// 仅同步日志
		apiR.Post("/ztna/sync", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			var body struct {
				MaxItems int `json:"max_items"`
			}
			_ = json.NewDecoder(req.Body).Decode(&body)

			go func() {
				_, _ = r.SyncZTNALogs(body.MaxItems)
			}()

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"message": "sync started",
			})
		})

		// 仅构建统计
		apiR.Post("/ztna/build-stats", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}

			go func() {
				_, _ = r.BuildZTNAAccessStats()
			}()

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"message": "build stats started",
			})
		})

		// 仅运行分析
		apiR.Post("/ztna/analyze", func(w http.ResponseWriter, req *http.Request) {
			if r == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "runner not initialized"})
				return
			}
			var body struct {
				MaxItems     int  `json:"max_items"`
				ReanalyzeAll bool `json:"reanalyze_all"`
			}
			_ = json.NewDecoder(req.Body).Decode(&body)

			go func() {
				_, _ = r.AnalyzeZTNAPolicies(body.MaxItems, body.ReanalyzeAll)
			}()

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      true,
				"message": "analysis started",
			})
		})

		// 更新同步定时开关
		apiR.Post("/ztna/sync-schedule", func(w http.ResponseWriter, req *http.Request) {
			if ztnaTaskStateRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			var body struct {
				Enabled bool `json:"enabled"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if err := ztnaTaskStateRepo.UpdateSyncScheduleEnabled(body.Enabled); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "sync_enabled": body.Enabled})
		})

		// 更新统计定时开关
		apiR.Post("/ztna/stats-schedule", func(w http.ResponseWriter, req *http.Request) {
			if ztnaTaskStateRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			var body struct {
				Enabled bool `json:"enabled"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if err := ztnaTaskStateRepo.UpdateStatsScheduleEnabled(body.Enabled); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "stats_enabled": body.Enabled})
		})

		// 更新分析定时开关
		apiR.Post("/ztna/analysis-schedule", func(w http.ResponseWriter, req *http.Request) {
			if ztnaTaskStateRepo == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "repository is nil"})
				return
			}
			var body struct {
				Enabled bool `json:"enabled"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if err := ztnaTaskStateRepo.UpdateAnalysisScheduleEnabled(body.Enabled); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "analysis_enabled": body.Enabled})
		})

		// --- 小插件：访客 Wi-Fi 申请 ---
		// 透传到飞连 OpenAPI：
		//   POST /api/open/v1/wifi/guest/apply   基于基本信息申请访客 Wi-Fi 账号（系统生成账号）
		//   POST /api/open/v1/wifi/guest/create  批量创建访客账号（自行指定用户名/密码）
		// 前端按文档组装请求体，后端原样透传并把飞连原始响应返回给页面展示。
		apiR.Post("/guest-wifi/apply", proxyGuestWifiRequest(cfg, configStore, guestWifiApplyPath))
		apiR.Post("/guest-wifi/create", proxyGuestWifiRequest(cfg, configStore, guestWifiCreatePath))

		// --- 小插件：自动化审批 ---
		// Webhook 端点：接收飞书设备申报事件回调。
		// 飞书要求 3 秒内返回，因此先响应再用 goroutine 异步处理。
		apiR.Post("/approval/webhook", func(w http.ResponseWriter, req *http.Request) {
			bodyBytes, err := io.ReadAll(req.Body)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "read body failed"})
				return
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &payload); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			// 飞书 URL 校验
			if typ, _ := payload["type"].(string); typ == "url_verification" {
				challenge, _ := payload["challenge"].(string)
				writeJSON(w, http.StatusOK, map[string]interface{}{"challenge": challenge})
				return
			}
			// 事件回调：立即响应，异步处理
			writeJSON(w, http.StatusOK, map[string]interface{}{"code": 0})
			if r != nil {
				go func() {
					defer func() {
						if rec := recover(); rec != nil {
							fmt.Printf("[approval webhook] panic recovered: %v\n", rec)
						}
					}()
					_ = r.ProcessDeviceApplyEvent(context.Background(), payload)
				}()
			}
		})

		// 获取审批配置
		apiR.Get("/approval/config", func(w http.ResponseWriter, req *http.Request) {
			if approvalConfigRepo == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"error": "db not ready"})
				return
			}
			cfg, err := approvalConfigRepo.Get()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, cfg)
		})

		// 保存审批配置
		apiR.Put("/approval/config", func(w http.ResponseWriter, req *http.Request) {
			if approvalConfigRepo == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"error": "db not ready"})
				return
			}
			var cfg storage.ApprovalConfig
			if err := json.NewDecoder(req.Body).Decode(&cfg); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json"})
				return
			}
			if err := approvalConfigRepo.Save(cfg); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		// 列出审批任务（可选 ?status=pending 过滤）
		apiR.Get("/approval/tasks", func(w http.ResponseWriter, req *http.Request) {
			if approvalTaskRepo == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"error": "db not ready"})
				return
			}
			status := req.URL.Query().Get("status")
			tasks, err := approvalTaskRepo.List(status)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"items": tasks})
		})

		// 手动通过
		apiR.Post("/approval/tasks/{id}/approve", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if r == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"error": "runner not ready"})
				return
			}
			if err := r.ApproveTask(req.Context(), id); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		// 手动驳回
		apiR.Post("/approval/tasks/{id}/reject", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if r == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"error": "runner not ready"})
				return
			}
			if err := r.RejectTask(id); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})
	})

	return rr, nil
}

func mask(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "…" + s[len(s)-3:]
}

func jobStatusPayload(status *runner.JobStatus) map[string]interface{} {
	if status == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"last_run":    status.LastRun,
		"last_ok":     status.LastOK,
		"last_error":  status.LastError,
		"duration_ms": status.DurationMs,
		"next_run":    status.NextRun,
	}
}

func latestScheduleRunLogs(repo *sqliteRepo.JobRunsRepository) (map[string]service.RunLog, error) {
	if repo == nil {
		return map[string]service.RunLog{}, nil
	}
	runs, err := repo.List(0)
	if err != nil {
		return nil, err
	}
	out := make(map[string]service.RunLog, len(runs))
	for _, item := range runs {
		if strings.TrimSpace(item.SourceType) != "job_schedule" {
			continue
		}
		scheduleID := strings.TrimSpace(item.SourceID)
		if scheduleID == "" {
			continue
		}
		if _, exists := out[scheduleID]; exists {
			continue
		}
		out[scheduleID] = item
	}
	return out, nil
}

func buildExternalIPSyncTaskSummary(task storage.ExternalIPSyncTask) map[string]interface{} {
	task = storage.NormalizeExternalIPSyncTask(task)
	resourceLabel := strings.TrimSpace(task.ResourceID)
	if name := strings.TrimSpace(task.ResourceTagNames); name != "" {
		if resourceLabel != "" {
			resourceLabel = fmt.Sprintf("%s (%s)", name, resourceLabel)
		} else {
			resourceLabel = name
		}
	}
	return map[string]interface{}{
		"task_id":            task.ID,
		"source_type":        task.SourceType,
		"source_label":       externalIPSyncSourceLabel(task.SourceType),
		"source_url":         task.SourceURL,
		"ip_version":         task.IPVersion,
		"ip_version_label":   externalIPSyncIPVersionLabel(task.IPVersion),
		"resource_id":        task.ResourceID,
		"resource_tag_names": task.ResourceTagNames,
		"resource_label":     resourceLabel,
		"write_action":       task.WriteAction,
		"write_api_path":     firstNonEmptyString(task.FeilianAPIPath, "/api/open/v1/addr/management/add"),
		"dry_run":            task.DryRun,
		"skip_when_empty":    task.SkipWhenEmpty,
	}
}

func externalIPSyncSourceLabel(sourceType string) string {
	switch strings.TrimSpace(sourceType) {
	case "", "google_ip_ranges":
		return "Google IP Ranges"
	default:
		return sourceType
	}
}

func externalIPSyncIPVersionLabel(version string) string {
	switch strings.TrimSpace(strings.ToLower(version)) {
	case "ipv6":
		return "IPv6"
	case "all":
		return "IPv4 + IPv6"
	case "", "ipv4":
		return "IPv4"
	default:
		return version
	}
}

func externalIPSyncRunSummaryFromRunLog(run service.RunLog) map[string]interface{} {
	targetType := strings.TrimSpace(run.TargetType)
	if targetType != "" && targetType != "external_ip_sync" {
		return nil
	}
	summary := service.ExternalIPSyncExecutionSummary{
		Status:       strings.TrimSpace(run.Status),
		ErrorMessage: strings.TrimSpace(run.ErrorMessage),
	}
	raw := strings.TrimSpace(run.ResultJSON)
	if raw != "" && raw != "null" {
		_ = json.Unmarshal([]byte(raw), &summary)
	}
	return map[string]interface{}{
		"task_id":        summary.TaskID,
		"resource_id":    summary.ResourceID,
		"ip_version":     summary.IPVersion,
		"source_total":   summary.SourceTotal,
		"filtered_total": summary.FilteredTotal,
		"existing_total": summary.ExistingTotal,
		"to_add_total":   summary.ToAddTotal,
		"added_total":    summary.AddedTotal,
		"write_api_path": firstNonEmptyString(summary.WriteAPIPath, "/api/open/v1/addr/management/add"),
		"dry_run":        summary.DryRun,
		"status":         firstNonEmptyString(summary.Status, strings.TrimSpace(run.Status)),
		"error_message":  firstNonEmptyString(summary.ErrorMessage, strings.TrimSpace(run.ErrorMessage)),
	}
}

func mergeStringAnyMap(base map[string]interface{}, overrides map[string]interface{}) map[string]interface{} {
	out := cloneMapAny(base)
	for k, v := range overrides {
		out[k] = v
	}
	return out
}

func buildBaseURL(ss config.SealSuiteConfig) string {
	baseURL := strings.TrimSpace(ss.BaseURL)
	if ss.Host != "" {
		scheme := ss.Scheme
		if scheme == "" {
			scheme = "https"
		}
		// 默认端口（https=443, http=80）必须省略，避免 Host header 含 ":443"/":80"
		isDefaultPort := (scheme == "https" && ss.Port == 443) || (scheme == "http" && ss.Port == 80)
		if ss.Port > 0 && !isDefaultPort {
			baseURL = fmt.Sprintf("%s://%s:%d", scheme, ss.Host, ss.Port)
		} else {
			baseURL = fmt.Sprintf("%s://%s", scheme, ss.Host)
		}
	}
	return baseURL
}

func connectionSummary(cfg *config.Config) map[string]interface{} {
	if cfg == nil {
		return map[string]interface{}{}
	}
	ss := cfg.SealSuite
	baseURL := buildBaseURL(ss)
	return map[string]interface{}{
		"scheme":      ss.Scheme,
		"host":        ss.Host,
		"port":        ss.Port,
		"base_url":    baseURL,
		"config_mode": "saved_runtime",
		"access_key":  mask(ss.AccessKey),
	}
}

func tokenRequestPreview(ss config.SealSuiteConfig, source string) map[string]interface{} {
	baseURL := buildBaseURL(ss)
	return map[string]interface{}{
		"token_url":         fmt.Sprintf("%s%s", baseURL, "/api/open/v1/token"),
		"content_type":      "application/json;charset=utf-8",
		"body_keys":         []string{"access_key_id", "access_key_secret"},
		"access_key_masked": mask(ss.AccessKey),
		"access_key_len":    len(strings.TrimSpace(ss.AccessKey)),
		"secret_key_len":    len(strings.TrimSpace(ss.SecretKey)),
		"source":            source,
	}
}

func maskRoleConfig(root map[string]interface{}, role string) {
	if root == nil {
		return
	}
	item, _ := root[role].(map[string]interface{})
	if item == nil {
		return
	}
	if _, ok := item["api_key"]; ok {
		item["api_key"] = "****"
	}
	root[role] = item
}

func maskRoleMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return map[string]interface{}{}
	}
	out := cloneMapAny(in)
	if _, ok := out["api_key"]; ok {
		out["api_key"] = "****"
	}
	return out
}

func cloneMapAny(in map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range in {
		child, ok := v.(map[string]interface{})
		if ok {
			out[k] = cloneMapAny(child)
			continue
		}
		out[k] = v
	}
	return out
}

func maskLLMAPIItem(item storage.LLMAPIItem, active bool) map[string]interface{} {
	out := map[string]interface{}{
		"id":               item.ID,
		"name":             item.Name,
		"tags":             append([]string(nil), item.Tags...),
		"enabled":          item.Enabled,
		"provider":         item.Provider,
		"base_url":         item.BaseURL,
		"api_key":          item.APIKey,
		"model":            item.Model,
		"timeout":          item.Timeout,
		"temperature":      item.Temperature,
		"max_tokens":       item.MaxTokens,
		"thinking":         item.Thinking,
		"reasoning_effort": item.ReasoningEffort,
		"system_prompt":    item.SystemPrompt,
		"created_at":       item.CreatedAt,
		"active":           active,
	}
	if _, ok := out["api_key"]; ok {
		out["api_key"] = "****"
	}
	if item.ResponseFormat != nil {
		out["response_format"] = cloneMapAny(item.ResponseFormat)
	}
	return out
}

func mapSingleLLMAPIToRuntimePayload(item storage.LLMAPIItem) map[string]interface{} {
	role := map[string]interface{}{
		"enabled":          item.Enabled,
		"provider":         item.Provider,
		"base_url":         item.BaseURL,
		"api_key":          item.APIKey,
		"model":            item.Model,
		"timeout":          item.Timeout,
		"temperature":      item.Temperature,
		"max_tokens":       item.MaxTokens,
		"thinking":         item.Thinking,
		"reasoning_effort": item.ReasoningEffort,
		"system_prompt":    item.SystemPrompt,
	}
	if item.ResponseFormat != nil {
		role["response_format"] = cloneMapAny(item.ResponseFormat)
	}
	assignPlanner, assignFormatter := resolveLLMAPIActivationRoles(item.Tags)
	out := map[string]interface{}{}
	if assignPlanner {
		out["planner"] = cloneMapAny(role)
	}
	if assignFormatter {
		out["formatter"] = cloneMapAny(role)
	}
	return out
}

func resolveLLMAPIActivationRoles(tags []string) (bool, bool) {
	hasPlanner := false
	hasFormatter := false
	for _, tag := range tags {
		switch strings.ToLower(strings.TrimSpace(tag)) {
		case "planner":
			hasPlanner = true
		case "formatter":
			hasFormatter = true
		}
	}
	switch {
	case hasPlanner && hasFormatter:
		return true, true
	case hasPlanner:
		return true, false
	case hasFormatter:
		return false, true
	default:
		return true, true
	}
}

func findLLMAPIItem(file *storage.LLMAPIFile, id string) (*storage.LLMAPIItem, bool) {
	if file == nil {
		return nil, false
	}
	id = strings.TrimSpace(id)
	for _, item := range file.Items {
		if item.ID == id {
			cp := item
			cp.Tags = append([]string(nil), item.Tags...)
			if item.ResponseFormat != nil {
				cp.ResponseFormat = cloneMapAny(item.ResponseFormat)
			}
			return &cp, true
		}
	}
	return nil, false
}

func sanitizeLLMConfigPayload(in map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	if v, ok := in["mock_mode"]; ok {
		out["mock_mode"] = v
	}
	for _, role := range []string{"planner", "formatter"} {
		roleIn, _ := in[role].(map[string]interface{})
		if roleIn == nil {
			continue
		}
		clean := map[string]interface{}{}
		for _, key := range []string{"enabled", "provider", "base_url", "api_key", "model", "timeout", "system_prompt", "temperature", "max_tokens", "thinking", "reasoning_effort", "response_format"} {
			if v, ok := roleIn[key]; ok {
				clean[key] = v
			}
		}
		out[role] = clean
	}
	return out
}

func resolveRoleConfigForTest(llmCfg config.LLMConfig, role string, in map[string]interface{}) config.LLMRoleConfig {
	var cfgOut config.LLMRoleConfig
	if role == "planner" {
		cfgOut = llmCfg.Planner
	} else {
		cfgOut = llmCfg.Formatter
	}
	for _, field := range []string{"provider", "base_url", "api_key", "model", "system_prompt", "reasoning_effort"} {
		if v, ok := in[field].(string); ok && v != "" {
			switch field {
			case "provider":
				cfgOut.Provider = v
			case "base_url":
				cfgOut.BaseURL = v
			case "api_key":
				cfgOut.APIKey = v
			case "model":
				cfgOut.Model = v
			case "system_prompt":
				cfgOut.SystemPrompt = v
			case "reasoning_effort":
				cfgOut.ReasoningEffort = v
			}
		}
	}
	if v, ok := in["enabled"].(bool); ok {
		cfgOut.Enabled = v
	}
	if v, ok := in["thinking"].(bool); ok {
		cfgOut.Thinking = v
	}
	if v, ok := in["timeout"].(float64); ok {
		cfgOut.Timeout = int(v)
	}
	if v, ok := in["temperature"].(float64); ok {
		cfgOut.Temperature = v
	}
	if v, ok := in["max_tokens"].(float64); ok {
		cfgOut.MaxTokens = int(v)
	}
	if v, ok := in["response_format"].(map[string]interface{}); ok {
		cfgOut.ResponseFormat = v
	}
	return cfgOut
}

func firstNonEmptyString(v interface{}, fallback string) string {
	s, _ := v.(string)
	if s == "" {
		return fallback
	}
	return s
}

func normalizeStringList(v interface{}) []string {
	raw := []string{}
	switch vv := v.(type) {
	case []string:
		raw = append(raw, vv...)
	case []interface{}:
		for _, item := range vv {
			if s, ok := item.(string); ok {
				raw = append(raw, s)
			}
		}
	case string:
		for _, part := range strings.Split(vv, ",") {
			raw = append(raw, part)
		}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s := strings.TrimSpace(item)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func boolFromAny(v interface{}, fallback bool) bool {
	switch vv := v.(type) {
	case bool:
		return vv
	case string:
		if vv == "" {
			return fallback
		}
		return strings.EqualFold(vv, "true")
	default:
		return fallback
	}
}

func intFromAny(v interface{}) int {
	switch vv := v.(type) {
	case int:
		return vv
	case int64:
		return int(vv)
	case float64:
		return int(vv)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(vv))
		if err == nil {
			return n
		}
	}
	return 0
}

func floatFromAny(v interface{}) float64 {
	switch vv := v.(type) {
	case float64:
		return vv
	case float32:
		return float64(vv)
	case int:
		return float64(vv)
	case int64:
		return float64(vv)
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(vv), 64)
		if err == nil {
			return n
		}
	}
	return 0
}

func mapFromAny(v interface{}) map[string]interface{} {
	out, _ := v.(map[string]interface{})
	if out == nil {
		return nil
	}
	return cloneMapAny(out)
}

func stringMapFromAny(v interface{}) map[string]string {
	out := map[string]string{}
	switch vv := v.(type) {
	case map[string]string:
		for k, val := range vv {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			out[key] = strings.TrimSpace(val)
		}
	case map[string]interface{}:
		for k, val := range vv {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			s, ok := val.(string)
			if !ok {
				continue
			}
			out[key] = strings.TrimSpace(s)
		}
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func normalizeHTTPRequestMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return http.MethodPost
	}
	return method
}

func loadConnectionsFromRepo(repo *sqliteRepo.ConnectionsRepository) (*storage.ConnectionsFile, error) {
	if repo == nil {
		return nil, fmt.Errorf("connections repository is nil")
	}
	return repo.Load()
}

func loadLLMAPIsFromRepo(repo *sqliteRepo.LLMAPIRepository) (*storage.LLMAPIFile, error) {
	if repo == nil {
		return nil, fmt.Errorf("llm api repository is nil")
	}
	return repo.Load()
}

func loadWebhooksFromRepo(repo *sqliteRepo.WebhookRepository) (*storage.WebhookFile, error) {
	if repo == nil {
		return nil, fmt.Errorf("webhook repository is nil")
	}
	return repo.Load()
}

func loadTemplatesFromRepo(repo *sqliteRepo.TemplateRepository) (*storage.TemplatesFile, error) {
	if repo == nil {
		return nil, fmt.Errorf("template repository is nil")
	}
	return repo.Load()
}

func loadOutputTemplatesFromRepo(repo *sqliteRepo.OutputTemplateRepository) (*storage.OutputTemplatesFile, error) {
	if repo == nil {
		return nil, fmt.Errorf("output template repository is nil")
	}
	return repo.Load()
}

func loadComplexTasksFromRepo(repo *sqliteRepo.ComplexTaskRepository) (*storage.ComplexTasksFile, error) {
	if repo == nil {
		return nil, fmt.Errorf("complex task repository is nil")
	}
	items, err := repo.List()
	if err != nil {
		return nil, err
	}
	return &storage.ComplexTasksFile{Version: 1, Items: items}, nil
}

func getConnectionFromRepo(repo *sqliteRepo.ConnectionsRepository, id string) (*storage.ConnectionItem, bool, error) {
	if repo == nil {
		return nil, false, fmt.Errorf("connections repository is nil")
	}
	return repo.Get(id)
}

func getTemplateFromRepo(repo *sqliteRepo.TemplateRepository, id string) (*storage.Template, bool, error) {
	if repo == nil {
		return nil, false, fmt.Errorf("template repository is nil")
	}
	return repo.Get(id)
}

func getOutputTemplateFromRepo(repo *sqliteRepo.OutputTemplateRepository, id string) (*storage.OutputTemplate, bool, error) {
	if repo == nil {
		return nil, false, fmt.Errorf("output template repository is nil")
	}
	return repo.Get(id)
}

func getComplexTaskFromRepo(repo *sqliteRepo.ComplexTaskRepository, id string) (storage.ComplexTask, bool, error) {
	if repo == nil {
		return storage.ComplexTask{}, false, fmt.Errorf("complex task repository is nil")
	}
	return repo.Get(id)
}

func getWebhookFromRepo(repo *sqliteRepo.WebhookRepository, id string) (*storage.WebhookItem, bool, error) {
	if repo == nil {
		return nil, false, fmt.Errorf("webhook repository is nil")
	}
	return repo.Get(id)
}

func hasHeaderInsensitive(headers map[string]string, target string) bool {
	for key := range headers {
		if strings.EqualFold(key, target) {
			return true
		}
	}
	return false
}

func webhookItemToMap(item storage.WebhookItem) map[string]interface{} {
	return map[string]interface{}{
		"id":            item.ID,
		"name":          item.Name,
		"provider":      item.Provider,
		"url":           item.URL,
		"method":        item.Method,
		"headers":       cloneStringMap(item.Headers),
		"auth_type":     item.AuthType,
		"body_template": item.BodyTmpl,
		"timeout_sec":   item.TimeoutSec,
		"retry_count":   item.RetryCount,
		"enabled":       item.Enabled,
		"created_at":    item.CreatedAt,
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func logWebhookDelivery(sourceType, sourceID string, delivery webhook.DeliveryResult) {
	log.Printf("webhook delivery source=%s source_id=%s attempted=%v ok=%v status=%d err=%s",
		sourceType, sourceID, delivery.Attempted, delivery.OK, delivery.StatusCode, delivery.Error)
}

func deliverWebhookFromConfig(ctx context.Context, repo *sqliteRepo.WebhookRepository, enabled bool, webhookID string, env webhook.Envelope) webhook.DeliveryResult {
	webhookID, enabled = normalizeWebhookReference(webhookID, enabled)
	if ctx == nil {
		ctx = context.Background()
	}
	delivery := webhook.DeliveryResult{Attempted: false}
	switch {
	case !enabled || webhookID == "":
	case repo == nil:
		delivery = webhook.DeliveryResult{Attempted: false, Error: "webhook repository is nil"}
	default:
		item, ok, err := getWebhookFromRepo(repo, webhookID)
		switch {
		case err != nil:
			delivery = webhook.DeliveryResult{Attempted: false, Error: err.Error()}
		case !ok:
			delivery = webhook.DeliveryResult{Attempted: false, Error: fmt.Sprintf("webhook config not found: %s", webhookID)}
		default:
			delivery = webhook.DeliverWebhookForSuccess(ctx, item, env)
		}
	}
	logWebhookDelivery(env.SourceType, env.SourceID, delivery)
	return delivery
}

func complexTaskWebhookData(result *runner.ComplexTaskRunResult) map[string]interface{} {
	if result == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"steps":        result.Steps,
		"final_output": result.FinalOutput,
		"started_at":   result.StartedAt,
		"finished_at":  result.FinishedAt,
	}
}

func normalizeWebhookReference(id string, enabled bool) (string, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", false
	}
	return id, enabled
}

func normalizeTaskDraftWebhookReference(draft *storage.TaskDraft) {
	if draft == nil {
		return
	}
	draft.WebhookConfigID, draft.WebhookEnabled = normalizeWebhookReference(draft.WebhookConfigID, draft.WebhookEnabled)
}

func normalizeJobScheduleWebhookReference(sched *storage.JobSchedule) {
	if sched == nil {
		return
	}
	sched.WebhookConfigID, sched.WebhookEnabled = normalizeWebhookReference(sched.WebhookConfigID, sched.WebhookEnabled)
}

func normalizeComplexTaskWebhookReference(task *storage.ComplexTask) {
	if task == nil {
		return
	}
	task.WebhookConfigID, task.WebhookEnabled = normalizeWebhookReference(task.WebhookConfigID, task.WebhookEnabled)
}

func normalizeScheduleTarget(s *storage.JobSchedule) {
	if s == nil {
		return
	}
	storage.NormalizeJobScheduleDefaults(s)
}

func validateScheduleTarget(s storage.JobSchedule, drafts repository.TaskDraftRepository, tasks repository.ComplexTaskRepository, externalTasks repository.ExternalIPSyncTaskRepository) error {
	if s.TargetType == "" || s.TargetID == "" {
		return fmt.Errorf("target_type/target_id required")
	}
	switch s.TargetType {
	case "task_draft":
		if _, ok, err := drafts.Get(s.TargetID); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("task draft not found")
		}
	case "complex_task":
		if _, ok, err := tasks.Get(s.TargetID); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("complex task not found")
		}
	case "external_ip_sync":
		if externalTasks == nil {
			return fmt.Errorf("external ip sync task repository is nil")
		}
		if _, ok, err := externalTasks.Get(s.TargetID); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("external ip sync task not found")
		}
	default:
		return fmt.Errorf("unsupported target_type: %s", s.TargetType)
	}
	return nil
}

func referencedLegacyJobsByTemplate(repo repository.LegacyJobRepository, templateID string) ([]string, error) {
	if repo == nil {
		return nil, fmt.Errorf("legacy job repository is nil")
	}
	return repo.ReferencedByTemplate(templateID)
}

func referencedSchedulesByDraft(store repository.ScheduleRepository, draftID string) ([]string, error) {
	items, err := store.List()
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, item := range items {
		if item.TargetType == "task_draft" && item.TargetID == draftID {
			refs = append(refs, item.ID)
			continue
		}
		if item.DraftID == draftID {
			refs = append(refs, item.ID)
		}
	}
	return refs, nil
}

func referencedSchedulesByTarget(store repository.ScheduleRepository, targetType, targetID, excludeID string) ([]string, error) {
	items, err := store.List()
	if err != nil {
		return nil, err
	}
	targetType = strings.TrimSpace(targetType)
	targetID = strings.TrimSpace(targetID)
	excludeID = strings.TrimSpace(excludeID)
	var refs []string
	for _, item := range items {
		if excludeID != "" && strings.TrimSpace(item.ID) == excludeID {
			continue
		}
		normalizedType, normalizedID := runner.NormalizeTarget(item.TargetType, item.TargetID, item.DraftID)
		if strings.TrimSpace(normalizedType) == targetType && strings.TrimSpace(normalizedID) == targetID {
			refs = append(refs, item.ID)
		}
	}
	return refs, nil
}

func loadExternalIPSyncResourceItems(cfg *config.Config, store storage.ConfigStore, loadTemplates func() (*storage.TemplatesFile, error)) ([]map[string]interface{}, string, error) {
	sealSuiteCfg := currentSealSuiteConfig(cfg, store)
	client := sealsuite.NewClient(&sealSuiteCfg)
	client.SetMockMode(false)

	if loadTemplates != nil {
		if tf, err := loadTemplates(); err == nil {
			if templateID, templates := selectExternalIPSyncResourceTemplate(tf); templateID != "" {
				exec := api.NewExecutor(client, templates)
				out, err := exec.Execute(api.ExecuteRequest{TemplateID: templateID})
				if err == nil && out != nil {
					items, decodeErr := decodeExternalIPSyncResourceItems(out.Body)
					if decodeErr == nil && len(items) > 0 {
						return items, "template:" + templateID, nil
					}
					if decodeErr != nil {
						return nil, "", decodeErr
					}
				}
			}
		}
	}

	var lastErr error
	for _, candidate := range []string{
		"/api/open/v1/addr/management/list",
		"/api/open/v1/addr/management/page",
	} {
		_, body, err := client.DoRaw(http.MethodGet, candidate, nil, nil)
		if err != nil {
			lastErr = err
			continue
		}
		items, err := decodeExternalIPSyncResourceItems(body)
		if err != nil {
			lastErr = err
			continue
		}
		if len(items) > 0 {
			return items, "feilian_api:" + candidate, nil
		}
		lastErr = fmt.Errorf("external ip sync resources: empty response from %s", candidate)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("external ip sync resources: no available source")
	}
	return nil, "", lastErr
}

func currentSealSuiteConfig(cfg *config.Config, store storage.ConfigStore) config.SealSuiteConfig {
	if cfg == nil {
		return config.SealSuiteConfig{}
	}
	out := cfg.SealSuite
	ss, err := store.GetSealsuiteConnection()
	if err != nil {
		return out
	}
	if v := firstNonEmptyString(ss["base_url"], ""); v != "" {
		out.BaseURL = v
	}
	if v := firstNonEmptyString(ss["scheme"], ""); v != "" {
		out.Scheme = v
	}
	if v := firstNonEmptyString(ss["host"], ""); v != "" {
		out.Host = v
	}
	if v := intFromAny(ss["port"]); v > 0 {
		out.Port = v
	}
	if v := firstNonEmptyString(ss["access_key"], ""); v != "" {
		out.AccessKey = v
	}
	if v := firstNonEmptyString(ss["secret_key"], ""); v != "" {
		out.SecretKey = v
	}
	return out
}

// 访客 Wi-Fi 申请相关的飞连 OpenAPI 路径白名单。
const (
	guestWifiApplyPath  = "/api/open/v1/wifi/guest/apply"  // 基于基本信息申请访客 Wi-Fi 账号
	guestWifiCreatePath = "/api/open/v1/wifi/guest/create" // 批量创建访客账号
)

// proxyGuestWifiRequest 返回一个 HTTP Handler，用于把访客 Wi-Fi 申请请求透传给指定的飞连 OpenAPI 路径。
// 设计为白名单透传：upstreamPath 只能是上面两个访客接口常量之一，避免该端点被当作开放代理使用。
//
// 处理流程：
//  1. 解析前端提交的 JSON 请求体；
//  2. 使用当前激活的飞连连接配置构建客户端（自动获取/缓存 access_token）；
//  3. 通过 DoRaw 发起 POST，并把飞连原始 JSON 响应（含 code/message/data）与 HTTP 状态码原样回传；
//  4. 传输层错误（网络、token 获取失败等）统一返回 502，业务错误（code!=0）仍透传给前端展示。
func proxyGuestWifiRequest(cfg *config.Config, store storage.ConfigStore, upstreamPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		// 请求体即飞连接口所需参数，使用 map 透传以同时兼容两种接口的字段差异。
		var payload map[string]interface{}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json body"})
			return
		}

		sealSuiteCfg := currentSealSuiteConfig(cfg, store)
		client := sealsuite.NewClient(&sealSuiteCfg)
		client.SetMockMode(false)

		// 落一条出站报文日志，便于核对 send_sms / mobile / account_count 等关键字段是否真的推给了飞连。
		// DoRaw 只记录 has_body，不记录 body 内容；访客短信“成功但未收到”时需要据此排查。
		if bodyBytes, marshalErr := json.Marshal(payload); marshalErr == nil {
			logger.Info("[GuestWiFi] upstream request",
				zap.String("upstream_path", upstreamPath),
				zap.String("body", string(bodyBytes)),
			)
		}

		status, raw, err := client.DoRaw(http.MethodPost, upstreamPath, nil, payload)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]interface{}{"error": err.Error()})
			return
		}
		if len(raw) == 0 {
			writeJSON(w, http.StatusBadGateway, map[string]interface{}{"error": "empty response from feilian"})
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if status > 0 {
			w.WriteHeader(status)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		_, _ = w.Write(raw)
	}
}

func selectExternalIPSyncResourceTemplate(tf *storage.TemplatesFile) (string, map[string]storage.Template) {
	if tf == nil {
		return "", nil
	}
	templates := make(map[string]storage.Template, len(tf.Templates))
	for _, item := range tf.Templates {
		templates[item.ID] = item
	}
	for _, candidatePath := range []string{
		"/api/open/v1/addr/management/list",
		"/api/open/v1/addr/management/page",
	} {
		for _, item := range tf.Templates {
			if strings.EqualFold(strings.TrimSpace(item.Method), http.MethodGet) && strings.TrimSpace(item.Path) == candidatePath {
				return item.ID, templates
			}
		}
	}
	return "", templates
}

func decodeExternalIPSyncResourceItems(raw []byte) ([]map[string]interface{}, error) {
	var payload interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode external ip sync resources response: %w", err)
	}
	return collectExternalIPSyncResourceItems(payload), nil
}

func collectExternalIPSyncResourceItems(payload interface{}) []map[string]interface{} {
	seen := map[string]map[string]interface{}{}
	var walk func(v interface{})
	walk = func(v interface{}) {
		switch vv := v.(type) {
		case map[string]interface{}:
			if item, ok := normalizeExternalIPSyncResourceItem(vv); ok {
				seen[item["resource_id"].(string)] = item
			}
			for _, child := range vv {
				walk(child)
			}
		case []interface{}:
			for _, child := range vv {
				walk(child)
			}
		}
	}
	walk(payload)
	out := make([]map[string]interface{}, 0, len(seen))
	for _, item := range seen {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return firstNonEmptyString(out[i]["label"], firstNonEmptyString(out[i]["resource_id"], "")) <
			firstNonEmptyString(out[j]["label"], firstNonEmptyString(out[j]["resource_id"], ""))
	})
	return out
}

func normalizeExternalIPSyncResourceItem(item map[string]interface{}) (map[string]interface{}, bool) {
	if item == nil {
		return nil, false
	}
	resourceID := strings.TrimSpace(firstNonEmptyString(item["resource_id"],
		firstNonEmptyString(item["id"], firstNonEmptyString(item["value"], ""))))
	if resourceID == "" {
		return nil, false
	}
	name := strings.TrimSpace(firstNonEmptyString(item["resource_name"],
		firstNonEmptyString(item["name"],
			firstNonEmptyString(item["label"], firstNonEmptyString(item["resource_name_snapshot"], "")))))
	tagNames := strings.TrimSpace(firstNonEmptyString(item["resource_tag_names"],
		firstNonEmptyString(item["tag_names"], "")))
	label := resourceID
	if name != "" {
		label = fmt.Sprintf("%s · %s", name, resourceID)
	}
	return map[string]interface{}{
		"id":                 resourceID,
		"value":              resourceID,
		"resource_id":        resourceID,
		"name":               name,
		"label":              label,
		"resource_tag_names": tagNames,
		"tag_names":          tagNames,
	}, true
}

func externalIPSyncResourceItemsFromTasks(tasks []storage.ExternalIPSyncTask) []map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(tasks))
	for _, task := range tasks {
		if item, ok := normalizeExternalIPSyncResourceItem(map[string]interface{}{
			"resource_id":        task.ResourceID,
			"resource_tag_names": task.ResourceTagNames,
		}); ok {
			items = append(items, item)
		}
	}
	return mergeExternalIPSyncResourceItems(nil, items)
}

func mergeExternalIPSyncResourceItems(base, overrides []map[string]interface{}) []map[string]interface{} {
	seen := map[string]map[string]interface{}{}
	for _, item := range base {
		if normalized, ok := normalizeExternalIPSyncResourceItem(item); ok {
			seen[normalized["resource_id"].(string)] = normalized
		}
	}
	for _, item := range overrides {
		if normalized, ok := normalizeExternalIPSyncResourceItem(item); ok {
			seen[normalized["resource_id"].(string)] = normalized
		}
	}
	out := make([]map[string]interface{}, 0, len(seen))
	for _, item := range seen {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return firstNonEmptyString(out[i]["label"], firstNonEmptyString(out[i]["resource_id"], "")) <
			firstNonEmptyString(out[j]["label"], firstNonEmptyString(out[j]["resource_id"], ""))
	})
	return out
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErrorJSON(w http.ResponseWriter, err error, extraFields map[string]interface{}) {
	statusCode := http.StatusInternalServerError
	if storage.IsValidationError(err) {
		statusCode = http.StatusBadRequest
	}
	body := make(map[string]interface{}, len(extraFields)+1)
	body["error"] = err.Error()
	for k, v := range extraFields {
		body[k] = v
	}
	writeJSON(w, statusCode, body)
}

func readTail(filename string, n int64) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := st.Size()
	start := size - n
	if start < 0 {
		start = 0
	}
	_, err = f.Seek(start, io.SeekStart)
	if err != nil {
		return "", err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
