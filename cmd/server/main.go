package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"

	httpadapter "relaybox/internal/adapter/input/http"
	"relaybox/internal/adapter/input/parser"
	"relaybox/internal/logging"
	tcpadapter "relaybox/internal/adapter/input/tcp"
	wsadapter "relaybox/internal/adapter/input/websocket"
	outputconfig "relaybox/internal/adapter/output/config"
	"relaybox/internal/adapter/output/expression"
	"relaybox/internal/adapter/output/filequeue"
	mariadbadapter "relaybox/internal/adapter/output/mariadb"
	sqliteadapter "relaybox/internal/adapter/output/sqlite"
	webhookadapter "relaybox/internal/adapter/output/webhook"
	"relaybox/internal/application/port/output"
	"relaybox/internal/application/service"
	cfgpkg "relaybox/internal/config"
	"relaybox/internal/domain"
)

var version = "dev"

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var cfgPath string
	root := &cobra.Command{Use: "relaybox", Short: "Generic relay hub — receives any inbound message, routes to outbound outputs"}
	start := &cobra.Command{
		Use:   "start",
		Short: "Start server",
		RunE:  func(_ *cobra.Command, _ []string) error { return runServer(cfgPath) },
	}
	start.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "config file path")
	root.AddCommand(start)
	root.AddCommand(&cobra.Command{
		Use: "version", Short: "Print version",
		Run: func(_ *cobra.Command, _ []string) { fmt.Println("relaybox " + version) },
	})
	root.AddCommand(secretCmd())
	return root
}

func secretCmd() *cobra.Command {
	secret := &cobra.Command{Use: "secret", Short: "Manage secrets"}
	var quiet bool
	var length int
	generate := &cobra.Command{
		Use:   "generate",
		Short: "Generate a cryptographically secure random secret",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := generateSecret(length)
			if err != nil {
				return err
			}
			if quiet {
				fmt.Fprintln(cmd.OutOrStdout(), s)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Secret: %s\n\nAdd to config.yaml under inputs[].secret or outputs[].secret\n", s)
			return nil
		},
	}
	generate.Flags().BoolVarP(&quiet, "quiet", "q", false, "print secret only (pipe-friendly)")
	generate.Flags().IntVarP(&length, "length", "l", 32, "number of random bytes (minimum 16)")
	secret.AddCommand(generate)
	return secret
}

