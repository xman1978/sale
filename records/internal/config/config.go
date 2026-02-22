package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 系统配置结构
type Config struct {
	Database Database `yaml:"database"`
	Feishu   Feishu   `yaml:"feishu"`
	AI       AI       `yaml:"ai"`
	Server   Server   `yaml:"server"`
	Logging  Logging  `yaml:"logging"`
	System   System   `yaml:"system"`
	Prompts  Prompts  `yaml:"prompts"`
	Messages Messages `yaml:"messages"`
}

// Database 数据库配置
type Database struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	User            string        `yaml:"user"`
	Schema          string        `yaml:"schema"`
	Password        string        `yaml:"password"`
	DBName          string        `yaml:"dbname"`
	SSLMode         string        `yaml:"sslmode"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

// Feishu 飞书配置
type Feishu struct {
	AppID             string `yaml:"app_id"`
	AppSecret         string `yaml:"app_secret"`
	VerificationToken string `yaml:"verification_token"`
	EncryptKey        string `yaml:"encrypt_key"`
}

// AI AI模型配置
type AI struct {
	OpenAI   OpenAI   `yaml:"openai"`
	Semantic Semantic `yaml:"semantic"`
	Dialogue Dialogue `yaml:"dialogue"`
}

// OpenAI OpenAI配置
type OpenAI struct {
	APIKey              string  `yaml:"api_key"`
	BaseURL             string  `yaml:"base_url"`
	ModelName           string  `yaml:"model_name"`
	Temperature         float64 `yaml:"temperature"`
	MaxCompletionTokens int64   `yaml:"max_completion_tokens"`
	TopP                float64 `yaml:"top_p"`
	FrequencyPenalty    float64 `yaml:"frequency_penalty"`
}

// Semantic 语义提取模型配置
type Semantic struct {
	ModelName           string  `yaml:"model_name"`
	Temperature         float64 `yaml:"temperature"`
	MaxCompletionTokens int64   `yaml:"max_completion_tokens"`
}

// Dialogue 对话生成模型配置
type Dialogue struct {
	ModelName           string  `yaml:"model_name"`
	Temperature         float64 `yaml:"temperature"`
	MaxCompletionTokens int64   `yaml:"max_completion_tokens"`
}

// Server 服务器配置
type Server struct {
	Port           int           `yaml:"port"`
	Host           string        `yaml:"host"`
	ReadTimeout    time.Duration `yaml:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout"`
	IdleTimeout   time.Duration `yaml:"idle_timeout"`
	StaticDir     string        `yaml:"static_dir"`      // 静态页面目录，相对于工作目录
	APIPrefix     string        `yaml:"api_prefix"`     // API 接口路径前缀，如 /api
	WebPrefix     string        `yaml:"web_prefix"`     // Web 页面路径前缀，如 / 或 /page
	JWTSecret     string        `yaml:"jwt_secret"`     // JWT 签名密钥，用于 page API 会话；空则回退到 x-user-id（不安全）
	AllowDemoUser bool          `yaml:"allow_demo_user"` // 是否允许 demo_user 回退（开发环境 true，生产环境 false）
}

// Logging 日志配置
type Logging struct {
	Level      string `yaml:"level"`
	Caller     bool   `yaml:"caller"`
	Format     string `yaml:"format"`
	Output     string `yaml:"output"`
	FilePath   string `yaml:"file_path"`
	MaxSize    string `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`
}

// System 系统配置
type System struct {
	SessionTimeout        int           `yaml:"session_timeout"`
	MaxConcurrentSessions int           `yaml:"max_concurrent_sessions"`
	MaxRetries            int           `yaml:"max_retries"`
	RetryInterval         time.Duration `yaml:"retry_interval"`
}

// Prompts 提示词配置
type Prompts struct {
	IsCustomerFollowRelated    string `yaml:"is_customer_follow_related"`
	IsUserConfirmation         string `yaml:"is_user_confirmation"`
	IsUserNoMoreCustomers      string `yaml:"is_user_no_more_customers"`
	SemanticAnalysis           string `yaml:"semantic_analysis"`
	DialogueCollecting      string `yaml:"dialogue_collecting"`
	DialogueConfirming      string `yaml:"dialogue_confirming"`
	CustomerSummary         string `yaml:"customer_summary"`
	EntityNormalization     string `yaml:"entity_normalization"`
}

type Messages struct {
	NewUser              string `yaml:"new_user"`
	WelcomeBack          string `yaml:"welcome_back"`
	ContinueSession      string `yaml:"continue_session"`
	NewDialog            string `yaml:"new_dialog"`
	AskingOtherCustomers string `yaml:"asking_other_customers"`
	OutputtingConfirm    string `yaml:"outputting_confirm"`
	OutputtingEnded      string `yaml:"outputting_ended"` // OUTPUTTING 阶段用户继续发消息且非跟进信息时的友好提示
	SystemError          string `yaml:"system_error"`
	ProcessError         string `yaml:"process_error"`
}

// Load 加载配置文件
func Load(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
