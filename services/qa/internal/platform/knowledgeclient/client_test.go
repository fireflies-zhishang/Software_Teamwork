package knowledgeclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/qa/internal/service"
)

func TestRetrievePropagatesTrustedContextAndMapsResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/knowledge-queries" {
			t.Errorf("path=%q", r.URL.Path)
		}
		for name, want := range map[string]string{"X-Service-Token": "service-token", "X-Caller-Service": "qa", "X-User-Id": "user-1", "X-Request-Id": "req-knowledge-test"} {
			if got := r.Header.Get(name); got != want {
				t.Errorf("%s=%q want %q", name, got, want)
			}
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["query"] != "query" || int(payload["topK"].(float64)) != 5 || payload["rerank"] != true || int(payload["rerankTopN"].(float64)) != 3 {
			t.Fatalf("payload=%+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"results":[{"score":0.77,"knowledgeBaseId":"kb-1","documentId":"doc-1","chunkId":"chunk-1","documentName":"guide","sectionPath":"1 / 2","contentPreview":"preview","chunkIndex":2,"chunkType":"paragraph","tags":["safe"]}]},"requestId":"req-knowledge-test"}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "service-token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := service.WithRequestID(context.Background(), "req-knowledge-test")
	results, err := client.Retrieve(ctx, "user-1", service.RetrievalTestInput{Question: "query", KnowledgeBaseIDs: []string{"kb-1"}, Retrieval: service.RetrievalSettings{TopK: 5, EnableRerank: true, RerankTopN: 3}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].DocumentID != "doc-1" || results[0].VectorScore != nil || results[0].RerankScore == nil || *results[0].RerankScore != 0.77 {
		t.Fatalf("results=%+v", results)
	}
	if results[0].Metadata["chunkIndex"] != float64(2) && results[0].Metadata["chunkIndex"] != 2 || results[0].Metadata["chunkType"] != "paragraph" {
		t.Fatalf("metadata=%+v", results[0].Metadata)
	}
	if _, ok := results[0].Metadata["vector"]; ok {
		t.Fatalf("metadata leaked vector payload: %+v", results[0].Metadata)
	}
}

func TestRetrieveTreatsKnowledgeScoreAsVectorScoreWithoutRerank(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"results":[{"score":0.88,"knowledgeBaseId":"kb-1","documentId":"doc-1","chunkId":"chunk-1","documentName":"guide","contentPreview":"preview"}]},"requestId":"req-knowledge-test"}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "service-token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	results, err := client.Retrieve(context.Background(), "user-1", service.RetrievalTestInput{Question: "query", Retrieval: service.RetrievalSettings{TopK: 5}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Score != 0.88 || results[0].VectorScore == nil || *results[0].VectorScore != 0.88 || results[0].RerankScore != nil {
		t.Fatalf("results=%+v", results)
	}
}

func TestRetrieveMapsForbiddenKnowledgeResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"resource not found","requestId":"req-test"}}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "service-token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Retrieve(context.Background(), "user-1", service.RetrievalTestInput{Question: "query", KnowledgeBaseIDs: []string{"kb-private"}})
	appErr, ok := service.Classify(err)
	if !ok || appErr.Code != service.CodeForbidden {
		t.Fatalf("error=%v, want forbidden", err)
	}
}

func TestRetrieveMapsKnowledgeServiceFailuresToDependency(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "unauthorized service token", status: http.StatusUnauthorized, body: `{"error":{"code":"unauthorized","message":"authentication is required","requestId":"req-test"}}`},
		{name: "route not found", status: http.StatusNotFound, body: `{"error":{"code":"not_found","message":"route not found","requestId":"req-test"}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			client, err := New(server.URL, "service-token", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Retrieve(context.Background(), "user-1", service.RetrievalTestInput{Question: "query", KnowledgeBaseIDs: []string{"kb-1"}})
			appErr, ok := service.Classify(err)
			if !ok || appErr.Code != service.CodeDependency {
				t.Fatalf("error=%v, want dependency_error", err)
			}
		})
	}
}
