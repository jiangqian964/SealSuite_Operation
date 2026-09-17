package storage

import (
	"fmt"
	"strings"
)

const (
	DefaultExternalIPSyncSourceType     = "google_ip_ranges"
	DefaultExternalIPSyncSourceURL      = "https://www.gstatic.com/ipranges/goog.json"
	DefaultExternalIPSyncIPVersion      = "ipv4"
	DefaultExternalIPSyncWriteAction    = "append_if_missing"
	DefaultExternalIPSyncFeilianAPIPath = "/api/open/v1/addr/management/add"
)

type ExternalIPSyncTask struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	SourceType       string `json:"source_type"`
	SourceURL        string `json:"source_url"`
	IPVersion        string `json:"ip_version"`
	ResourceID       string `json:"resource_id"`
	ResourceTagNames string `json:"resource_tag_names"`
	CreateMode       string `json:"create_mode"`
	NewResourceName  string `json:"new_resource_name"`
	WriteAction      string `json:"write_action"`
	FeilianAPIPath   string `json:"feilian_api_path"`
	DryRun           bool   `json:"dry_run"`
	SkipWhenEmpty    bool   `json:"skip_when_empty"`
	Enabled          bool   `json:"enabled"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

func NormalizeExternalIPSyncTask(in ExternalIPSyncTask) ExternalIPSyncTask {
	in.ID = strings.TrimSpace(in.ID)
	in.Name = strings.TrimSpace(in.Name)
	in.SourceType = strings.TrimSpace(in.SourceType)
	in.SourceURL = strings.TrimSpace(in.SourceURL)
	in.IPVersion = strings.TrimSpace(in.IPVersion)
	in.ResourceID = strings.TrimSpace(in.ResourceID)
	in.ResourceTagNames = strings.TrimSpace(in.ResourceTagNames)
	in.CreateMode = strings.TrimSpace(in.CreateMode)
	in.NewResourceName = strings.TrimSpace(in.NewResourceName)
	in.WriteAction = strings.TrimSpace(in.WriteAction)
	in.FeilianAPIPath = strings.TrimSpace(in.FeilianAPIPath)
	in.CreatedAt = strings.TrimSpace(in.CreatedAt)
	in.UpdatedAt = strings.TrimSpace(in.UpdatedAt)

	if in.SourceType == "" {
		in.SourceType = DefaultExternalIPSyncSourceType
	}
	if in.SourceURL == "" {
		in.SourceURL = DefaultExternalIPSyncSourceURL
	}
	if in.IPVersion == "" {
		in.IPVersion = DefaultExternalIPSyncIPVersion
	}
	if in.WriteAction == "" {
		in.WriteAction = DefaultExternalIPSyncWriteAction
	}
	if in.FeilianAPIPath == "" {
		in.FeilianAPIPath = DefaultExternalIPSyncFeilianAPIPath
	}
	if in.CreateMode == "" {
		in.CreateMode = FeishuResourceCreateModeSelect
	}

	return in
}

func (t ExternalIPSyncTask) Validate() error {
	if t.ID == "" {
		return NewValidationError("external ip sync task: id is required")
	}
	if t.Name == "" {
		return NewValidationError("external ip sync task: name is required")
	}
	switch t.CreateMode {
	case FeishuResourceCreateModeAdd:
		if t.NewResourceName == "" {
			return NewValidationError("external ip sync task: new_resource_name is required for create_mode=add")
		}
	case FeishuResourceCreateModeSelect, "":
		if t.ResourceID == "" {
			return NewValidationError("external ip sync task: resource_id is required")
		}
	default:
		return NewValidationError(fmt.Sprintf("external ip sync task: invalid create_mode %q", t.CreateMode))
	}
	switch t.SourceType {
	case DefaultExternalIPSyncSourceType:
	default:
		return NewValidationError(fmt.Sprintf("external ip sync task: invalid source_type %q", t.SourceType))
	}
	switch t.IPVersion {
	case "ipv4", "ipv6", "all":
	default:
		return NewValidationError(fmt.Sprintf("external ip sync task: invalid ip_version %q", t.IPVersion))
	}
	switch t.WriteAction {
	case DefaultExternalIPSyncWriteAction:
	default:
		return NewValidationError(fmt.Sprintf("external ip sync task: invalid write_action %q", t.WriteAction))
	}
	return nil
}