func runServer(cfgPath string) error {
	cfg, err := cfgpkg.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logCloser := setupLogger(cfg)
	defer func() {
		if err := logCloser.Close(); err != nil {
			slog.Error("log file close error", "err", err)
		}
	}()
	slog.Info("starting relaybox",
		"version", version,
		"logLevel", cfg.Log.Level,
		"logStdoutEnabled", cfg.Log.Stdout.Enabled,
		"storageType", cfg.Storage.Type,
		"storageTableName", cfg.Storage.TableName,
		"queueWorkerCount", cfg.Queue.WorkerCount,
	)

	// Outbound adapters
	repo, repoCloser, err := newRepository(cfg.Storage)
	if err != nil {
		return fmt.Errorf("init repository: %w", err)
	}
	defer func() {
		if err := repoCloser.Close(); err != nil {
			slog.Error("repository close error", "err", err)
		}
	}()

	queue, err := filequeue.New(cfg.Queue.Path)
	if err != nil {
		return fmt.Errorf("init queue: %w", err)
	}

	sender := webhookadapter.NewSender()
	registry := webhookadapter.NewRegistry(map[domain.OutputType]output.OutputSender{
		domain.OutputTypeWebhook: sender,
	})

	// Expression engine registry
	exprRegistry := expression.NewInMemoryExpressionEngineRegistry()
	celEngine, err := expression.NewCELEngine()
	if err != nil {
		return fmt.Errorf("init CEL engine: %w", err)
	}
	exprEngine := expression.NewExprEngine()
	exprRegistry.Register(celEngine)
	exprRegistry.Register(exprEngine)

	// Config-based routing (hot-reload support)
	ruleReader := outputconfig.NewInMemoryRuleConfigReader(cfg)
	configQuerySvc := service.NewConfigQueryService(cfg)

	// Viper WatchConfig -> hot-reload
	v, err := cfgpkg.NewViper(cfgPath)
	if err != nil {
		return fmt.Errorf("init viper: %w", err)
	}
	cfgpkg.Watch(v, func(newCfg *cfgpkg.Config) {
		ruleReader.Update(newCfg)
		configQuerySvc.Update(newCfg)
		slog.Info("config reloaded")
	})

	// Parser registry
	parserRegistry := parser.NewInMemoryParserRegistry()
	parserRegistry.Register(parser.NewJSONParser())
	parserRegistry.Register(parser.NewFormParser())
	parserRegistry.Register(parser.NewXMLParser())
	parserRegistry.Register(parser.NewLogfmtParser())

	parserTypes := make(map[string]string)
	for _, inp := range cfg.Inputs {
		if inp.Parser == "" {
			continue
		}
		parserKey := inp.Parser
		if inp.Parser == "REGEX" {
			regexParser, err := parser.NewRegexParser(inp.Pattern)
			if err != nil {
				return fmt.Errorf("input %q: %w", inp.ID, err)
			}
			parserKey = "REGEX:" + inp.ID
			parserRegistry.RegisterWithKey(parserKey, regexParser)
		}
		parserTypes[inp.ID] = parserKey
	}

	// Application services
	msgSvc := service.NewMessageService(repo, queue, parserTypes, parserRegistry)
	workerCfg, err := buildRelayWorkerConfig(cfg.Worker)
	if err != nil {
		return fmt.Errorf("worker config: %w", err)
	}
	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, exprRegistry, workerCfg)

	var rotWorker *service.StorageRotationWorker
	if cfg.Storage.Rotation.Enabled {
		rotCfg, err := buildStorageRotationConfig(cfg.Storage.Rotation)
		if err != nil {
			return fmt.Errorf("rotation config: %w", err)
		}
		rotWorker = service.NewStorageRotationWorker(repo, rotCfg)
	}

	// HTTP + WebSocket adapter assembly
	resolver := newConfigInputResolver(cfg)
	wsHandler := wsadapter.NewHandler(msgSvc)
	router := httpadapter.NewRouter(msgSvc, msgSvc, msgSvc, msgSvc, configQuerySvc, resolver, wsHandler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker.Start(ctx, cfg.Queue.WorkerCount)
	if rotWorker != nil {
		rotWorker.Start(ctx)
	}

	// TCP listeners (per InputConfig with Address set)
	for _, inp := range cfg.Inputs {
		if inp.Address == "" {
			continue
		}
		delimiter := byte('\n')
		if inp.Delimiter != "" {
			delimiter = inp.Delimiter[0]
		}
		contentType := parserToContentType(inp.Parser)
		tcpL := tcpadapter.NewListener(msgSvc, inp.ID, inp.Address, delimiter, contentType)
		go func() {
			slog.Info("tcp listener starting", "address", inp.Address, "inputID", inp.ID)
			if err := tcpL.Start(ctx); err != nil {
				slog.Error("tcp listener error", "address", inp.Address, "err", err)
			}
		}()
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "port", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
		}
	}()

	<-sig
	slog.Info("shutting down")
	cancel()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()

	waitDone := make(chan struct{})
	go func() {
		worker.Wait()
		if rotWorker != nil {
			rotWorker.Wait()
		}
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-shutCtx.Done():
		slog.Warn("worker drain timed out, forcing shutdown")
	}
	return srv.Shutdown(shutCtx)
}

// configInputResolver Config-based InputResolver implementation
type configInputResolver struct {
	inputs  map[string]string
	secrets map[string]string
}

func newConfigInputResolver(cfg *cfgpkg.Config) *configInputResolver {
	inputs := make(map[string]string, len(cfg.Inputs))
	secrets := make(map[string]string, len(cfg.Inputs))
	for _, s := range cfg.Inputs {
		inputs[s.ID] = s.ID
		secrets[s.ID] = s.Secret
	}
	return &configInputResolver{inputs: inputs, secrets: secrets}
}

func (r *configInputResolver) Resolve(inputID string) (string, error) {
	st, ok := r.inputs[inputID]
	if !ok {
		return "", fmt.Errorf("resolve %q: %w", inputID, domain.ErrInputNotFound)
	}
	return st, nil
}

func (r *configInputResolver) ValidateToken(inputID, token string) bool {
	return r.secrets[inputID] == token
}

func parserToContentType(parserType string) string {
	switch parserType {
	case "JSON":
		return "application/json"
	case "XML":
		return "application/xml"
	case "FORM":
		return "application/x-www-form-urlencoded"
	case "LOGFMT":
		return "text/plain"
	case "REGEX", "":
		return "application/octet-stream"
	default:
		slog.Warn("unknown parser type for content-type mapping", "parser", parserType)
		return "application/octet-stream"
	}
}

func buildStorageRotationConfig(rc cfgpkg.RotationConfig) (service.StorageRotationConfig, error) {
	retention, err := time.ParseDuration(rc.Retention)
	if err != nil {
		return service.StorageRotationConfig{}, fmt.Errorf("invalid rotation.retention %q: %w", rc.Retention, err)
	}
	interval, err := time.ParseDuration(rc.Interval)
	if err != nil {
		return service.StorageRotationConfig{}, fmt.Errorf("invalid rotation.interval %q: %w", rc.Interval, err)
	}
	statuses := make([]domain.MessageStatus, 0, len(rc.Statuses))
	for _, s := range rc.Statuses {
		statuses = append(statuses, domain.MessageStatus(s))
	}
	return service.StorageRotationConfig{
		Retention: retention,
		Interval:  interval,
		Statuses:  statuses,
	}, nil
}

