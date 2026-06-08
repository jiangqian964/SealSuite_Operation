package storage

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

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

type TaskDraftsStore struct {
	Path string
}

func normalizeTaskDraftDefaults(draft *TaskDraft) {
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
	if draft.CycleMode == "once" {
		draft.RunCount = 1
		draft.RunUntil = ""
		return
	}
	if draft.RunCount <= 0 {
		draft.RunCount = 1
	}
}

func (s TaskDraftsStore) Load() (*TaskDraftsFile, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return &TaskDraftsFile{Version: 1, Items: []TaskDraft{}}, nil
		}
		return nil, fmt.Errorf("read task drafts file: %w", err)
	}
	var tf TaskDraftsFile
	if err := yaml.Unmarshal(b, &tf); err != nil {
		return nil, fmt.Errorf("unmarshal task drafts yaml: %w", err)
	}
	if tf.Version == 0 {
		tf.Version = 1
	}
	if tf.Items == nil {
		tf.Items = []TaskDraft{}
	}
	for i := range tf.Items {
		normalizeTaskDraftDefaults(&tf.Items[i])
	}
	return &tf, nil
}

func (s TaskDraftsStore) Save(tf *TaskDraftsFile) error {
	if tf == nil {
		return fmt.Errorf("task drafts file is nil")
	}
	if tf.Version == 0 {
		tf.Version = 1
	}
	b, err := yaml.Marshal(tf)
	if err != nil {
		return fmt.Errorf("marshal task drafts yaml: %w", err)
	}
	return AtomicWriteFile(s.Path, b, 0o644)
}

func (s TaskDraftsStore) Get(id string) (*TaskDraft, bool, error) {
	tf, err := s.Load()
	if err != nil {
		return nil, false, err
	}
	for _, item := range tf.Items {
		if item.ID == id {
			cp := item
			return &cp, true, nil
		}
	}
	return nil, false, nil
}

func (s TaskDraftsStore) Upsert(draft TaskDraft) error {
	normalizeTaskDraftDefaults(&draft)
	tf, err := s.Load()
	if err != nil {
		return err
	}
	replaced := false
	for i := range tf.Items {
		if tf.Items[i].ID == draft.ID {
			tf.Items[i] = draft
			replaced = true
			break
		}
	}
	if !replaced {
		tf.Items = append(tf.Items, draft)
	}
	return s.Save(tf)
}

func (s TaskDraftsStore) Delete(id string) error {
	tf, err := s.Load()
	if err != nil {
		return err
	}
	out := make([]TaskDraft, 0, len(tf.Items))
	for _, item := range tf.Items {
		if item.ID != id {
			out = append(out, item)
		}
	}
	tf.Items = out
	return s.Save(tf)
}
