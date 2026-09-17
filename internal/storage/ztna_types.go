package storage

const (
	ZTNAAccessLogTable      = "ztna_access_logs"
	ZTNAAccessStatsTable    = "ztna_access_stats"
	ZTNAPolicyAnalysisTable = "ztna_policy_analysis"
	ZTNATaskStateTable      = "ztna_task_state"
)

const (
	ZTNAnalysisCategoryShouldHave   = "access_should_have"
	ZTNAnalysisCategoryShouldRemove = "access_should_remove"
	ZTNAnalysisCategoryNeedsReview  = "access_needs_review"
	ZTNAnalysisCategoryUnknown      = "unknown"
)

const (
	ZTNAnalysisStatusPending  = "pending"
	ZTNAnalysisStatusAnalyzed = "analyzed"
	ZTNAnalysisStatusReviewed = "reviewed"
)

const (
	ZTNAActionAccept = "accept"
	ZTNAActionDrop   = "drop"
)

type ZTNAAccessLog struct {
	ID              string `json:"id"`
	LogID           string `json:"log_id"`
	UserID          string `json:"user_id"`
	UserFullName    string `json:"user_full_name"`
	DepartmentID    string `json:"department_id"`
	DepartmentPath  string `json:"department_path"`
	RolesName       string `json:"roles_name"`
	RolesID         string `json:"roles_id"`
	DestIP          string `json:"dest_ip"`
	DestPort        int    `json:"dest_port"`
	Action          string `json:"action"`
	Protocol        string `json:"protocol"`
	EventTime       string `json:"event_time"`
	ImportedAt      string `json:"imported_at"`
	RawJSON         string `json:"raw_json"`
}

type ZTNAAccessStats struct {
	ID                string `json:"id"`
	UserID            string `json:"user_id"`
	UserName          string `json:"user_name"`
	UserRole          string `json:"user_role"`
	UserDepartment    string `json:"user_department"`
	DepartmentID      string `json:"department_id"`
	DestIP            string `json:"dest_ip"`
	DestPort          int    `json:"dest_port"`
	Protocol          string `json:"protocol"`
	ResourceTag       string `json:"resource_tag"`
	FirstAccessTime   string `json:"first_access_time"`
	LastAccessTime    string `json:"last_access_time"`
	AccessDays30      int    `json:"access_days_30"`
	AccessDays90      int    `json:"access_days_90"`
	TotalAccessCount  int    `json:"total_access_count"`
	SuccessCount      int    `json:"success_count"`
	FailedCount       int    `json:"failed_count"`
	AvgDurationSec    int    `json:"avg_duration_sec"`
	TotalBytes        int64  `json:"total_bytes"`
	WorkHourCount     int    `json:"work_hour_count"`
	OffHourCount      int    `json:"off_hour_count"`
	NightCount        int    `json:"night_count"`
	UpdatedAt         string `json:"updated_at"`
}

type ZTNAPolicyAnalysis struct {
	ID              string   `json:"id"`
	StatsID         string   `json:"stats_id"`
	UserID          string   `json:"user_id"`
	UserName        string   `json:"user_name"`
	UserRole        string   `json:"user_role"`
	UserDepartment  string   `json:"user_department"`
	DestIP          string   `json:"dest_ip"`
	DestPort        int      `json:"dest_port"`
	Protocol        string   `json:"protocol"`
	ResourceTag     string   `json:"resource_tag"`
	Category        string   `json:"category"`
	ShouldRevoke    bool     `json:"should_revoke"`
	Confidence      float64  `json:"confidence"`
	Reasoning       string   `json:"reasoning"`
	Reasons         []string `json:"reasons"`
	SuggestedPolicy string   `json:"suggested_policy"`
	Method          string   `json:"method"`
	Status          string   `json:"status"`
	ReviewedBy      string   `json:"reviewed_by"`
	ReviewedAt      string   `json:"reviewed_at"`
	ReviewComment   string   `json:"review_comment"`
	AnalyzedAt      string   `json:"analyzed_at"`
	Revoked         bool     `json:"revoked"`
	RevokedAt       string   `json:"revoked_at"`
	RevokeError     string   `json:"revoke_error"`
}

type ZTNATaskState struct {
	ID                      string `json:"id"`
	LastSyncAt              string `json:"last_sync_at"`
	LastSyncCount           int    `json:"last_sync_count"`
	LastSyncError           string `json:"last_sync_error"`
	LastStatsAt             string `json:"last_stats_at"`
	LastStatsUserCount      int    `json:"last_stats_user_count"`
	LastStatsResourceCount  int    `json:"last_stats_resource_count"`
	LastStatsError          string `json:"last_stats_error"`
	LastAnalysisAt          string `json:"last_analysis_at"`
	LastAnalysisCount       int    `json:"last_analysis_count"`
	LastAnalysisError       string `json:"last_analysis_error"`
	SyncScheduleEnabled     bool   `json:"sync_schedule_enabled"`
	StatsScheduleEnabled    bool   `json:"stats_schedule_enabled"`
	AnalysisScheduleEnabled bool   `json:"analysis_schedule_enabled"`
}

type ZTNASyncResult struct {
	SyncedCount int    `json:"synced_count"`
	SyncAt      string `json:"sync_at"`
	Error       string `json:"error,omitempty"`
}

type ZTNAStatsResult struct {
	UserCount    int    `json:"user_count"`
	ResourceCount int   `json:"resource_count"`
	StatsAt     string `json:"stats_at"`
	Error       string `json:"error,omitempty"`
}

type ZTNAAnalyzeResult struct {
	AnalyzedCount   int    `json:"analyzed_count"`
	ShouldHaveCount int    `json:"should_have_count"`
	ShouldRemoveCount int  `json:"should_remove_count"`
	NeedsReviewCount int   `json:"needs_review_count"`
	UnknownCount    int    `json:"unknown_count"`
	AnalyzeAt       string `json:"analyze_at"`
	Error           string `json:"error,omitempty"`
}

type ZTNAFullResult struct {
	SyncedCount     int    `json:"synced_count"`
	UserCount       int    `json:"user_count"`
	ResourceCount   int    `json:"resource_count"`
	AnalyzedCount   int    `json:"analyzed_count"`
	ShouldHaveCount int    `json:"should_have_count"`
	ShouldRemoveCount int  `json:"should_remove_count"`
	Error           string `json:"error,omitempty"`
}

type ZTNAAnalysisSummary struct {
	Total            int `json:"total"`
	ShouldHaveCount  int `json:"should_have_count"`
	ShouldRemoveCount int `json:"should_remove_count"`
	NeedsReviewCount int `json:"needs_review_count"`
	UnknownCount     int `json:"unknown_count"`
	UserCount        int `json:"user_count"`
	ResourceCount    int `json:"resource_count"`
}

type ZTNAStatsFilter struct {
	UserID       string
	UserKeyword  string
	DestIP       string
	Category     string
	ShouldRevoke *bool
}

type ZTNAAccessLogFilter struct {
	UserID   string
	DestIP   string
	Action   string
}
