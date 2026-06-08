package storage

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type JobSchedulesFile struct {
	Version int           `yaml:"version" json:"version"`
	Items   []JobSchedule `yaml:"items" json:"items"`
}

type JobSchedule struct {
	ID              string                 `yaml:"id" json:"id"`
	DraftID         string                 `yaml:"draft_id,omitempty" json:"draft_id,omitempty"`
	TargetType      string                 `yaml:"target_type,omitempty" json:"target_type,omitempty"`
	TargetID        string                 `yaml:"target_id,omitempty" json:"target_id,omitempty"`
	Enabled         bool                   `yaml:"enabled" json:"enabled"`
	WebhookConfigID string                 `yaml:"webhook_config_id,omitempty" json:"webhook_config_id,omitempty"`
	WebhookEnabled  bool                   `yaml:"webhook_enabled,omitempty" json:"webhook_enabled,omitempty"`
	StartAt         string                 `yaml:"start_at,omitempty" json:"start_at,omitempty"`
	EndAt           string                 `yaml:"end_at,omitempty" json:"end_at,omitempty"`
	ScheduleType    string                 `yaml:"schedule_type" json:"schedule_type"`
	Cron            string                 `yaml:"cron,omitempty" json:"cron,omitempty"`
	Interval        string                 `yaml:"interval,omitempty" json:"interval,omitempty"`
	Timezone        string                 `yaml:"timezone,omitempty" json:"timezone,omitempty"`
	PostFilter      map[string]interface{} `yaml:"post_filter,omitempty" json:"post_filter,omitempty"`
	FieldSelect     []string               `yaml:"field_select,omitempty" json:"field_select,omitempty"`
	TruncateRules   map[string]interface{} `yaml:"truncate_rules,omitempty" json:"truncate_rules,omitempty"`
}

type JobSchedulesStore struct {
	Path string
}

func (s JobSchedulesStore) Load() (*JobSchedulesFile, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return &JobSchedulesFile{Version: 1, Items: []JobSchedule{}}, nil
		}
		return nil, fmt.Errorf("read job schedules file: %w", err)
	}
	var jf JobSchedulesFile
	if err := yaml.Unmarshal(b, &jf); err != nil {
		return nil, fmt.Errorf("unmarshal job schedules yaml: %w", err)
	}
	if jf.Version == 0 {
		jf.Version = 1
	}
	if jf.Items == nil {
		jf.Items = []JobSchedule{}
	}
	return &jf, nil
}

func (s JobSchedulesStore) Save(jf *JobSchedulesFile) error {
	if jf == nil {
		return fmt.Errorf("job schedules file is nil")
	}
	if jf.Version == 0 {
		jf.Version = 1
	}
	b, err := yaml.Marshal(jf)
	if err != nil {
		return fmt.Errorf("marshal job schedules yaml: %w", err)
	}
	return AtomicWriteFile(s.Path, b, 0o644)
}

func (s JobSchedulesStore) Get(id string) (*JobSchedule, bool, error) {
	jf, err := s.Load()
	if err != nil {
		return nil, false, err
	}
	for _, item := range jf.Items {
		if item.ID == id {
			cp := item
			return &cp, true, nil
		}
	}
	return nil, false, nil
}

func (s JobSchedulesStore) Upsert(schedule JobSchedule) error {
	jf, err := s.Load()
	if err != nil {
		return err
	}
	if schedule.TargetType == "" && schedule.DraftID != "" {
		schedule.TargetType = "task_draft"
		schedule.TargetID = schedule.DraftID
	}
	replaced := false
	for i := range jf.Items {
		if jf.Items[i].ID == schedule.ID {
			jf.Items[i] = schedule
			replaced = true
			break
		}
	}
	if !replaced {
		jf.Items = append(jf.Items, schedule)
	}
	return s.Save(jf)
}

func (s JobSchedulesStore) Delete(id string) error {
	jf, err := s.Load()
	if err != nil {
		return err
	}
	out := make([]JobSchedule, 0, len(jf.Items))
	for _, item := range jf.Items {
		if item.ID != id {
			out = append(out, item)
		}
	}
	jf.Items = out
	return s.Save(jf)
}
