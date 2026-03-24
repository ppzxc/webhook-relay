package e2e_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	httpadapter "relaybox/internal/adapter/input/http"
	"relaybox/internal/adapter/output/expression"
	"relaybox/internal/adapter/output/filequeue"
	sqliteadapter "relaybox/internal/adapter/output/sqlite"
	webhookadapter "relaybox/internal/adapter/output/webhook"
	"relaybox/internal/application/port/output"
	"relaybox/internal/application/service"
	cfgpkg "relaybox/internal/config"
	"relaybox/internal/domain"
)

// configInputResolver replicates cmd/server/main.go logic for E2E DI
type configInputResolver struct {
	inputs  map[string]string
	secrets map[string]string
}

func (r *configInputResolver) Resolve(id string) (string, error) {
	st, ok := r.inputs[id]
	if !ok {
		return "", domain.ErrInputNotFound
	}
	return st, nil
}

func (r *configInputResolver) ValidateToken(id, token string) bool {
	return r.secrets[id] == token
}

func newExprRegistry() output.ExpressionEngineRegistry {
	reg := expression.NewInMemoryExpressionEngineRegistry()
	celEng, err := expression.NewCELEngine()
	if err != nil {
		panic("NewCELEngine: " + err.Error())
	}
	reg.Register(celEng)
	reg.Register(expression.NewExprEngine())
	return reg
}

func TestE2E_PostMessage_Returns201(t *testing.T) {
	// Outbound webhook receiver
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
		Inputs: []cfgpkg.InputConfig{{
			ID: "beszel", Engine: "CEL", Secret: "tok",
			Rules: []cfgpkg.RuleConfig{{OutputIDs: []string{"ch1"}}},
		}},
		Outputs: []cfgpkg.OutputConfig{{
			ID: "ch1", Type: "WEBHOOK", Engine: "CEL", URL: targetSrv.URL,
			Template: map[string]string{
				"src": `data.input`,
			},
			RetryCount: 1, RetryDelayMs: 10,
		}},
		Queue: cfgpkg.QueueConfig{WorkerCount: 1},
	}

	repo, _ := sqliteadapter.New(":memory:")
	defer repo.Close()
	queue, _ := filequeue.New(t.TempDir())
	sender := webhookadapter.NewSender()
	registry := webhookadapter.NewRegistry(map[domain.OutputType]output.OutputSender{
		domain.OutputTypeWebhook: sender,
	})
	ruleReader := cfgpkg.NewInMemoryRuleConfigReader(cfg)
	msgSvc := service.NewMessageService(repo, queue, nil, nil)
	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())

	resolver := &configInputResolver{
		inputs:  map[string]string{"beszel": "beszel"},
		secrets: map[string]string{"beszel": "tok"},
	}
	router := httpadapter.NewRouter(msgSvc, msgSvc, resolver, nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	worker.Start(ctx, 1)

	// POST message
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/inputs/beszel/messages",
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

	// Wait for relay worker to deliver
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
		t.Error("relay worker did not deliver the message")
	}
	// Template is {"src": data.input} which evaluates to {"src":"beszel"} (input ID)
	var result map[string]any
	if err := json.Unmarshal(got, &result); err != nil {
		t.Fatalf("unmarshal delivered payload: %v", err)
	}
	if result["src"] != "beszel" {
		t.Errorf("delivered src = %v, want beszel", result["src"])
	}
}

func TestE2E_Healthz(t *testing.T) {
	repo, _ := sqliteadapter.New(":memory:")
	defer repo.Close()
	queue, _ := filequeue.New(t.TempDir())
	msgSvc := service.NewMessageService(repo, queue, nil, nil)
	resolver := &configInputResolver{}
	router := httpadapter.NewRouter(msgSvc, msgSvc, resolver, nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/healthz")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz status = %d, want 200", resp.StatusCode)
	}
}
