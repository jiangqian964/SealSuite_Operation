package web

import (
	"bytes"
	"context"
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
	"sealsuite-operation/internal/llm"
	"sealsuite-operation/internal/runner"
	"sealsuite-operation/internal/sealsuite"
	"sealsuite-operation/internal/storage"
	"sealsuite-operation/internal/web/assets"
	"sealsuite-operation/internal/webhook"

	"github.com/go-chi/chi/v5"
)

func NewServer(cfg *config.Config, r *runner.Runner) (*http.Server, error) {
	router, err := NewRouter(cfg, r)
	if err != nil {
		return nil, err
	}
	addr := cfg.Server.Bind + ":" + strconv.Itoa(cfg.Server.Port)
	return &http.Server{
		Addr:    addr,
		Handler: router,
	}, nil
}

func NewRouter(cfg *config.Config, r *runner.Runner) (http.Handler, error) {
	rr := chi.NewRouter()
	jobsStore := storage.JobsStore{Path: "jobs.yaml"}
	templatesStore := storage.TemplatesStore{Path: "api-templates.yaml"}
	outputTemplatesStore := storage.OutputTemplatesStore{Path: "output-templates.yaml"}
	configStore := storage.ConfigStore{Path: "config.yaml"}
	connectionsStore := storage.ConnectionsStore{Path: "connections.yaml"}
	llmAPIStore := storage.NewLLMAPIStore("llm-apis.yaml")
	webhookStore := storage.NewWebhookStore("webhooks.yaml")
	taskDraftsStore := storage.TaskDraftsStore{Path: "task-drafts.yaml"}
	jobSchedulesStore := storage.JobSchedulesStore{Path: "job-schedules.yaml"}
	complexTasksStore := storage.ComplexTasksStore{Path: "complex-tasks.yaml"}
	jobRunsStore := storage.JobRunsStore{Path: "job-runs.json"}

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
			tf, err := taskDraftsStore.Load()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, tf)
		})

		apiR.Get("/task-drafts/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			draft, ok, err := taskDraftsStore.Get(id)
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
			if err := taskDraftsStore.Upsert(draft); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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
			if err := taskDraftsStore.Upsert(draft); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Delete("/task-drafts/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if refs, err := referencedSchedulesByDraft(jobSchedulesStore, id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			} else if len(refs) > 0 {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{
					"error":         "task draft is referenced by schedules",
					"referenced_by": refs,
				})
				return
			}
			if err := taskDraftsStore.Delete(id); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"version":         jf.Version,
				"items":           jf.Items,
				"drafts":          drafts,
				"schedule_status": runs,
			})
		})

		apiR.Get("/job-schedules/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			sched, ok, err := jobSchedulesStore.Get(id)
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
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"schedule": sched,
				"draft":    drafts[targetID],
				"target":   targetSummary,
				"run":      runs[id],
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
			if err := validateScheduleTarget(sched, taskDraftsStore, complexTasksStore); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			if err := jobSchedulesStore.Upsert(sched); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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
			if err := validateScheduleTarget(sched, taskDraftsStore, complexTasksStore); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			if err := jobSchedulesStore.Upsert(sched); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Delete("/job-schedules/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if err := jobSchedulesStore.Delete(id); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Post("/job-schedules/{id}/run", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			delivery, err := r.RunScheduleOnce(req.Context(), id)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":               true,
				"webhook_delivery": delivery,
			})
		})

		// --- complex tasks: 工作流编排与未来 Agent 扩展 ---
		apiR.Get("/complex-tasks", func(w http.ResponseWriter, req *http.Request) {
			cf, err := complexTasksStore.Load()
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
				delivery = deliverWebhookFromConfig(req.Context(), webhookStore, task.WebhookEnabled, task.WebhookConfigID, webhook.Envelope{
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
			task, ok, err := complexTasksStore.Get(id)
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
			tf, err := outputTemplatesStore.Load()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, tf)
		})

		apiR.Get("/output-templates/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			item, ok, err := outputTemplatesStore.Get(id)
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
			if err := outputTemplatesStore.Upsert(item); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Delete("/output-templates/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if err := outputTemplatesStore.Delete(id); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Post("/complex-tasks/{id}/run", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			task, ok, err := complexTasksStore.Get(id)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "complex task not found"})
				return
			}
			result, err := r.RunComplexTask(*task)
			delivery := webhook.DeliveryResult{Attempted: false}
			if err == nil {
				delivery = deliverWebhookFromConfig(req.Context(), webhookStore, task.WebhookEnabled, task.WebhookConfigID, webhook.Envelope{
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
			if err := complexTasksStore.Upsert(task); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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
			if err := complexTasksStore.Upsert(task); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Delete("/complex-tasks/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if err := complexTasksStore.Delete(id); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		// --- connections.yaml：最近启用清单（最多 3 条） ---
		apiR.Get("/connections", func(w http.ResponseWriter, req *http.Request) {
			cf, err := connectionsStore.Load()
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
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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
			_ = connectionsStore.AddAndActivate(storage.ConnectionItem{
				ID:          id,
				Name:        name,
				Scheme:      scheme,
				Host:        host,
				Port:        int(portF),
				AccessKeyID: accessKey,
				SecretRef:   "config",
				CreatedAt:   createdAt,
			})

			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "id": id})
		})

		apiR.Post("/connections/{id}/activate", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			it, err := connectionsStore.Activate(id)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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
			cf, err := connectionsStore.Load()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if cf.ActiveID == id {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "cannot delete active connection"})
				return
			}
			if err := connectionsStore.Delete(id); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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
			file, err := llmAPIStore.Load()
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
			if err := llmAPIStore.UpsertAndMaybeActivate(item, false); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}

			if activate {
				file, err := llmAPIStore.Load()
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
					writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
					return
				}
				if err := hotReloadRuntime(); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
					return
				}
				if _, err := llmAPIStore.Activate(id); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
					return
				}
			}

			file, err := llmAPIStore.Load()
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
			file, err := llmAPIStore.Load()
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
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			if err := hotReloadRuntime(); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if _, err := llmAPIStore.Activate(id); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "active_id": id})
		})

		apiR.Delete("/settings/llm/apis/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if err := llmAPIStore.Delete(id); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Get("/settings/webhooks", func(w http.ResponseWriter, req *http.Request) {
			file, err := webhookStore.Load()
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
			if err := webhookStore.Upsert(item); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "id": item.ID})
		})

		apiR.Delete("/settings/webhooks/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if err := webhookStore.Delete(id); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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
			timeoutSec := intFromAny(in["timeout_sec"])
			if timeoutSec <= 0 {
				timeoutSec = 10
			}

			bodyBytes, err := webhook.BuildRequestBodyForTest(&storage.WebhookItem{
				Provider: provider,
				BodyTmpl: bodyTemplate,
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
				"url":          targetURL,
				"method":       method,
				"provider":     provider,
				"headers":      cloneStringMap(headers),
				"payload":      payload,
				"request_body": string(bodyBytes),
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
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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
				Prompt:       "请回答：LLM 连接测试成功",
				SystemPrompt: roleCfg.SystemPrompt,
				Input:        map[string]interface{}{"ping": "pong"},
				Thinking:     boolPtr(roleCfg.Thinking),
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
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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

			tf, err := templatesStore.Load()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			tmap := map[string]storage.Template{}
			for _, t := range tf.Templates {
				tmap[t.ID] = t
			}

			tmpClient := sealsuite.NewClient(&tmp)
			tmpClient.SetMockMode(false)
			exec := api.NewExecutor(tmpClient, tmap)

			// 1) 先显式获取 token（用于展示）
			token, exp, tokErr := tmpClient.FetchAccessToken()
			tokenOK := tokErr == nil && token != "" && exp > 0
			tokenPreview := ""
			if tokenOK {
				tokenPreview = mask(token)
			}

			// 2) probe：只有 tokenOK 才进行业务探测
			probeOK := false
			var probeResult interface{} = nil
			var probeError string
			if tokenOK {
				// 优先用 users_list；如果模板文件不包含该模板，则使用任意一个已有模板做探测，
				// 避免落到硬编码的 /api/v1/users（该路径在飞连环境中可能不存在，从而造成“误报失败”）。
				probeTplID := ""
				if _, ok := tmap["users_list"]; ok {
					probeTplID = "users_list"
				} else if len(tmap) > 0 {
					ids := make([]string, 0, len(tmap))
					for id := range tmap {
						ids = append(ids, id)
					}
					sort.Strings(ids)
					probeTplID = ids[0]
				}

				if probeTplID == "" {
					probeError = "skip probe because no templates available"
				} else {
					out, err := exec.Execute(api.ExecuteRequest{TemplateID: probeTplID})
					if err != nil {
						probeError = err.Error()
					} else {
						probeOK = true
						probeResult = out
					}
				}
			} else if tokErr != nil {
				probeError = "skip probe because token failed"
			}

			resp := map[string]interface{}{
				"token_ok":         tokenOK,
				"token_preview":    tokenPreview,
				"token_expires_in": exp,
				"token_request_preview": tokenRequestPreview(tmp, "temporary_form"),
				"probe_ok":         probeOK,
				"probe_result":     probeResult,
				"probe_error":      probeError,
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
			job, ok, err := jobsStore.Get(name)
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
			if err := jobsStore.Upsert(job); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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
			if err := jobsStore.Save(&jf); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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
			if err := jobsStore.Upsert(job); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Delete("/jobs/{name}", func(w http.ResponseWriter, req *http.Request) {
			name := chi.URLParam(req, "name")
			if err := jobsStore.Delete(name); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Post("/jobs/{name}/enable", func(w http.ResponseWriter, req *http.Request) {
			name := chi.URLParam(req, "name")
			jf, err := jobsStore.Load()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			for i := range jf.Jobs {
				if jf.Jobs[i].Name == name {
					jf.Jobs[i].Enabled = true
				}
			}
			if err := jobsStore.Save(jf); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Post("/jobs/{name}/disable", func(w http.ResponseWriter, req *http.Request) {
			name := chi.URLParam(req, "name")
			jf, err := jobsStore.Load()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			for i := range jf.Jobs {
				if jf.Jobs[i].Name == name {
					jf.Jobs[i].Enabled = false
				}
			}
			if err := jobsStore.Save(jf); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Post("/jobs/{name}/run", func(w http.ResponseWriter, req *http.Request) {
			name := chi.URLParam(req, "name")
			if err := r.RunOnce(name); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		// --- API 工具箱 ---
		apiR.Get("/api/templates", func(w http.ResponseWriter, req *http.Request) {
			tf, err := templatesStore.Load()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, tf)
		})

		apiR.Get("/api/templates/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			tpl, ok, err := templatesStore.Get(id)
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
			if err := templatesStore.Upsert(tpl); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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
			if err := templatesStore.Upsert(tpl); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Delete("/api/templates/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			if refs, err := referencedJobsByTemplate(jobsStore, id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			} else if len(refs) > 0 {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{
					"error":         "template is referenced by jobs",
					"referenced_by": refs,
				})
				return
			}
			if err := templatesStore.Delete(id); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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
			if err := templatesStore.Save(&tf); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			_ = r.Reload()
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		})

		apiR.Post("/api/templates/{id}/test", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			var in struct {
				PathParams map[string]string      `json:"path_params"`
				Query      map[string]string      `json:"query"`
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
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{
					"error":              err.Error(),
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
					ID:              firstNonEmptyString(inMap["draft_id"], "temporary_execute"),
					Name:            firstNonEmptyString(inMap["name"], "飞连任务"),
					Mode:            firstNonEmptyString(in.Mode, "api_only"),
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
					delivery = deliverWebhookFromConfig(req.Context(), webhookStore, webhookEnabled, webhookID, webhook.Envelope{
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
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{
					"error":              err.Error(),
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
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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
			jf, err := jobsStore.Load()
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
			if err := jobsStore.Save(jf); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
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
			jf, err := jobRunsStore.Load()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			items := make([]map[string]interface{}, 0, len(jf.Items))
			for key, item := range jf.Items {
				targetType := "legacy_job"
				targetID := key
				if strings.HasPrefix(key, "schedule:") {
					targetType = "scheduled_task"
					targetID = strings.TrimPrefix(key, "schedule:")
				}
				items = append(items, map[string]interface{}{
					"run_id":       key,
					"target_type":  targetType,
					"target_id":    targetID,
					"last_run":     item.LastRun,
					"ok":           item.OK,
					"duration_ms":  item.DurationMs,
					"error":        item.Error,
				})
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
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

func connectionSummary(cfg *config.Config) map[string]interface{} {
	if cfg == nil {
		return map[string]interface{}{}
	}
	ss := cfg.SealSuite
	baseURL := strings.TrimSpace(ss.BaseURL)
	if ss.Host != "" {
		scheme := ss.Scheme
		if scheme == "" {
			scheme = "https"
		}
		if ss.Port > 0 {
			baseURL = fmt.Sprintf("%s://%s:%d", scheme, ss.Host, ss.Port)
		} else {
			baseURL = fmt.Sprintf("%s://%s", scheme, ss.Host)
		}
	}
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
	baseURL := strings.TrimSpace(ss.BaseURL)
	if ss.Host != "" {
		scheme := ss.Scheme
		if scheme == "" {
			scheme = "https"
		}
		if ss.Port > 0 {
			baseURL = fmt.Sprintf("%s://%s:%d", scheme, ss.Host, ss.Port)
		} else {
			baseURL = fmt.Sprintf("%s://%s", scheme, ss.Host)
		}
	}
	return map[string]interface{}{
		"token_url":        fmt.Sprintf("%s%s", baseURL, "/api/open/v1/token"),
		"content_type":     "application/json;charset=utf-8",
		"body_keys":        []string{"access_key_id", "access_key_secret"},
		"access_key_masked": mask(ss.AccessKey),
		"access_key_len":   len(strings.TrimSpace(ss.AccessKey)),
		"secret_key_len":   len(strings.TrimSpace(ss.SecretKey)),
		"source":           source,
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

func deliverWebhookFromConfig(ctx context.Context, store *storage.WebhookStore, enabled bool, webhookID string, env webhook.Envelope) webhook.DeliveryResult {
	webhookID, enabled = normalizeWebhookReference(webhookID, enabled)
	if ctx == nil {
		ctx = context.Background()
	}
	delivery := webhook.DeliveryResult{Attempted: false}
	switch {
	case !enabled || webhookID == "":
	case store == nil:
		delivery = webhook.DeliveryResult{Attempted: false, Error: "webhook store is nil"}
	default:
		item, ok, err := store.Get(webhookID)
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
	if s.TargetType == "" && s.DraftID != "" {
		s.TargetType = "task_draft"
		s.TargetID = s.DraftID
	}
	if s.DraftID == "" && s.TargetType == "task_draft" {
		s.DraftID = s.TargetID
	}
}

func validateScheduleTarget(s storage.JobSchedule, drafts storage.TaskDraftsStore, tasks storage.ComplexTasksStore) error {
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
	default:
		return fmt.Errorf("unsupported target_type: %s", s.TargetType)
	}
	return nil
}

func referencedJobsByTemplate(store storage.JobsStore, templateID string) ([]string, error) {
	jf, err := store.Load()
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, j := range jf.Jobs {
		if j.Params == nil {
			continue
		}
		if tid, ok := j.Params["template_id"].(string); ok && tid == templateID {
			refs = append(refs, j.Name)
		}
	}
	return refs, nil
}

func referencedSchedulesByDraft(store storage.JobSchedulesStore, draftID string) ([]string, error) {
	jf, err := store.Load()
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, item := range jf.Items {
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

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
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
