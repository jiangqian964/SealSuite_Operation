package storage

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type JobsFile struct {
	Version int   `yaml:"version" json:"version"`
	Jobs    []Job `yaml:"jobs" json:"jobs"`
}

type Job struct {
	Name    string                 `yaml:"name" json:"name"`
	Enabled bool                   `yaml:"enabled" json:"enabled"`
	Cron    string                 `yaml:"cron" json:"cron"`
	Type    string                 `yaml:"type" json:"type"`
	Params  map[string]interface{} `yaml:"params" json:"params"`
}

func (jf *JobsFile) Validate() error {
	if jf.Version == 0 {
		return fmt.Errorf("jobs.yaml: version is required")
	}
	seen := map[string]struct{}{}
	for i, j := range jf.Jobs {
		if j.Name == "" {
			return fmt.Errorf("jobs.yaml: jobs[%d].name is required", i)
		}
		if _, ok := seen[j.Name]; ok {
			return fmt.Errorf("jobs.yaml: duplicated job name: %s", j.Name)
		}
		seen[j.Name] = struct{}{}

		if j.Cron == "" {
			return fmt.Errorf("jobs.yaml: jobs[%d].cron is required", i)
		}
		if j.Type == "" {
			return fmt.Errorf("jobs.yaml: jobs[%d].type is required", i)
		}
	}
	return nil
}

type JobsStore struct {
	Path string
}

func (s JobsStore) Load() (*JobsFile, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return &JobsFile{Version: 1, Jobs: []Job{}}, nil
		}
		return nil, fmt.Errorf("read jobs file: %w", err)
	}
	var jf JobsFile
	if err := yaml.Unmarshal(b, &jf); err != nil {
		return nil, fmt.Errorf("unmarshal jobs yaml: %w", err)
	}
	if err := jf.Validate(); err != nil {
		return nil, err
	}
	return &jf, nil
}

func (s JobsStore) Save(jf *JobsFile) error {
	if jf == nil {
		return fmt.Errorf("jobs file is nil")
	}
	if err := jf.Validate(); err != nil {
		return err
	}
	b, err := yaml.Marshal(jf)
	if err != nil {
		return fmt.Errorf("marshal jobs yaml: %w", err)
	}
	return AtomicWriteFile(s.Path, b, 0o644)
}

func (s JobsStore) Get(name string) (*Job, bool, error) {
	jf, err := s.Load()
	if err != nil {
		return nil, false, err
	}
	for _, j := range jf.Jobs {
		if j.Name == name {
			cp := j
			return &cp, true, nil
		}
	}
	return nil, false, nil
}

func (s JobsStore) Upsert(job Job) error {
	jf, err := s.Load()
	if err != nil {
		return err
	}
	replaced := false
	for i := range jf.Jobs {
		if jf.Jobs[i].Name == job.Name {
			jf.Jobs[i] = job
			replaced = true
			break
		}
	}
	if !replaced {
		jf.Jobs = append(jf.Jobs, job)
	}
	return s.Save(jf)
}

func (s JobsStore) Delete(name string) error {
	jf, err := s.Load()
	if err != nil {
		return err
	}
	out := make([]Job, 0, len(jf.Jobs))
	for _, j := range jf.Jobs {
		if j.Name != name {
			out = append(out, j)
		}
	}
	jf.Jobs = out
	return s.Save(jf)
}
