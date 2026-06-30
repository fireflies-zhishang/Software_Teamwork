package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/knowledge/internal/service"
)

func TestAIGatewayRerankerSendsHeadersBodyAndMapsResults(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodPost || r.URL.Path != "/internal/v1/rerankings" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" ||
			r.Header.Get("X-Caller-Service") != "knowledge" ||
			r.Header.Get("X-Service-Token") != "svc_token" ||
			r.Header.Get("X-User-Id") != "usr_1" ||
			r.Header.Get("X-Request-Id") != "req_1" {
			t.Fatalf("headers = %+v", r.Header)
		}

		var body aiGatewayRerankRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "rerank-model" || body.ProfileID != "profile_1" || body.Query != "find transformer" || body.TopN != 1 {
			t.Fatalf("body = %+v", body)
		}
		if len(body.Documents) != 2 || body.Documents[0].ID != "chunk_1" || body.Documents[0].Text != "alpha" || body.Documents[1].ID != "chunk_2" || body.Documents[1].Text != "beta" {
			t.Fatalf("documents = %+v", body.Documents)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"document_id":"chunk_2","score":0.92},{"document_id":"chunk_1","score":0.81}]}`))
	}))
	defer server.Close()

	reranker, err := NewAIGatewayReranker(AIGatewayConfig{
		BaseURL:      server.URL,
		ServiceToken: "svc_token",
		Model:        "rerank-model",
		ProfileID:    "profile_1",
	})
	if err != nil {
		t.Fatal(err)
	}

	results, err := reranker.Rerank(context.Background(), service.RerankRequest{
		Query:     "find transformer",
		Documents: []service.RerankDocument{{ID: " chunk_1 ", Text: "alpha"}, {ID: "chunk_2", Text: "beta"}},
		TopN:      1,
		UserID:    " usr_1 ",
		RequestID: " req_1 ",
	})
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}
	if !called {
		t.Fatal("expected AI Gateway call")
	}
	if len(results) != 2 || results[0].DocumentID != "chunk_2" || results[0].Score != .92 || results[1].DocumentID != "chunk_1" {
		t.Fatalf("results = %+v", results)
	}
}

func TestAIGatewayRerankerErrorDoesNotExposeProviderBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "provider raw secret response", http.StatusBadGateway)
	}))
	defer server.Close()

	reranker, err := NewAIGatewayReranker(AIGatewayConfig{
		BaseURL:      server.URL,
		ServiceToken: "svc_token",
		Model:        "rerank-model",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = reranker.Rerank(context.Background(), service.RerankRequest{
		Query:     "query",
		Documents: []service.RerankDocument{{ID: "chunk_1", Text: "alpha"}},
		TopN:      1,
		UserID:    "usr_1",
		RequestID: "req_1",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "provider raw secret response") {
		t.Fatalf("provider body leaked in error: %v", err)
	}
}
