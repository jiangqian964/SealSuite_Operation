package sqlite

import (
	"database/sql"
	"encoding/json"
	"strings"
)

type baseRepo struct {
	db *sql.DB
}

func mustJSON(v interface{}) string {
	if v == nil {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func mustJSONArray(v interface{}) string {
	if v == nil {
		return "[]"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func scanJSON(raw string, defaultRaw string, out interface{}) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = defaultRaw
	}
	return json.Unmarshal([]byte(raw), out)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func intToBool(v int) bool {
	return v != 0
}
