package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	httpadapter "webhook-relay/internal/adapter/input/http"
	"webhook-relay/internal/adapter/output/filequeue"
	sqliteadapter "webhook-relay/internal/adapter/output/sqlite"
	webhookadapter "webhook-relay/internal/adapter/output/webhook"
	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/application/service"
	cfgpkg "webhook-relay/internal/config"
	"webhook-relay/internal/domain"
)

// configSourceResolver는 cmd/server/main.go와 동일한 로직을 E2E에서 재구현 (DI 검증용)
type configSourceResolver struct {
	sources map[string]domain.SourceType
	secrets map[string]string
}

func (r *configSourceResolver) Resolve(id string) (domain.SourceType, error) {
	st, ok := r.sources[id]
	if !ok {
		return "", domain.ErrSourceNotFound
	}
	return st, nil
}

func (r *configSourceResolver) ValidateToken(id, token string) bool {
	return r.secrets[id] == token
}

func TestE2E_PostAlert_Returns201(t *testing.T) {
	// 아웃바운드 웹훅 수신 서버
	var mu sync.Mutex
	var deliveredPayload []byte
	targetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		deliveredPayload = b
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer targetSrv.Close()

	cfg := &cfgpkg.Config{
		Sources:  []cfgpkg.SourceConfig{{ID: "beszel", Type: "BESZEL", Secret: "tok"}},
		Channels: []cfgpkg.ChannelConfig{{ID: "ch1", Type: "WEBHOOK", URL: targetSrv.URL, Template: `{"src":"{{ .Source }}"}`, RetryCount: 1, RetryDelayMs: 10}},
		Routes:   []cfgpkg.RouteConfig{{SourceID: "beszel", ChannelIDs: []string{"ch1"}}},
		Queue:    cfgpkg.QueueConfig{WorkerCount: 1},
	}

	repo, _ := sqliteadapter.New(":memory:")
	defer repo.Close()
	queue, _ := filequeue.New(t.TempDir())
	sender := webhookadapter.NewSender()
	registry := webhookadapter.NewRegistry(map[domain.ChannelType]output.AlertSender{
		domain.ChannelTypeWebhook: sender,
	})
	routeReader := cfgpkg.NewInMemoryRouteConfigReader(cfg)
	alertSvc := service.NewAlertService(repo, queue)
	worker := service.NewDeliveryWorker(queue, repo, routeReader, registry)

	resolver := &configSourceResolver{
		sources: map[string]domain.SourceType{"beszel": domain.SourceTypeBeszel},
		secrets: map[string]string{"beszel": "tok"},
	}
	router := httpadapter.NewRouter(alertSvc, resolver, nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	worker.Start(ctx, 1)

	// POST 알람
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/sources/beszel/alerts",
		strings.NewReader(`{"host":"server1"}`))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	if v := resp.Header.Get("X-API-Version"); v == "" {
		t.Error("X-API-Version header missing")
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "PENDING" {
		t.Errorf("body status = %v, want PENDING", body["status"])
	}

	// DeliveryWorker가 전달 완료할 때까지 대기
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(deliveredPayload)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	got := make([]byte, len(deliveredPayload))
	copy(got, deliveredPayload)
	mu.Unlock()

	if len(got) == 0 {
		t.Error("delivery worker did not deliver the alert")
	}
	want := fmt.Sprintf(`{"src":"%s"}`, string(domain.SourceTypeBeszel))
	if string(got) != want {
		t.Errorf("delivered payload = %q, want %q", got, want)
	}
}

func TestE2E_Healthz(t *testing.T) {
	repo, _ := sqliteadapter.New(":memory:")
	defer repo.Close()
	queue, _ := filequeue.New(t.TempDir())
	alertSvc := service.NewAlertService(repo, queue)
	resolver := &configSourceResolver{}
	router := httpadapter.NewRouter(alertSvc, resolver, nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/healthz")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz status = %d, want 200", resp.StatusCode)
	}
}
