package storage

import "fmt"

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
		return fmt.Errorf("legacy jobs: version is required")
	}
	seen := map[string]struct{}{}
	for i, j := range jf.Jobs {
		if j.Name == "" {
			return fmt.Errorf("legacy jobs: jobs[%d].name is required", i)
		}
		if _, ok := seen[j.Name]; ok {
			return fmt.Errorf("legacy jobs: duplicated job name: %s", j.Name)
		}
		seen[j.Name] = struct{}{}

		if j.Cron == "" {
			return fmt.Errorf("legacy jobs: jobs[%d].cron is required", i)
		}
		if j.Type == "" {
			return fmt.Errorf("legacy jobs: jobs[%d].type is required", i)
		}
	}
	return nil
}
