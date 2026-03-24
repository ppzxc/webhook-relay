package config

import (
	"fmt"
	"log/slog"
	"net"

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

type StorageConfig struct {
	Type string `mapstructure:"type"`
	Path string `mapstructure:"path"`
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
	v.SetDefault("queue.workerCount", 2)
	v.SetDefault("worker.defaultRetryCount", 3)
	v.SetDefault("worker.defaultRetryDelay", "1s")
	v.SetDefault("worker.pollBackoff", "500ms")
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

func validateConfig(cfg *Config) error {
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
