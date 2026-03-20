package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	httpadapter "relaybox/internal/adapter/input/http"
	wsadapter "relaybox/internal/adapter/input/websocket"
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

	// 아웃바운드 어댑터
	repo, err := sqliteadapter.New(cfg.Storage.Path)
	if err != nil {
		return fmt.Errorf("init sqlite: %w", err)
	}
	defer repo.Close()

	queue, err := filequeue.New(cfg.Queue.Path)
	if err != nil {
		return fmt.Errorf("init queue: %w", err)
	}

	sender := webhookadapter.NewSender()
	registry := webhookadapter.NewRegistry(map[domain.OutputType]output.OutputSender{
		domain.OutputTypeWebhook: sender,
	})

	// 설정 기반 라우팅 (핫리로드 지원)
	ruleReader := cfgpkg.NewInMemoryRuleConfigReader(cfg)

	// Viper WatchConfig → 핫리로드
	v, err := cfgpkg.NewViper(cfgPath)
	if err != nil {
		return fmt.Errorf("init viper: %w", err)
	}
	cfgpkg.Watch(v, func(newCfg *cfgpkg.Config) {
		ruleReader.Update(newCfg)
		slog.Info("config reloaded")
	})

	// 애플리케이션 서비스
	msgSvc := service.NewMessageService(repo, queue)
	worker := service.NewRelayWorker(queue, repo, ruleReader, registry)

	// HTTP + WebSocket 어댑터 조립
	resolver := newConfigInputResolver(cfg)
	wsHandler := wsadapter.NewHandler(msgSvc)
	router := httpadapter.NewRouter(msgSvc, resolver, wsHandler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker.Start(ctx, cfg.Queue.WorkerCount)

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

// configInputResolver Config 기반 InputResolver 구현
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
