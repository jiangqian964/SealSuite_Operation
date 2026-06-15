package storage

import "strings"

const DefaultScheduleTimezone = "Asia/Shanghai"

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

func EnsureJobSchedulesFileDefaults(jf *JobSchedulesFile) {
	if jf == nil {
		return
	}
	if jf.Version == 0 {
		jf.Version = 1
	}
	if jf.Items == nil {
		jf.Items = []JobSchedule{}
	}
	for i := range jf.Items {
		NormalizeJobScheduleDefaults(&jf.Items[i])
	}
}

func NormalizeJobSchedule(s JobSchedule) JobSchedule {
	NormalizeJobScheduleDefaults(&s)
	return s
}

func NormalizeJobScheduleDefaults(s *JobSchedule) {
	if s == nil {
		return
	}
	s.DraftID = strings.TrimSpace(s.DraftID)
	s.TargetType = strings.TrimSpace(s.TargetType)
	s.TargetID = strings.TrimSpace(s.TargetID)
	s.ScheduleType = strings.TrimSpace(s.ScheduleType)
	s.Timezone = strings.TrimSpace(s.Timezone)

	if s.TargetType == "" && s.DraftID != "" {
		s.TargetType = "task_draft"
		s.TargetID = s.DraftID
	}
	if s.DraftID == "" && s.TargetType == "task_draft" {
		s.DraftID = s.TargetID
	}
	if s.ScheduleType == "" {
		s.ScheduleType = "interval"
	}
	if s.Timezone == "" {
		s.Timezone = DefaultScheduleTimezone
	}
}
