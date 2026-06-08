package storage

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ConfigStore 以“保留未知字段”的方式更新 config.yaml 的部分字段。
// 这里不依赖 viper，以避免丢字段/丢结构；同时也便于保持将来扩展兼容。
type ConfigStore struct {
	Path string
}

// GetSealsuiteConnection 读取 sealsuite 的连接信息（包含 scheme/host/port/base_url/access_key/secret_key）。
// 注意：secret_key 返回明文仅用于服务端逻辑，不应直接回显给前端。
func (s ConfigStore) GetSealsuiteConnection() (map[string]interface{}, error) {
	m, err := s.loadMap()
	if err != nil {
		return nil, err
	}
	ss, _ := m["sealsuite"].(map[string]interface{})
	if ss == nil {
		ss = map[string]interface{}{}
	}
	return ss, nil
}

func (s ConfigStore) GetLLMConfig() (map[string]interface{}, error) {
	m, err := s.loadMap()
	if err != nil {
		return nil, err
	}
	sec, _ := m["llm"].(map[string]interface{})
	if sec == nil {
		sec = map[string]interface{}{}
	}
	if _, ok := sec["planner"]; !ok && len(sec) > 0 {
		legacy := cloneMap(sec)
		delete(legacy, "mock_mode")
		sec = map[string]interface{}{
			"mock_mode": sec["mock_mode"],
			"planner":   cloneMap(legacy),
			"formatter": cloneMap(legacy),
		}
	}
	return sec, nil
}

// UpdateSealsuiteConnection 更新 sealsuite 的连接信息。
// 规则：若 secret_key 为空字符串，则不覆盖已有 secret_key（便于“只改 host/port 不改 secret”）。
func (s ConfigStore) UpdateSealsuiteConnection(in map[string]interface{}) error {
	m, err := s.loadMap()
	if err != nil {
		return err
	}
	ss, _ := m["sealsuite"].(map[string]interface{})
	if ss == nil {
		ss = map[string]interface{}{}
	}

	// 若 secret_key 为空，保留旧值
	if v, ok := in["secret_key"]; ok {
		if vs, ok := v.(string); ok && vs == "" {
			delete(in, "secret_key")
		}
	}

	for k, v := range in {
		ss[k] = v
	}
	m["sealsuite"] = ss

	out, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal config yaml: %w", err)
	}
	return AtomicWriteFile(s.Path, out, 0o644)
}

// UpdateLLMConfig 更新 llm 段配置。
// 若 api_key 为空字符串，则保留旧 api_key。
func (s ConfigStore) UpdateLLMConfig(in map[string]interface{}) error {
	m, err := s.loadMap()
	if err != nil {
		return err
	}
	sec, _ := m["llm"].(map[string]interface{})
	if sec == nil {
		sec = map[string]interface{}{}
	}
	if _, ok := sec["planner"]; !ok && len(sec) > 0 {
		legacy := cloneMap(sec)
		delete(legacy, "mock_mode")
		sec = map[string]interface{}{
			"mock_mode": sec["mock_mode"],
			"planner":   cloneMap(legacy),
			"formatter": cloneMap(legacy),
		}
	}
	for k, v := range in {
		switch k {
		case "planner", "formatter":
			next, _ := v.(map[string]interface{})
			if next == nil {
				continue
			}
			cur, _ := sec[k].(map[string]interface{})
			if cur == nil {
				cur = map[string]interface{}{}
			}
			mergeRoleConfig(cur, next)
			sec[k] = cur
		default:
			sec[k] = v
		}
	}
	m["llm"] = sec
	out, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal config yaml: %w", err)
	}
	return AtomicWriteFile(s.Path, out, 0o644)
}

func mergeRoleConfig(dst, src map[string]interface{}) {
	if dst == nil || src == nil {
		return
	}
	if v, ok := src["api_key"]; ok {
		if vs, ok := v.(string); ok && vs == "" {
			delete(src, "api_key")
		}
	}
	for k, v := range src {
		dst[k] = v
	}
}

func cloneMap(in map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range in {
		if child, ok := v.(map[string]interface{}); ok {
			out[k] = cloneMap(child)
			continue
		}
		out[k] = v
	}
	return out
}

func (s ConfigStore) loadMap() (map[string]interface{}, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	var m map[string]interface{}
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("unmarshal config yaml: %w", err)
	}
	if m == nil {
		m = map[string]interface{}{}
	}
	return m, nil
}
