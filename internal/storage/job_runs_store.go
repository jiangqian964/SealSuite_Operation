package storage

import (
	"encoding/json"
	"fmt"
	"os"
)

type JobRunsFile struct {
	Version int                      `json:"version"`
	Items   map[string]JobRunRecord  `json:"items"`
}

type JobRunRecord struct {
	LastRun    string `json:"last_run"`
	OK         bool   `json:"ok"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error"`
}

type JobRunsStore struct {
	Path string
}

func (s JobRunsStore) Load() (*JobRunsFile, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return &JobRunsFile{Version: 1, Items: map[string]JobRunRecord{}}, nil
		}
		return nil, fmt.Errorf("read job-runs file: %w", err)
	}
	var jf JobRunsFile
	if err := json.Unmarshal(b, &jf); err != nil {
		return nil, fmt.Errorf("unmarshal job-runs json: %w", err)
	}
	if jf.Version == 0 {
		jf.Version = 1
	}
	if jf.Items == nil {
		jf.Items = map[string]JobRunRecord{}
	}
	return &jf, nil
}

func (s JobRunsStore) Save(jf *JobRunsFile) error {
	if jf == nil {
		return fmt.Errorf("job runs file is nil")
	}
	if jf.Version == 0 {
		jf.Version = 1
	}
	if jf.Items == nil {
		jf.Items = map[string]JobRunRecord{}
	}
	b, err := json.MarshalIndent(jf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal job-runs json: %w", err)
	}
	return AtomicWriteFile(s.Path, b, 0o644)
}

func (s JobRunsStore) Put(name string, rec JobRunRecord) error {
	jf, err := s.Load()
	if err != nil {
		return err
	}
	jf.Items[name] = rec
	return s.Save(jf)
}

