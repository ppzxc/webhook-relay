package config

import (
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

type ServerConfig struct {
	Port         int       `mapstructure:"port"`
	ReadTimeout  string    `mapstructure:"readTimeout"`
	WriteTimeout string    `mapstructure:"writeTimeout"`
	TLS          TLSConfig `mapstructure:"tls"`
}

type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	CertFile string `mapstructure:"certFile"`
	KeyFile  string `mapstructure:"keyFile"`
}

type LogConfig struct {
	Level  string          `mapstructure:"level"`
	Format string          `mapstructure:"format"` // 하위 호환: stdout/file format 미설정 시 fallback
	Stdout LogStdoutConfig `mapstructure:"stdout"`
	File   LogFileConfig   `mapstructure:"file"`
}

type LogStdoutConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Format  string `mapstructure:"format"` // JSON, TEXT; 빈 문자열이면 log.format 상속
}

type LogFileConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Format     string `mapstructure:"format"` // JSON, TEXT
	Path       string `mapstructure:"path"`
	MaxSizeMB  int    `mapstructure:"maxSizeMB"`  // 0 = 무제한 (내부적으로 1TB)
	MaxBackups int    `mapstructure:"maxBackups"` // 보관할 이전 파일 수
	MaxAgeDays int    `mapstructure:"maxAgeDays"` // 최대 보존 일수 (0 = 무제한)
	Compress   bool   `mapstructure:"compress"`   // 이전 파일 gzip 압축
}

type InputConfig struct {
	ID        string       `mapstructure:"id"`
	Engine    string       `mapstructure:"engine"`   // "CEL" or "EXPR"; required
	Parser    string       `mapstructure:"parser"`   // "JSON", "FORM", "XML", "LOGFMT", "REGEX"
	Secret    string       `mapstructure:"secret"`
	Pattern   string       `mapstructure:"pattern"`   // for REGEX parser
	Address   string       `mapstructure:"address"`   // TCP: bind address, e.g. ":9000"
	Delimiter string       `mapstructure:"delimiter"` // TCP: message delimiter, default "\n"
	Rules     []RuleConfig `mapstructure:"rules"`
}

type OutputConfig struct {
	ID            string            `mapstructure:"id"`
	Type          string            `mapstructure:"type"`
	Engine        string            `mapstructure:"engine"` // "CEL" or "EXPR"; required
	URL           string            `mapstructure:"url"`
	Template      map[string]string `mapstructure:"template"`
	Secret        string            `mapstructure:"secret"`
	RetryCount    int               `mapstructure:"retryCount"`
	RetryDelayMs  int               `mapstructure:"retryDelayMs"`
	TimeoutSec    int               `mapstructure:"timeoutSec"`
	SkipTLSVerify bool              `mapstructure:"skipTLSVerify"`
}

type RouteConditionConfig struct {
	Condition string   `mapstructure:"condition"`
	OutputIDs []string `mapstructure:"outputIds"`
}

type RuleConfig struct {
	OutputIDs []string               `mapstructure:"outputIds"`
	Filter    string                 `mapstructure:"filter"`
	Mapping   map[string]string      `mapstructure:"mapping"`
	Routing   []RouteConditionConfig `mapstructure:"routing"`
}

type RotationConfig struct {
	Enabled   bool     `mapstructure:"enabled"`
	Retention string   `mapstructure:"retention"` // Go duration, e.g. "720h"; must be positive
	Interval  string   `mapstructure:"interval"`  // Go duration, e.g. "1h"; must be positive
	// Statuses는 삭제 대상 메시지 상태 목록 (DELIVERED, FAILED, PENDING 허용).
	// 비어있으면 모든 상태 삭제.
	// 주의: PENDING 포함 시 아직 처리되지 않은 메시지도 삭제됨.
	Statuses []string `mapstructure:"statuses"`
}

type StorageConfig struct {
	Type            string         `mapstructure:"type"`
	Path            string         `mapstructure:"path"`            // SQLite 파일 경로
	DSN             string         `mapstructure:"dsn"`             // MariaDB/MySQL DSN
	MaxOpenConns    int            `mapstructure:"maxOpenConns"`
	MaxIdleConns    int            `mapstructure:"maxIdleConns"`
	ConnMaxLifetime string         `mapstructure:"connMaxLifetime"` // Go duration, e.g. "5m"
	ConnMaxIdleTime string         `mapstructure:"connMaxIdleTime"` // Go duration, e.g. "3m"
	TableName       string         `mapstructure:"tableName"`       // 메시지 테이블 이름 (기본값: messages)
	Rotation        RotationConfig `mapstructure:"rotation"`
}

