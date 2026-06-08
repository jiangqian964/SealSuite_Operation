// Package config 提供配置管理功能
// 支持从 YAML 文件加载配置，并支持环境变量覆盖
package config

import (
	"fmt"  // 格式化输出

	"github.com/spf13/viper"  // 第三方配置管理库
)

// Config 是应用程序的总配置结构
// 包含所有子系统的配置项
type Config struct {
	SealSuite SealSuiteConfig `mapstructure:"sealsuite"`  // SealSuite API 配置
	LLM       LLMConfig       `mapstructure:"llm"`        // 大模型配置
	Scheduler SchedulerConfig `mapstructure:"scheduler"`  // 定时任务调度器配置
	Log       LogConfig       `mapstructure:"log"`        // 日志系统配置
	Server    ServerConfig    `mapstructure:"server"`     // HTTP 服务配置（预留）
}

// SealSuiteConfig 定义了 SealSuite API 客户端的配置项
// 包含连接认证和请求参数
type SealSuiteConfig struct {
	// BaseURL 为兼容旧配置保留；当 Scheme/Host/Port 填写时，建议优先用结构化字段组装 base_url。
	BaseURL    string `mapstructure:"base_url"`    // API 基础地址，例如：https://api.example.com
	Scheme     string `mapstructure:"scheme"`      // https/http
	Host       string `mapstructure:"host"`        // 域名或 IP
	Port       int    `mapstructure:"port"`        // 端口（0 表示使用 scheme 默认端口或由 BaseURL 决定）
	AccessKey  string `mapstructure:"access_key"`  // API 访问密钥 ID
	SecretKey  string `mapstructure:"secret_key"`  // API 访问密钥密码
	Timeout    int    `mapstructure:"timeout"`     // 请求超时时间（秒）
	RetryTimes int    `mapstructure:"retry_times"` // 失败重试次数
	MockMode   bool   `mapstructure:"mock_mode"`   // 是否启用模拟模式
}

type LLMRoleConfig struct {
	Enabled         bool                   `mapstructure:"enabled"`
	Provider        string                 `mapstructure:"provider"`
	BaseURL         string                 `mapstructure:"base_url"`
	APIKey          string                 `mapstructure:"api_key"`
	Model           string                 `mapstructure:"model"`
	Timeout         int                    `mapstructure:"timeout"`
	MockMode        bool                   `mapstructure:"mock_mode"`
	SystemPrompt    string                 `mapstructure:"system_prompt"`
	Temperature     float64                `mapstructure:"temperature"`
	MaxTokens       int                    `mapstructure:"max_tokens"`
	Thinking        bool                   `mapstructure:"thinking"`
	ReasoningEffort string                 `mapstructure:"reasoning_effort"`
	ResponseFormat  map[string]interface{} `mapstructure:"response_format"`
}

// LegacyLLMConfig 用于兼容历史单模型配置。
type LegacyLLMConfig struct {
	Enabled      bool    `mapstructure:"enabled"`
	Provider     string  `mapstructure:"provider"`
	BaseURL      string  `mapstructure:"base_url"`
	APIKey       string  `mapstructure:"api_key"`
	Model        string  `mapstructure:"model"`
	Timeout      int     `mapstructure:"timeout"`
	MockMode     bool    `mapstructure:"mock_mode"`
	SystemPrompt string  `mapstructure:"system_prompt"`
	Temperature  float64 `mapstructure:"temperature"`
	MaxTokens    int     `mapstructure:"max_tokens"`
	Thinking     bool    `mapstructure:"thinking"`
	ReasoningEffort string `mapstructure:"reasoning_effort"`
}

// LLMConfig 定义双角色大模型配置。
type LLMConfig struct {
	MockMode  bool          `mapstructure:"mock_mode"`
	Planner   LLMRoleConfig `mapstructure:"planner"`
	Formatter LLMRoleConfig `mapstructure:"formatter"`
}

// SchedulerConfig 定义了定时任务调度器的配置项
type SchedulerConfig struct {
	Enabled     bool   `mapstructure:"enabled"`    // 是否启用调度器
	Timezone    string `mapstructure:"timezone"`   // 时区设置，例如：Asia/Shanghai
}

