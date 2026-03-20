package config

import (
	"fmt"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"webhook-relay/internal/domain"
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

type SourceConfig struct {
	ID     string `mapstructure:"id"`
	Type   string `mapstructure:"type"`
	Secret string `mapstructure:"secret"`
}

type ChannelConfig struct {
	ID            string `mapstructure:"id"`
	Type          string `mapstructure:"type"`
	URL           string `mapstructure:"url"`
	Template      string `mapstructure:"template"`
	RetryCount    int    `mapstructure:"retryCount"`
	RetryDelayMs  int    `mapstructure:"retryDelayMs"`
	SkipTLSVerify bool   `mapstructure:"skipTLSVerify"`
}

type RouteConfig struct {
	SourceID   string   `mapstructure:"sourceId"`
	ChannelIDs []string `mapstructure:"channelIds"`
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

type Config struct {
	Server   ServerConfig    `mapstructure:"server"`
	Log      LogConfig       `mapstructure:"log"`
	Sources  []SourceConfig  `mapstructure:"sources"`
	Channels []ChannelConfig `mapstructure:"channels"`
	Routes   []RouteConfig   `mapstructure:"routes"`
	Storage  StorageConfig   `mapstructure:"storage"`
	Queue    QueueConfig     `mapstructure:"queue"`
}

// Load 설정 파일을 읽어 Config를 반환한다. 템플릿 검증 포함.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	setDefaults(v)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return unmarshalAndValidate(v)
}

// Watch 설정 파일 변경 감지. channels/routes만 핫리로드. 유효하지 않으면 기존 유지.
func Watch(v *viper.Viper, onChange func(cfg *Config)) {
	v.WatchConfig()
	v.OnConfigChange(func(_ fsnotify.Event) {
		cfg, err := unmarshalAndValidate(v)
		if err != nil {
			return // 유효하지 않은 설정 무시
		}
		onChange(cfg)
	})
}

// NewViper path에서 viper 인스턴스를 반환한다 (Watch용).
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
	for _, ch := range cfg.Channels {
		if ch.Template == "" {
			continue
		}
		if err := domain.ValidateTemplate(ch.Template); err != nil {
			return nil, fmt.Errorf("channel %q: %w", ch.ID, err)
		}
	}
	return &cfg, nil
}