type QueueConfig struct {
	Type        string `mapstructure:"type"`
	Path        string `mapstructure:"path"`
	WorkerCount int    `mapstructure:"workerCount"`
}

type WorkerConfig struct {
	DefaultRetryCount int    `mapstructure:"defaultRetryCount"`
	DefaultRetryDelay string `mapstructure:"defaultRetryDelay"` // Go duration string, e.g. "1s"
	PollBackoff       string `mapstructure:"pollBackoff"`       // Go duration string, e.g. "500ms"
}

type Config struct {
	Server  ServerConfig   `mapstructure:"server"`
	Log     LogConfig      `mapstructure:"log"`
	Inputs  []InputConfig  `mapstructure:"inputs"`
	Outputs []OutputConfig `mapstructure:"outputs"`
	Storage StorageConfig  `mapstructure:"storage"`
	Queue   QueueConfig    `mapstructure:"queue"`
	Worker  WorkerConfig   `mapstructure:"worker"`
}

// Load reads and validates the config file.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	setDefaults(v)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return unmarshalAndValidate(v)
}

// Watch detects config file changes. Only inputs/outputs are hot-reloaded.
func Watch(v *viper.Viper, onChange func(cfg *Config)) {
	v.WatchConfig()
	v.OnConfigChange(func(_ fsnotify.Event) {
		cfg, err := unmarshalAndValidate(v)
		if err != nil {
			slog.Warn("config reload rejected: invalid config", "err", err)
			return
		}
		onChange(cfg)
	})
}

// NewViper returns a viper instance for Watch.
func NewViper(path string) (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigFile(path)
	setDefaults(v)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return v, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.readTimeout", "30s")
	v.SetDefault("server.writeTimeout", "30s")
	v.SetDefault("log.level", "INFO")
	v.SetDefault("log.format", "JSON")
	v.SetDefault("log.stdout.enabled", true)
	v.SetDefault("log.stdout.format", "")
	v.SetDefault("log.file.enabled", false)
	v.SetDefault("log.file.format", "JSON")
	v.SetDefault("log.file.path", "./data/relaybox.log")
	v.SetDefault("log.file.maxSizeMB", 100)
	v.SetDefault("log.file.maxBackups", 5)
	v.SetDefault("log.file.maxAgeDays", 30)
	v.SetDefault("log.file.compress", true)
	v.SetDefault("queue.workerCount", 2)
	v.SetDefault("worker.defaultRetryCount", 3)
	v.SetDefault("worker.defaultRetryDelay", "1s")
	v.SetDefault("worker.pollBackoff", "500ms")
	v.SetDefault("storage.tableName", "messages")
	v.SetDefault("storage.rotation.enabled", false)
	v.SetDefault("storage.rotation.retention", "720h")
	v.SetDefault("storage.rotation.interval", "1h")
}