// LogConfig 定义了日志系统的配置项
// 支持日志分级和文件滚动
type LogConfig struct {
	Level      string `mapstructure:"level"`        // 日志级别：debug, info, warn, error
	Filename   string `mapstructure:"filename"`     // 日志文件路径
	MaxSize    int    `mapstructure:"max_size"`     // 单个日志文件最大大小（MB）
	MaxBackups int    `mapstructure:"max_backups"`  // 保留的旧日志文件数量
	MaxAge     int    `mapstructure:"max_age"`      // 保留旧日志文件的最长天数
}

// ServerConfig 定义了 HTTP 服务的配置项（预留扩展用）
type ServerConfig struct {
	Bind string `mapstructure:"bind"`    // 服务监听地址，例如：127.0.0.1
	Port int    `mapstructure:"port"`    // 服务监听端口
	Mode string `mapstructure:"mode"`    // 运行模式：debug, release
}

// globalConfig 是全局配置单例
// 通过 Load 函数初始化后，可以通过 Get 函数获取
var globalConfig *Config

// Load 从指定路径加载配置文件
// 参数:
//   configPath - 配置文件路径，例如：config.yaml
// 返回:
//   *Config - 加载成功的配置对象
//   error - 加载失败时的错误信息
func Load(configPath string) (*Config, error) {
	// 设置配置文件路径
	viper.SetConfigFile(configPath)
	// 设置配置文件类型为 YAML
	viper.SetConfigType("yaml")

	// 启用自动环境变量绑定
	// 环境变量命名规则：将配置名转为大写，用下划线分隔，例如：SEALSUITE_BASE_URL
	viper.AutomaticEnv()

	// 读取配置文件内容
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// 初始化全局配置对象
	globalConfig = &Config{}
	// 将配置文件内容反序列化为 Config 结构体
	if err := viper.Unmarshal(globalConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	normalizeLLMConfig(viper.GetViper(), globalConfig)

	// 返回配置对象
	return globalConfig, nil
}

// Get 获取全局配置对象
// 必须先调用 Load 函数初始化后才能使用
// 返回:
//   *Config - 全局配置对象
func Get() *Config {
	return globalConfig
}

func normalizeLLMConfig(v *viper.Viper, cfg *Config) {
	if cfg == nil {
		return
	}
	if cfg.LLM.Planner.Provider == "" && cfg.LLM.Formatter.Provider == "" {
		var legacy LegacyLLMConfig
		if err := v.UnmarshalKey("llm", &legacy); err == nil {
			if legacy.Provider != "" || legacy.BaseURL != "" || legacy.Model != "" || legacy.APIKey != "" {
				role := LLMRoleConfig{
					Enabled:         legacy.Enabled,
					Provider:        legacy.Provider,
					BaseURL:         legacy.BaseURL,
					APIKey:          legacy.APIKey,
					Model:           legacy.Model,
					Timeout:         legacy.Timeout,
					MockMode:        legacy.MockMode,
					SystemPrompt:    legacy.SystemPrompt,
					Temperature:     legacy.Temperature,
					MaxTokens:       legacy.MaxTokens,
					Thinking:        legacy.Thinking,
					ReasoningEffort: legacy.ReasoningEffort,
				}
				cfg.LLM.Formatter = role
				cfg.LLM.Planner = role
				if cfg.LLM.Planner.Provider == "" {
					cfg.LLM.Planner.Provider = "deepseek"
				}
				if cfg.LLM.Formatter.Provider == "" {
					cfg.LLM.Formatter.Provider = "volcengine_ark"
				}
				cfg.LLM.MockMode = legacy.MockMode
			}
		}
	}
	if cfg.LLM.Planner.Provider == "" {
		cfg.LLM.Planner.Provider = "deepseek"
	}
	cfg.LLM.Planner.MockMode = cfg.LLM.MockMode
	if cfg.LLM.Formatter.Provider == "" {
		cfg.LLM.Formatter.Provider = "volcengine_ark"
	}
	cfg.LLM.Formatter.MockMode = cfg.LLM.MockMode
}
