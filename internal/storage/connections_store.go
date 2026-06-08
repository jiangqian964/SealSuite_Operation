package storage

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type ConnectionsFile struct {
	Version  int              `yaml:"version"`
	ActiveID string           `yaml:"active_id"`
	Items    []ConnectionItem `yaml:"items"`
}

type ConnectionItem struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Scheme      string `yaml:"scheme"`
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	AccessKeyID string `yaml:"access_key_id"`
	SecretRef   string `yaml:"secret_ref"`
	CreatedAt   string `yaml:"created_at"`
}

type ConnectionsStore struct {
	Path string
}

func (s ConnectionsStore) Load() (*ConnectionsFile, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ConnectionsFile{Version: 1, Items: []ConnectionItem{}}, nil
		}
		return nil, fmt.Errorf("read connections file: %w", err)
	}
	var cf ConnectionsFile
	if err := yaml.Unmarshal(b, &cf); err != nil {
		return nil, fmt.Errorf("unmarshal connections yaml: %w", err)
	}
	if cf.Version == 0 {
		cf.Version = 1
	}
	if cf.Items == nil {
		cf.Items = []ConnectionItem{}
	}
	return &cf, nil
}

func (s ConnectionsStore) Save(cf *ConnectionsFile) error {
	if cf == nil {
		return fmt.Errorf("connections file is nil")
	}
	if cf.Version == 0 {
		cf.Version = 1
	}
	b, err := yaml.Marshal(cf)
	if err != nil {
		return fmt.Errorf("marshal connections yaml: %w", err)
	}
	return AtomicWriteFile(s.Path, b, 0o644)
}

// AddAndActivate 追加一条连接（去重：按 ID），并将其设为 active，
// 同时裁剪为最近 3 条（保留顺序为旧 -> 新）。
func (s ConnectionsStore) AddAndActivate(item ConnectionItem) error {
	cf, err := s.Load()
	if err != nil {
		return err
	}
	// remove existing same id
	out := make([]ConnectionItem, 0, len(cf.Items)+1)
	for _, it := range cf.Items {
		if it.ID != item.ID {
			out = append(out, it)
		}
	}
	out = append(out, item)
	if len(out) > 3 {
		out = out[len(out)-3:]
	}
	cf.Items = out
	cf.ActiveID = item.ID
	return s.Save(cf)
}

func (s ConnectionsStore) Activate(id string) (*ConnectionItem, error) {
	cf, err := s.Load()
	if err != nil {
		return nil, err
	}
	for _, it := range cf.Items {
		if it.ID == id {
			cf.ActiveID = id
			if err := s.Save(cf); err != nil {
				return nil, err
			}
			cp := it
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("connection not found: %s", id)
}

func (s ConnectionsStore) Delete(id string) error {
	cf, err := s.Load()
	if err != nil {
		return err
	}
	if cf.ActiveID == id {
		return fmt.Errorf("cannot delete active connection")
	}
	out := make([]ConnectionItem, 0, len(cf.Items))
	found := false
	for _, it := range cf.Items {
		if it.ID == id {
			found = true
			continue
		}
		out = append(out, it)
	}
	if !found {
		return fmt.Errorf("connection not found: %s", id)
	}
	cf.Items = out
	return s.Save(cf)
}