func unmarshalAndValidate(v *viper.Viper) (*Config, error) {
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

var validEngines = map[string]struct{}{"CEL": {}, "EXPR": {}}
var validTableName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,63}$`)

var validLogFormats = map[string]struct{}{"JSON": {}, "TEXT": {}, "": {}}

func validateConfig(cfg *Config) error {
	// log 검증
	if !cfg.Log.Stdout.Enabled && !cfg.Log.File.Enabled {
		return fmt.Errorf("log: at least one of stdout or file must be enabled")
	}
	if _, ok := validLogFormats[strings.ToUpper(cfg.Log.Stdout.Format)]; !ok {
		return fmt.Errorf("log.stdout.format: invalid value %q (valid: JSON, TEXT)", cfg.Log.Stdout.Format)
	}
	if _, ok := validLogFormats[strings.ToUpper(cfg.Log.File.Format)]; !ok {
		return fmt.Errorf("log.file.format: invalid value %q (valid: JSON, TEXT)", cfg.Log.File.Format)
	}
	if cfg.Log.File.Enabled && cfg.Log.File.Path == "" {
		return fmt.Errorf("log.file.path: must not be empty when file logging is enabled")
	}
	if cfg.Log.File.MaxSizeMB < 0 {
		return fmt.Errorf("log.file.maxSizeMB: must be non-negative, got %d", cfg.Log.File.MaxSizeMB)
	}

	if strings.ToUpper(cfg.Storage.Type) == "MARIADB" && cfg.Storage.DSN == "" {
		return fmt.Errorf("storage.dsn is required when storage.type is MARIADB")
	}
	if cfg.Storage.TableName != "" && !validTableName.MatchString(cfg.Storage.TableName) {
		return fmt.Errorf("storage.tableName: invalid name %q (must match ^[a-zA-Z_][a-zA-Z0-9_]{0,63}$)", cfg.Storage.TableName)
	}
	if cfg.Storage.ConnMaxLifetime != "" {
		if _, err := time.ParseDuration(cfg.Storage.ConnMaxLifetime); err != nil {
			return fmt.Errorf("storage.connMaxLifetime: invalid duration %q: %w", cfg.Storage.ConnMaxLifetime, err)
		}
	}
	if cfg.Storage.ConnMaxIdleTime != "" {
		if _, err := time.ParseDuration(cfg.Storage.ConnMaxIdleTime); err != nil {
			return fmt.Errorf("storage.connMaxIdleTime: invalid duration %q: %w", cfg.Storage.ConnMaxIdleTime, err)
		}
	}
	if rot := cfg.Storage.Rotation; rot.Retention != "" {
		d, err := time.ParseDuration(rot.Retention)
		if err != nil {
			return fmt.Errorf("rotation.retention: invalid duration %q: %w", rot.Retention, err)
		}
		if d <= 0 {
			return fmt.Errorf("rotation.retention: must be positive, got %q", rot.Retention)
		}
	}
	if rot := cfg.Storage.Rotation; rot.Interval != "" {
		d, err := time.ParseDuration(rot.Interval)
		if err != nil {
			return fmt.Errorf("rotation.interval: invalid duration %q: %w", rot.Interval, err)
		}
		if d <= 0 {
			return fmt.Errorf("rotation.interval: must be positive, got %q", rot.Interval)
		}
	}
	for _, s := range cfg.Storage.Rotation.Statuses {
		if s != "PENDING" && s != "DELIVERED" && s != "FAILED" {
			return fmt.Errorf("rotation.statuses: invalid status %q (valid: PENDING, DELIVERED, FAILED)", s)
		}
	}

	// Build output ID set for reference checks
	seenOutputs := make(map[string]struct{}, len(cfg.Outputs))
	for _, c := range cfg.Outputs {
		if c.ID == "" {
			return fmt.Errorf("output ID must not be empty")
		}
		if _, dup := seenOutputs[c.ID]; dup {
			return fmt.Errorf("duplicate output ID %q", c.ID)
		}
		seenOutputs[c.ID] = struct{}{}
		if c.Engine == "" {
			return fmt.Errorf("output %q: engine must not be empty", c.ID)
		}
		if _, ok := validEngines[c.Engine]; !ok {
			return fmt.Errorf("output %q: unsupported engine %q (valid: CEL, EXPR)", c.ID, c.Engine)
		}
	}

	seenInputs := make(map[string]struct{}, len(cfg.Inputs))
	for _, inp := range cfg.Inputs {
		if inp.ID == "" {
			return fmt.Errorf("input ID must not be empty")
		}
		if _, dup := seenInputs[inp.ID]; dup {
			return fmt.Errorf("duplicate input ID %q", inp.ID)
		}
		seenInputs[inp.ID] = struct{}{}
		if inp.Engine == "" {
			return fmt.Errorf("input %q: engine must not be empty", inp.ID)
		}
		if _, ok := validEngines[inp.Engine]; !ok {
			return fmt.Errorf("input %q: unsupported engine %q (valid: CEL, EXPR)", inp.ID, inp.Engine)
		}
		if inp.Address != "" {
			if _, err := net.ResolveTCPAddr("tcp", inp.Address); err != nil {
				return fmt.Errorf("input %q: invalid TCP address %q: %w", inp.ID, inp.Address, err)
			}
		}
		if inp.Address != "" && inp.Delimiter != "" && len(inp.Delimiter) > 1 {
			return fmt.Errorf("input %q: delimiter must be a single character, got %q", inp.ID, inp.Delimiter)
		}
		// Validate rule output references
		for _, rule := range inp.Rules {
			for _, outID := range rule.OutputIDs {
				if _, ok := seenOutputs[outID]; !ok {
					return fmt.Errorf("input %q: rule references unknown output %q", inp.ID, outID)
				}
			}
			for _, rc := range rule.Routing {
				for _, outID := range rc.OutputIDs {
					if _, ok := seenOutputs[outID]; !ok {
						return fmt.Errorf("input %q: routing condition references unknown output %q", inp.ID, outID)
					}
				}
			}
		}
	}
	return nil
}
