package knowledgeclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/document/internal/service"
)

func TestClientRetrievesReportContextWithInternalHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/v1/knowledge-queries" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Service-Token") != "service-token" {
			t.Fatalf("X-Service-Token = %q", r.Header.Get("X-Service-Token"))
		}
		if r.Header.Get("X-Caller-Service") != "document" {
			t.Fatalf("X-Caller-Service = %q", r.Header.Get("X-Caller-Service"))
		}
		if r.Header.Get("X-Request-Id") != "req-knowledge" {
			t.Fatalf("X-Request-Id = %q", r.Header.Get("X-Request-Id"))
		}
		if r.Header.Get("X-User-Id") != "user-1" {
			t.Fatalf("X-User-Id = %q", r.Header.Get("X-User-Id"))
		}
		var body struct {
			Query            string   `json:"query"`
			KnowledgeBaseIDs []string `json:"knowledgeBaseIds"`
			TopK             int      `json:"topK"`
			Rerank           bool     `json:"rerank"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Query != "summer peak inspection" || len(body.KnowledgeBaseIDs) != 1 || body.KnowledgeBaseIDs[0] != "kb-1" || body.TopK != 2 || !body.Rerank {
			t.Fatalf("request body = %+v", body)
		}
		_, _ = w.Write([]byte(`{"data":{"results":[{"score":0.9,"knowledgeBaseId":"kb-1","documentId":"doc-1","chunkId":"chunk-1","documentName":"guide","sectionPath":"1","contentPreview":"safe context"}]},"requestId":"req-knowledge"}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "service-token", server.Client())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	results, err := client.RetrieveReportContext(context.Background(), service.RequestContext{RequestID: "req-knowledge", UserID: "user-1"}, service.ReportKnowledgeRetrievalInput{
		Query:            "summer peak inspection",
		KnowledgeBaseIDs: []string{"kb-1"},
		TopK:             2,
		Rerank:           true,
	})
	if err != nil {
		t.Fatalf("RetrieveReportContext() error = %v", err)
	}
	if len(results) != 1 || results[0].ContentPreview != "safe context" || results[0].DocumentName != "guide" {
		t.Fatalf("results = %+v", results)
	}
}

func TestClientSanitizesKnowledgeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"postgres at http://knowledge.internal with sk-secret"}}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "service-token", server.Client())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = client.RetrieveReportContext(context.Background(), service.RequestContext{}, service.ReportKnowledgeRetrievalInput{
		Query:            "prompt must stay local",
		KnowledgeBaseIDs: []string{"kb-1"},
	})
	if err == nil {
		t.Fatal("RetrieveReportContext() error = nil, want dependency error")
	}
	if strings.Contains(err.Error(), "knowledge.internal") || strings.Contains(err.Error(), "sk-secret") || strings.Contains(err.Error(), "prompt must stay local") {
		t.Fatalf("error leaked sensitive data: %v", err)
	}
}
