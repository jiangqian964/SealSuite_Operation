package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"sealsuite-operation/internal/config"
	"sealsuite-operation/internal/sealsuite"
	"sealsuite-operation/internal/storage"
)

func TestExecutorExecuteWithTemplateParsesBusinessCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// token endpoint
		if r.URL.Path == "/api/open/v1/token" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"access_token":"T","expires_in":7200}}`))
			return
		}
		if r.URL.Path != "/api/v1/users" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "T" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"total":1}}`))
	}))
	defer srv.Close()

	client := sealsuite.NewClient(&config.SealSuiteConfig{
		BaseURL:    srv.URL,
		AccessKey:  "ak",
		SecretKey:  "sk",
		Timeout:    5,
		RetryTimes: 0,
	})
	client.SetMockMode(false)

	templates := map[string]storage.Template{
		"users_list": {ID: "users_list", Name: "用户-列表", Category: "users", Method: "GET", Path: "/api/v1/users"},
	}
	exec := NewExecutor(client, templates)

	resp, err := exec.Execute(ExecuteRequest{TemplateID: "users_list"})
	if err != nil {
		t.Fatalf("Execute err=%v", err)
	}
	if resp.BusinessCode == nil || *resp.BusinessCode != 0 {
		t.Fatalf("expected business code 0, got=%v", resp.BusinessCode)
	}
}
