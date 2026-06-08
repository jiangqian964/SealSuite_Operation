package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"sealsuite-operation/internal/sealsuite"
	"sealsuite-operation/internal/storage"
)

type Executor struct {
	client    *sealsuite.Client
	templates map[string]storage.Template

	mu      sync.RWMutex
	history []HistoryItem
}

func NewExecutor(client *sealsuite.Client, templates map[string]storage.Template) *Executor {
	if templates == nil {
		templates = map[string]storage.Template{}
	}
	return &Executor{
		client:    client,
		templates: templates,
		history:   make([]HistoryItem, 0, 200),
	}
}

func (e *Executor) SetClient(client *sealsuite.Client) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.client = client
}

func (e *Executor) SetTemplates(templates map[string]storage.Template) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if templates == nil {
		templates = map[string]storage.Template{}
	}
	e.templates = templates
}

func (e *Executor) Execute(req ExecuteRequest) (*ExecuteResponse, error) {
	method := strings.ToUpper(req.Method)
	path := req.Path
	query := req.Query
	body := req.Body
	templateID := req.TemplateID

	if templateID != "" {
		tpl, ok := e.templates[templateID]
		if !ok {
			return nil, fmt.Errorf("template not found: %s", templateID)
		}
		method = strings.ToUpper(tpl.Method)
		path = tpl.Path
	}
	if method == "" {
		return nil, fmt.Errorf("method is required")
	}
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	if len(req.PathParams) > 0 {
		for k, v := range req.PathParams {
			path = strings.ReplaceAll(path, "{"+k+"}", v)
		}
	}
	if strings.Contains(path, "{") && strings.Contains(path, "}") {
		return nil, fmt.Errorf("unresolved path params in path: %s", path)
	}

	start := time.Now()
	status, raw, err := e.client.DoRaw(method, path, query, body)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		e.appendHistory(HistoryItem{
			Time:       time.Now().Unix(),
			TemplateID: templateID,
			Method:     method,
			Path:       path,
			HTTPStatus: status,
			LatencyMs:  latency,
			OK:         false,
		})
		return nil, err
	}

	out := &ExecuteResponse{
		HTTPStatus: status,
		LatencyMs:  latency,
		Body:       raw,
	}

	// 解析业务码（兼容当前 CommonResponse 结构：code/message/data）
	var biz struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &biz); err == nil && biz.Code != nil {
		out.BusinessCode = biz.Code
		out.BusinessMessage = biz.Message
	}

	ok := status >= 200 && status < 300
	if out.BusinessCode != nil {
		ok = ok && *out.BusinessCode == 0
	}
	e.appendHistory(HistoryItem{
		Time:       time.Now().Unix(),
		TemplateID: templateID,
		Method:     method,
		Path:       path,
		HTTPStatus: status,
		LatencyMs:  latency,
		OK:         ok,
	})

	return out, nil
}

func (e *Executor) History(limit int) []HistoryItem {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if limit <= 0 || limit >= len(e.history) {
		cp := make([]HistoryItem, len(e.history))
		copy(cp, e.history)
		return cp
	}
	start := len(e.history) - limit
	cp := make([]HistoryItem, limit)
	copy(cp, e.history[start:])
	return cp
}

func (e *Executor) appendHistory(item HistoryItem) {
	e.mu.Lock()
	defer e.mu.Unlock()
	const capN = 200
	if len(e.history) >= capN {
		// drop oldest
		copy(e.history, e.history[1:])
		e.history[len(e.history)-1] = item
		return
	}
	e.history = append(e.history, item)
}
