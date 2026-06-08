package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

func (e *Executor) Preview(req PreviewRequest) (*PreviewResponse, error) {
	method, path, query, body, templateID, dryRunParam, err := e.resolve(req.ExecuteRequest)
	if err != nil {
		return nil, err
	}

	out := &PreviewResponse{
		DryRunSupported: false,
		RequestSummary: map[string]interface{}{
			"template_id": templateID,
			"method":      method,
			"path":        path,
		},
	}

	previewMode := strings.ToLower(req.PreviewMode)
	if previewMode == "" {
		previewMode = "diff_only"
	}

	// dry-run：优先走模板声明的 dry_run_query_param
	if previewMode == "dry_run" && dryRunParam != "" {
		if query == nil {
			query = map[string]string{}
		}
		query[dryRunParam] = "true"
		er := ExecuteRequest{
			TemplateID: templateID,
			Method:     method,
			Path:       path,
			Query:      query,
			Body:       body,
		}
		r, err := e.Execute(er)
		if err != nil {
			return nil, err
		}
		out.DryRunSupported = true
		out.DryRunResult = r
		return out, nil
	}

	// fallback：diff_only（v0：只做顶层字段 diff）
	var warnings []string
	var beforeObj map[string]interface{}
	if req.ReadBeforeWrite {
		status, raw, err := e.client.DoRaw("GET", path, nil, nil)
		if err != nil || status < 200 || status >= 300 {
			warnings = append(warnings, fmt.Sprintf("read-before-write failed (status=%d): %v", status, err))
		} else {
			_ = json.Unmarshal(raw, &beforeObj) // v0 宽松处理
			// 若返回的是 CommonResponse，尝试取 data 字段作为 before
			if beforeObj != nil {
				if data, ok := beforeObj["data"]; ok {
					if dm, ok := data.(map[string]interface{}); ok {
						beforeObj = dm
					}
				}
			}
		}
	}
	out.Warnings = warnings

	out.Diff = diffTopLevel(beforeObj, body)
	return out, nil
}

func (e *Executor) resolve(req ExecuteRequest) (method, path string, query map[string]string, body map[string]interface{}, templateID, dryRunParam string, err error) {
	method = strings.ToUpper(req.Method)
	path = req.Path
	query = req.Query
	body = req.Body
	templateID = req.TemplateID

	if templateID != "" {
		tpl, ok := e.templates[templateID]
		if !ok {
			return "", "", nil, nil, "", "", fmt.Errorf("template not found: %s", templateID)
		}
		method = strings.ToUpper(tpl.Method)
		path = tpl.Path
		dryRunParam = tpl.DryRunQueryParam
	}
	if method == "" {
		return "", "", nil, nil, "", "", fmt.Errorf("method is required")
	}
	if path == "" {
		return "", "", nil, nil, "", "", fmt.Errorf("path is required")
	}
	if len(req.PathParams) > 0 {
		for k, v := range req.PathParams {
			path = strings.ReplaceAll(path, "{"+k+"}", v)
		}
	}
	if strings.Contains(path, "{") && strings.Contains(path, "}") {
		return "", "", nil, nil, "", "", fmt.Errorf("unresolved path params in path: %s", path)
	}
	return method, path, query, body, templateID, dryRunParam, nil
}

func diffTopLevel(before map[string]interface{}, after map[string]interface{}) []DiffItem {
	if after == nil {
		return nil
	}
	var out []DiffItem
	for k, v := range after {
		var bv interface{} = nil
		if before != nil {
			bv = before[k]
		}
		if !jsonDeepEqual(bv, v) {
			out = append(out, DiffItem{
				Path:   "$." + k,
				Before: bv,
				After:  v,
			})
		}
	}
	return out
}

func jsonDeepEqual(a, b interface{}) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

