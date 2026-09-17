package storage

const (
	DLPEventTable        = "dlp_events"
	DLPAnalysisTable    = "dlp_analysis_results"
	DLPWhitelistTable   = "dlp_whitelist"
	DLPAnalysisStateTable = "dlp_analysis_state"
)

type DLPEvent struct {
	ID              string `json:"id"`
	FileInfoName    string `json:"file_info_name"`
	FileInfoPath    string `json:"file_info_path"`
	FileInfoType    string `json:"file_info_type"`
	LeakWayAppName  string `json:"leak_way_app_name"`
	EvidenceURL     string `json:"evidence_url"`
	EventType       string `json:"event_type"`
	UserID          string `json:"user_id"`
	UserName        string `json:"user_name"`
	DeviceID        string `json:"device_id"`
	EventTime       string `json:"event_time"`
	ImportedAt      string `json:"imported_at"`
	Analyzed        bool   `json:"analyzed"`
	RawJSON         string `json:"raw_json"`
}

type DLPAnalysisResult struct {
	ID            string  `json:"id"`
	EventID       string  `json:"event_id"`
	Category      string  `json:"category"`
	ShouldExclude bool    `json:"should_exclude"`
	Confidence    float64 `json:"confidence"`
	Reasoning     string  `json:"reasoning"`
	AnalyzedAt    string  `json:"analyzed_at"`
	Whitelisted   bool    `json:"whitelisted"`
	MatchType     string  `json:"match_type"`
	MatchValue    string  `json:"match_value"`
}

type DLPWhitelistItem struct {
	ID          string `json:"id"`
	MatchType   string `json:"match_type"`
	MatchValue  string `json:"match_value"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
}

type DLPAnalysisState struct {
	LastSyncAt        string `json:"last_sync_at"`
	LastSyncCount     int    `json:"last_sync_count"`
	SyncScheduleEnabled bool  `json:"sync_schedule_enabled"`
	LastAnalysisAt    string `json:"last_analysis_at"`
	LastAnalysisCount int    `json:"last_analysis_count"`
	AnalysisScheduleEnabled bool `json:"analysis_schedule_enabled"`
}

func (w DLPWhitelistItem) Match(filePath, fileName string) bool {
	switch w.MatchType {
	case "path":
		return filePath == w.MatchValue
	case "name":
		return fileName == w.MatchValue
	case "extension":
		return hasSuffix(fileName, w.MatchValue)
	default:
		return false
	}
}

func hasSuffix(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}
