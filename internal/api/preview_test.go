package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"sealsuite-operation/internal/config"
	"sealsuite-operation/internal/sealsuite"
	"sealsuite-operation/internal/storage"
)

func TestPreviewDiffOnlyReadBeforeWrite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// token endpoint
		if r.URL.Path == "/api/open/v1/token" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"access_token":"T","expires_in":7200}}`))
			return
		}
		if r.URL.Path != "/api/v1/policies/1" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "T" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"name":"old"}`))
			return
		}
		if r.Method == http.MethodPut {
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{}}`))
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

	exec := NewExecutor(client, map[string]storage.Template{})
	out, err := exec.Preview(PreviewRequest{
		ExecuteRequest: ExecuteRequest{
			Method: "PUT",
			Path:   "/api/v1/policies/1",
			Body:   map[string]interface{}{"name": "new"},
		},
		PreviewMode:     "diff_only",
		ReadBeforeWrite: true,
	})
	if err != nil {
		t.Fatalf("Preview err=%v", err)
	}
	if len(out.Diff) == 0 || out.Diff[0].Path != "$.name" {
		t.Fatalf("expected diff on $.name, got=%#v", out.Diff)
	}
}
