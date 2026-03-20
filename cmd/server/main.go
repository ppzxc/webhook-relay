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
	httpadapter "webhook-relay/internal/adapter/input/http"
	wsadapter "webhook-relay/internal/adapter/input/websocket"
	"webhook-relay/internal/adapter/output/filequeue"
	sqliteadapter "webhook-relay/internal/adapter/output/sqlite"
	webhookadapter "webhook-relay/internal/adapter/output/webhook"
	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/application/service"
	cfgpkg "webhook-relay/internal/config"
	"webhook-relay/internal/domain"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var cfgPath string
	root := &cobra.Command{Use: "webhook-relay", Short: "Monitoring alert relay hub"}
	start := &cobra.Command{
		Use:   "start",
		Short: "Start server",
		RunE:  func(_ *cobra.Command, _ []string) error { return runServer(cfgPath) },
	}
	start.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "config file path")
	root.AddCommand(start)
	root.AddCommand(&cobra.Command{
		Use: "version", Short: "Print version",
		Run: func(_ *cobra.Command, _ []string) { fmt.Println("webhook-relay v0.1.0") },
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
	registry := webhookadapter.NewRegistry(map[domain.ChannelType]output.AlertSender{
		domain.ChannelTypeWebhook: sender,
	})

	// 설정 기반 라우팅 (핫리로드 지원)
	routeReader := cfgpkg.NewInMemoryRouteConfigReader(cfg)

	// Viper WatchConfig → 핫리로드
	v, err := cfgpkg.NewViper(cfgPath)
	if err != nil {
		return fmt.Errorf("init viper: %w", err)
	}
	cfgpkg.Watch(v, func(newCfg *cfgpkg.Config) {
		routeReader.Update(newCfg)
		slog.Info("config reloaded")
	})

	// 애플리케이션 서비스
	alertSvc := service.NewAlertService(repo, queue)
	worker := service.NewDeliveryWorker(queue, repo, routeReader, registry)

	// HTTP + WebSocket 어댑터 조립
	resolver := newConfigSourceResolver(cfg)
	wsHandler := wsadapter.NewHandler(alertSvc)
	router := httpadapter.NewRouter(alertSvc, resolver, wsHandler)

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
	return srv.Shutdown(shutCtx)
}

// configSourceResolver Config 기반 SourceResolver 구현
type configSourceResolver struct {
	sources map[string]domain.SourceType
	secrets map[string]string
}

func newConfigSourceResolver(cfg *cfgpkg.Config) *configSourceResolver {
	sources := make(map[string]domain.SourceType, len(cfg.Sources))
	secrets := make(map[string]string, len(cfg.Sources))
	for _, s := range cfg.Sources {
		sources[s.ID] = domain.SourceType(s.Type)
		secrets[s.ID] = s.Secret
	}
	return &configSourceResolver{sources: sources, secrets: secrets}
}

func (r *configSourceResolver) Resolve(sourceID string) (domain.SourceType, error) {
	st, ok := r.sources[sourceID]
	if !ok {
		return "", fmt.Errorf("resolve %q: %w", sourceID, domain.ErrSourceNotFound)
	}
	return st, nil
}

func (r *configSourceResolver) ValidateToken(sourceID, token string) bool {
	return r.secrets[sourceID] == token
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
