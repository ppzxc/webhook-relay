package main

import (
	"context"
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
	httpadapter "relaybox/internal/adapter/input/http"
	"relaybox/internal/adapter/input/parser"
	tcpadapter "relaybox/internal/adapter/input/tcp"
	wsadapter "relaybox/internal/adapter/input/websocket"
	"relaybox/internal/adapter/output/expression"
	"relaybox/internal/adapter/output/filequeue"
	sqliteadapter "relaybox/internal/adapter/output/sqlite"
	webhookadapter "relaybox/internal/adapter/output/webhook"
	"relaybox/internal/application/port/output"
	"relaybox/internal/application/service"
	cfgpkg "relaybox/internal/config"
	"relaybox/internal/domain"
)

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
		Run: func(_ *cobra.Command, _ []string) { fmt.Println("relaybox v0.2.0") },
	})
	return root
}

func runServer(cfgPath string) error {
	cfg, err := cfgpkg.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	setupLogger(cfg)

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
	if cfg.Expression.DefaultEngine != "" {
		if err := exprRegistry.SetDefault(cfg.Expression.DefaultEngine); err != nil {
			return fmt.Errorf("set default expression engine: %w", err)
		}
	}

	// Config-based routing (hot-reload support)
	ruleReader := cfgpkg.NewInMemoryRuleConfigReader(cfg)

	// Viper WatchConfig -> hot-reload
	v, err := cfgpkg.NewViper(cfgPath)
	if err != nil {
		return fmt.Errorf("init viper: %w", err)
	}
	cfgpkg.Watch(v, func(newCfg *cfgpkg.Config) {
		ruleReader.Update(newCfg)
		slog.Info("config reloaded")
	})

	// Parser registry
	parserRegistry := parser.NewInMemoryParserRegistry()
	parserRegistry.Register(parser.NewJSONParser())
	parserRegistry.Register(parser.NewFormParser())
	parserRegistry.Register(parser.NewXMLParser())
	parserRegistry.Register(parser.NewLogfmtParser())

	parserTypes := make(map[domain.InputType]string)
	for _, inp := range cfg.Inputs {
		if inp.Parser == "" {
			continue
		}
		parserKey := inp.Parser
		if inp.Parser == "regex" {
			regexParser, err := parser.NewRegexParser(inp.Pattern)
			if err != nil {
				return fmt.Errorf("input %q: %w", inp.ID, err)
			}
			parserKey = "regex:" + inp.ID
			parserRegistry.RegisterWithKey(parserKey, regexParser)
		}
		parserTypes[domain.InputType(inp.Type)] = parserKey
	}

	// Application services
	msgSvc := service.NewMessageService(repo, queue, parserTypes, parserRegistry)
	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, exprRegistry, service.DefaultRelayWorkerConfig())

	// HTTP + WebSocket adapter assembly
	resolver := newConfigInputResolver(cfg)
	wsHandler := wsadapter.NewHandler(msgSvc)
	router := httpadapter.NewRouter(msgSvc, resolver, wsHandler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker.Start(ctx, cfg.Queue.WorkerCount)

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
		tcpL := tcpadapter.NewListener(msgSvc, domain.InputType(inp.Type), inp.Address, delimiter, contentType)
		go func() {
			slog.Info("tcp listener starting", "address", inp.Address, "inputType", inp.Type)
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
	go func() { worker.Wait(); close(waitDone) }()
	select {
	case <-waitDone:
	case <-shutCtx.Done():
		slog.Warn("worker drain timed out, forcing shutdown")
	}
	return srv.Shutdown(shutCtx)
}

// configInputResolver Config-based InputResolver implementation
type configInputResolver struct {
	inputs  map[string]domain.InputType
	secrets map[string]string
}

func newConfigInputResolver(cfg *cfgpkg.Config) *configInputResolver {
	inputs := make(map[string]domain.InputType, len(cfg.Inputs))
	secrets := make(map[string]string, len(cfg.Inputs))
	for _, s := range cfg.Inputs {
		inputs[s.ID] = domain.InputType(s.Type)
		secrets[s.ID] = s.Secret
	}
	return &configInputResolver{inputs: inputs, secrets: secrets}
}

func (r *configInputResolver) Resolve(inputID string) (domain.InputType, error) {
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
	case "json":
		return "application/json"
	case "xml":
		return "application/xml"
	case "form":
		return "application/x-www-form-urlencoded"
	case "logfmt":
		return "text/plain"
	case "regex", "":
		return "application/octet-stream"
	default:
		slog.Warn("unknown parser type for content-type mapping", "parser", parserType)
		return "application/octet-stream"
	}
}

func setupLogger(cfg *cfgpkg.Config) {
	level := map[string]slog.Level{
		"debug": slog.LevelDebug, "warn": slog.LevelWarn, "error": slog.LevelError,
	}[cfg.Log.Level]
	var h slog.Handler
	opts := &slog.HandlerOptions{Level: level}
	if cfg.Log.Format == "json" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(h))
}

func newRepository(cfg cfgpkg.StorageConfig) (output.MessageRepository, io.Closer, error) {
	switch strings.ToUpper(cfg.Type) {
	case "SQLITE":
		repo, err := sqliteadapter.New(cfg.Path)
		if err != nil {
			return nil, nil, fmt.Errorf("sqlite repository: %w", err)
		}
		return repo, repo, nil
	default:
		return nil, nil, fmt.Errorf("unsupported storage type: %q", cfg.Type)
	}
}