func buildRelayWorkerConfig(wc cfgpkg.WorkerConfig) (service.RelayWorkerConfig, error) {
	cfg := service.RelayWorkerConfig{DefaultRetryCount: wc.DefaultRetryCount}
	d, err := time.ParseDuration(wc.DefaultRetryDelay)
	if err != nil {
		return service.RelayWorkerConfig{}, fmt.Errorf("invalid worker.defaultRetryDelay %q: %w", wc.DefaultRetryDelay, err)
	}
	cfg.DefaultRetryDelay = d
	d, err = time.ParseDuration(wc.PollBackoff)
	if err != nil {
		return service.RelayWorkerConfig{}, fmt.Errorf("invalid worker.pollBackoff %q: %w", wc.PollBackoff, err)
	}
	cfg.PollBackoff = d
	return cfg, nil
}

// generateSecret returns a cryptographically random base64url-encoded secret of the given byte length.
// length must be at least 16.
func generateSecret(length int) (string, error) {
	if length < 16 {
		return "", fmt.Errorf("length must be at least 16, got %d", length)
	}
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// setupLogger는 cfg에 따라 slog 기본 로거를 설정하고,
// 파일 로거가 활성화된 경우 종료 시 Close()를 호출해야 할 Closer를 반환한다.
// 파일 로거가 없으면 no-op Closer를 반환한다.
func setupLogger(cfg *cfgpkg.Config) io.Closer {
	opts := &slog.HandlerOptions{Level: parseLogLevel(cfg.Log.Level)}
	var handlers []slog.Handler
	var fileCloser io.Closer = io.NopCloser(nil)

	if cfg.Log.Stdout.Enabled {
		format := resolveFormat(cfg.Log.Stdout.Format, cfg.Log.Format)
		handlers = append(handlers, newHandler(os.Stdout, format, opts))
	}

	if cfg.Log.File.Enabled {
		maxSize := cfg.Log.File.MaxSizeMB
		if maxSize == 0 {
			maxSize = 1024 * 1024 // 실질적 무제한 (1TB)
		}
		w := &lumberjack.Logger{
			Filename:   cfg.Log.File.Path,
			MaxSize:    maxSize,
			MaxBackups: cfg.Log.File.MaxBackups,
			MaxAge:     cfg.Log.File.MaxAgeDays,
			Compress:   cfg.Log.File.Compress,
		}
		fileCloser = w
		format := resolveFormat(cfg.Log.File.Format, cfg.Log.Format)
		handlers = append(handlers, newHandler(w, format, opts))
	}

	switch len(handlers) {
	case 0:
		// config.Validate()에서 이미 거부되므로 여기에 도달할 수 없음
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, opts)))
	case 1:
		slog.SetDefault(slog.New(handlers[0]))
	default:
		slog.SetDefault(slog.New(logging.NewMultiHandler(handlers...)))
	}

	return fileCloser
}

func resolveFormat(specific, fallback string) string {
	if specific != "" {
		return strings.ToUpper(specific)
	}
	return strings.ToUpper(fallback)
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func newHandler(w io.Writer, format string, opts *slog.HandlerOptions) slog.Handler {
	if strings.ToUpper(format) == "JSON" {
		return slog.NewJSONHandler(w, opts)
	}
	return slog.NewTextHandler(w, opts)
}

func newRepository(cfg cfgpkg.StorageConfig) (output.MessageRepository, io.Closer, error) {
	switch strings.ToUpper(cfg.Type) {
	case "SQLITE":
		repo, err := sqliteadapter.New(cfg.Path, cfg.TableName)
		if err != nil {
			return nil, nil, fmt.Errorf("sqlite repository: %w", err)
		}
		return repo, repo, nil
	case "MARIADB":
		lifetime, err := time.ParseDuration(cfg.ConnMaxLifetime)
		if err != nil && cfg.ConnMaxLifetime != "" {
			return nil, nil, fmt.Errorf("storage.connMaxLifetime: %w", err)
		}
		idleTime, err := time.ParseDuration(cfg.ConnMaxIdleTime)
		if err != nil && cfg.ConnMaxIdleTime != "" {
			return nil, nil, fmt.Errorf("storage.connMaxIdleTime: %w", err)
		}
		repo, err := mariadbadapter.New(mariadbadapter.Config{
			DSN:             cfg.DSN,
			MaxOpenConns:    cfg.MaxOpenConns,
			MaxIdleConns:    cfg.MaxIdleConns,
			ConnMaxLifetime: lifetime,
			ConnMaxIdleTime: idleTime,
			TableName:       cfg.TableName,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("mariadb repository: %w", err)
		}
		return repo, repo, nil
	default:
		return nil, nil, fmt.Errorf("unsupported storage type: %q", cfg.Type)
	}
}
