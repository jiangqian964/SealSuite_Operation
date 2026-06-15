package storage

type OutputTemplatesFile struct {
	Version int              `yaml:"version" json:"version"`
	Items   []OutputTemplate `yaml:"items" json:"items"`
}

type OutputTemplate struct {
	ID                  string                 `yaml:"id" json:"id"`
	Name                string                 `yaml:"name" json:"name"`
	Format              string                 `yaml:"format" json:"format"`
	Title               string                 `yaml:"title,omitempty" json:"title,omitempty"`
	Content             string                 `yaml:"content,omitempty" json:"content,omitempty"`
	SourceComplexTaskID string                 `yaml:"source_complex_task_id,omitempty" json:"source_complex_task_id,omitempty"`
	OutputConfig        map[string]interface{} `yaml:"output_config,omitempty" json:"output_config,omitempty"`
	Meta                map[string]interface{} `yaml:"meta,omitempty" json:"meta,omitempty"`
}
