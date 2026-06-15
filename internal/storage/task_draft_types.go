package storage

import "strings"

type TaskDraftsFile struct {
	Version int         `yaml:"version" json:"version"`
	Items   []TaskDraft `yaml:"items" json:"items"`
}

type TaskDraft struct {
	ID               string                 `yaml:"id" json:"id"`
	Name             string                 `yaml:"name" json:"name"`
	Mode             string                 `yaml:"mode" json:"mode"`
	CycleMode        string                 `yaml:"cycle_mode,omitempty" json:"cycle_mode,omitempty"`
	RunCount         int                    `yaml:"run_count,omitempty" json:"run_count,omitempty"`
	RunUntil         string                 `yaml:"run_until,omitempty" json:"run_until,omitempty"`
	SourceTemplateID string                 `yaml:"source_template_id,omitempty" json:"source_template_id,omitempty"`
	WebhookConfigID  string                 `yaml:"webhook_config_id,omitempty" json:"webhook_config_id,omitempty"`
	WebhookEnabled   bool                   `yaml:"webhook_enabled,omitempty" json:"webhook_enabled,omitempty"`
	InputConfig      map[string]interface{} `yaml:"input_config,omitempty" json:"input_config,omitempty"`
	TransformConfig  map[string]interface{} `yaml:"transform_config,omitempty" json:"transform_config,omitempty"`
	LLMConfig        map[string]interface{} `yaml:"llm_config,omitempty" json:"llm_config,omitempty"`
	OutputConfig     map[string]interface{} `yaml:"output_config,omitempty" json:"output_config,omitempty"`
}

func EnsureTaskDraftsFileDefaults(tf *TaskDraftsFile) {
	if tf == nil {
		return
	}
	if tf.Version == 0 {
		tf.Version = 1
	}
	if tf.Items == nil {
		tf.Items = []TaskDraft{}
	}
	for i := range tf.Items {
		NormalizeTaskDraftDefaults(&tf.Items[i])
	}
}

func NormalizeTaskDraft(d TaskDraft) TaskDraft {
	NormalizeTaskDraftDefaults(&d)
	return d
}

func NormalizeTaskDraftDefaults(draft *TaskDraft) {
	if draft == nil {
		return
	}
	draft.CycleMode = strings.TrimSpace(draft.CycleMode)
	draft.RunUntil = strings.TrimSpace(draft.RunUntil)

	switch draft.CycleMode {
	case "", "once", "5min", "30min", "1h", "6h", "24h", "7day", "1month", "1year":
	default:
		draft.CycleMode = "once"
	}

	if draft.CycleMode == "" {
		draft.CycleMode = "once"
	}
	if draft.CycleMode == "once" {
		draft.RunCount = 1
		draft.RunUntil = ""
		return
	}
	if draft.RunCount <= 0 {
		draft.RunCount = 1
	}
}
