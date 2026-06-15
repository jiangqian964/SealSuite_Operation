package api

import "encoding/json"

type ExecuteRequest struct {
	TemplateID      string                 `json:"template_id,omitempty"`
	Method          string                 `json:"method,omitempty"`
	Path            string                 `json:"path,omitempty"`
	Query           map[string]string      `json:"query,omitempty"`
	PathParams      map[string]string      `json:"path_params,omitempty"`
	Body            map[string]interface{} `json:"body,omitempty"`
	Mode            string                 `json:"mode,omitempty"`
	TransformConfig map[string]interface{} `json:"transform_config,omitempty"`
	LLMConfig       map[string]interface{} `json:"llm_config,omitempty"`
	OutputConfig    map[string]interface{} `json:"output_config,omitempty"`
}

type ExecuteResponse struct {
	HTTPStatus int             `json:"http_status"`
	LatencyMs  int64           `json:"latency_ms"`
	Body       json.RawMessage `json:"body"`

	BusinessCode    *int   `json:"business_code,omitempty"`
	BusinessMessage string `json:"business_message,omitempty"`
}

type HistoryItem struct {
	Time       int64  `json:"time"`
	TemplateID string `json:"template_id,omitempty"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	HTTPStatus int    `json:"http_status"`
	LatencyMs  int64  `json:"latency_ms"`
	OK         bool   `json:"ok"`
}

type PreviewRequest struct {
	ExecuteRequest
	PreviewMode     string `json:"preview_mode"` // dry_run | diff_only
	ReadBeforeWrite bool   `json:"read_before_write"`
}

type DiffItem struct {
	Path   string      `json:"path"`
	Before interface{} `json:"before,omitempty"`
	After  interface{} `json:"after,omitempty"`
}

type PreviewResponse struct {
	DryRunSupported bool             `json:"dry_run_supported"`
	RequestSummary  interface{}      `json:"request_summary"`
	Diff            []DiffItem       `json:"diff,omitempty"`
	Warnings        []string         `json:"warnings,omitempty"`
	DryRunResult    *ExecuteResponse `json:"dry_run_result,omitempty"`
}
