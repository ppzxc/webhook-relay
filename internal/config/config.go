package config

import (
	"fmt"
	"log/slog"

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
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type InputConfig struct {
	ID      string `mapstructure:"id"`
	Type    string `mapstructure:"type"`
	Parser  string `mapstructure:"parser"`
	Secret  string `mapstructure:"secret"`
	Pattern string `mapstructure:"pattern"` // for regex parser
}

type OutputConfig struct {
	ID            string            `mapstructure:"id"`
	Type          string            `mapstructure:"type"`
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
	InputID   string                 `mapstructure:"inputId"`
	OutputIDs []string               `mapstructure:"outputIds"`
	Engine    string                 `mapstructure:"engine"`
	Filter    string                 `mapstructure:"filter"`
	Mapping   map[string]string      `mapstructure:"mapping"`
	Routing   []RouteConditionConfig `mapstructure:"routing"`
}

type StorageConfig struct {
	Type string `mapstructure:"type"`
	Path string `mapstructure:"path"`
}

type QueueConfig struct {
	Type        string `mapstructure:"type"`
	Path        string `mapstructure:"path"`
	WorkerCount int    `mapstructure:"workerCount"`
}

type ExpressionConfig struct {
	DefaultEngine string `mapstructure:"defaultEngine"` // "cel" or "expr"
}

type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Log        LogConfig        `mapstructure:"log"`
	Inputs     []InputConfig    `mapstructure:"inputs"`
	Outputs    []OutputConfig   `mapstructure:"outputs"`
	Rules      []RuleConfig     `mapstructure:"rules"`
	Storage    StorageConfig    `mapstructure:"storage"`
	Queue      QueueConfig      `mapstructure:"queue"`
	Expression ExpressionConfig `mapstructure:"expression"`
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

// Watch detects config file changes. Only outputs/rules are hot-reloaded.
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
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
	v.SetDefault("queue.workerCount", 2)
}

func unmarshalAndValidate(v *viper.Viper) (*Config, error) {
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := validateIDs(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func validateIDs(cfg *Config) error {
	seenInputs := make(map[string]struct{}, len(cfg.Inputs))
	for _, s := range cfg.Inputs {
		if s.ID == "" {
			return fmt.Errorf("input ID must not be empty")
		}
		if _, dup := seenInputs[s.ID]; dup {
			return fmt.Errorf("duplicate input ID %q", s.ID)
		}
		seenInputs[s.ID] = struct{}{}
	}

	seenOutputs := make(map[string]struct{}, len(cfg.Outputs))
	for _, c := range cfg.Outputs {
		if c.ID == "" {
			return fmt.Errorf("output ID must not be empty")
		}
		if _, dup := seenOutputs[c.ID]; dup {
			return fmt.Errorf("duplicate output ID %q", c.ID)
		}
		seenOutputs[c.ID] = struct{}{}
	}

	for _, rt := range cfg.Rules {
		for _, outID := range rt.OutputIDs {
			if _, ok := seenOutputs[outID]; !ok {
				return fmt.Errorf("rule for input %q references unknown output %q", rt.InputID, outID)
			}
		}
	}
	return nil
}
