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
	outputconfig "relaybox/internal/adapter/output/config"
	"relaybox/internal/adapter/output/expression"
	"relaybox/internal/adapter/output/filequeue"
	sqliteadapter "relaybox/internal/adapter/output/sqlite"
	webhookadapter "relaybox/internal/adapter/output/webhook"
	inputport "relaybox/internal/application/port/input"
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

	repo, _ := sqliteadapter.New(":memory:", "")
	defer repo.Close()
	queue, _ := filequeue.New(t.TempDir())
	sender := webhookadapter.NewSender()
	registry := webhookadapter.NewRegistry(map[domain.OutputType]output.OutputSender{
		domain.OutputTypeWebhook: sender,
	})
	ruleReader := outputconfig.NewInMemoryRuleConfigReader(cfg)
	msgSvc := service.NewMessageService(repo, queue, nil, nil)
	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())

	resolver := &configInputResolver{
		inputs:  map[string]string{"beszel": "beszel"},
		secrets: map[string]string{"beszel": "tok"},
	}
	router := httpadapter.NewRouter(msgSvc, msgSvc, nil, nil, nil, resolver, nil)
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

func TestE2E_ListMessages(t *testing.T) {
	cfg := &cfgpkg.Config{
		Inputs:  []cfgpkg.InputConfig{{ID: "beszel", Engine: "CEL", Secret: "tok"}},
		Outputs: []cfgpkg.OutputConfig{{ID: "wh1", Type: "WEBHOOK", Engine: "CEL", URL: "http://localhost:9999"}},
		Queue:   cfgpkg.QueueConfig{WorkerCount: 1},
	}
	repo, _ := sqliteadapter.New(":memory:", "")
	defer repo.Close()
	queue, _ := filequeue.New(t.TempDir())
	msgSvc := service.NewMessageService(repo, queue, nil, nil)
	configQuerySvc := service.NewConfigQueryService(cfg)
	resolver := &configInputResolver{
		inputs:  map[string]string{"beszel": "beszel"},
		secrets: map[string]string{"beszel": "tok"},
	}
	router := httpadapter.NewRouter(msgSvc, msgSvc, msgSvc, msgSvc, configQuerySvc, resolver, nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	// POST two messages
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/inputs/beszel/messages",
			strings.NewReader(`{"n":1}`))
		req.Header.Set("Authorization", "Bearer tok")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST error: %v", err)
		}
		resp.Body.Close()
	}

	// List messages
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/inputs/beszel/messages?limit=10&offset=0", nil)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET list error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", resp.StatusCode)
	}
	var msgs []domain.Message
	json.NewDecoder(resp.Body).Decode(&msgs)
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages in list, got %d", len(msgs))
	}
}

func TestE2E_ConfigEndpoints(t *testing.T) {
	cfg := &cfgpkg.Config{
		Inputs: []cfgpkg.InputConfig{
			{ID: "beszel", Engine: "CEL", Secret: "tok"},
			{ID: "dozzle", Engine: "EXPR", Secret: "tok2"},
		},
		Outputs: []cfgpkg.OutputConfig{
			{ID: "wh1", Type: "WEBHOOK", Engine: "CEL", URL: "http://example.com/hook", RetryCount: 3, RetryDelayMs: 100},
		},
	}
	repo, _ := sqliteadapter.New(":memory:", "")
	defer repo.Close()
	queue, _ := filequeue.New(t.TempDir())
	msgSvc := service.NewMessageService(repo, queue, nil, nil)
	configQuerySvc := service.NewConfigQueryService(cfg)
	resolver := &configInputResolver{
		inputs:  map[string]string{"beszel": "beszel", "dozzle": "dozzle"},
		secrets: map[string]string{"beszel": "tok", "dozzle": "tok2"},
	}
	router := httpadapter.NewRouter(msgSvc, msgSvc, msgSvc, msgSvc, configQuerySvc, resolver, nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	t.Run("GET /inputs", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/inputs")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var inputs []inputport.InputSummary
		json.NewDecoder(resp.Body).Decode(&inputs)
		if len(inputs) != 2 {
			t.Errorf("expected 2 inputs, got %d", len(inputs))
		}
	})

	t.Run("GET /inputs/beszel", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/inputs/beszel")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var inp inputport.InputSummary
		json.NewDecoder(resp.Body).Decode(&inp)
		if inp.ID != "beszel" {
			t.Errorf("id = %q, want beszel", inp.ID)
		}
	})

	t.Run("GET /outputs", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/outputs")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var outputs []inputport.OutputSummary
		json.NewDecoder(resp.Body).Decode(&outputs)
		if len(outputs) != 1 {
			t.Errorf("expected 1 output, got %d", len(outputs))
		}
		if outputs[0].ID != "wh1" {
			t.Errorf("output id = %q, want wh1", outputs[0].ID)
		}
	})

	t.Run("GET /outputs/wh1", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/outputs/wh1")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var out inputport.OutputSummary
		json.NewDecoder(resp.Body).Decode(&out)
		if out.RetryCount != 3 {
			t.Errorf("retryCount = %d, want 3", out.RetryCount)
		}
	})
}

func TestE2E_Healthz(t *testing.T) {
	repo, _ := sqliteadapter.New(":memory:", "")
	defer repo.Close()
	queue, _ := filequeue.New(t.TempDir())
	msgSvc := service.NewMessageService(repo, queue, nil, nil)
	resolver := &configInputResolver{}
	router := httpadapter.NewRouter(msgSvc, msgSvc, nil, nil, nil, resolver, nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/healthz")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz status = %d, want 200", resp.StatusCode)
	}
}
