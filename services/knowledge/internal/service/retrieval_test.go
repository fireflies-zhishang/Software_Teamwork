package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	rerankplatform "github.com/Sakayori-Iroha-168/Software_Teamwork/services/knowledge/internal/platform/rerank"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/knowledge/internal/repository"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/knowledge/internal/service"
)

type retrievalEmbedder struct{}

func (retrievalEmbedder) Embed(context.Context, service.EmbeddingRequest) (service.EmbeddingResult, error) {
	return service.EmbeddingResult{Vectors: [][]float32{{1, 0}}, Provider: "fake", Model: "fake", Dimension: 2}, nil
}

type retrievalIndex struct {
	hits    []service.VectorSearchHit
	request service.VectorSearchRequest
}

func (*retrievalIndex) Upsert(context.Context, []service.VectorPoint) error { return nil }
func (*retrievalIndex) DeleteByDocument(context.Context, string) error      { return nil }
func (i *retrievalIndex) Search(_ context.Context, request service.VectorSearchRequest) ([]service.VectorSearchHit, error) {
	i.request = request
	return append([]service.VectorSearchHit(nil), i.hits...), nil
}

type retrievalReranker struct {
	request service.RerankRequest
	results []service.RerankResult
	err     error
}

func (r *retrievalReranker) Rerank(_ context.Context, request service.RerankRequest) ([]service.RerankResult, error) {
	r.request = request
	return append([]service.RerankResult(nil), r.results...), r.err
}

func TestKnowledgeQueryFiltersAndHydratesSafeResults(t *testing.T) {
	repo := repository.NewMemoryRepository()
	seedRetrievalBase(t, repo, "kb_owned", "usr_owner")
	seedRetrievalBase(t, repo, "kb_other", "usr_other")
	seedRetrievalDocument(t, repo, "doc_ready", "kb_owned", "usr_owner", service.DocumentStatusReady, nil, []string{"ops", "manual"}, map[string]any{"region": "east"})
	seedRetrievalDocument(t, repo, "doc_pending", "kb_owned", "usr_owner", service.DocumentStatusEmbedding, nil, []string{"ops"}, map[string]any{"region": "east"})
	seedRetrievalDocument(t, repo, "doc_other", "kb_other", "usr_other", service.DocumentStatusReady, nil, []string{"ops"}, map[string]any{"region": "east"})
	deleted := time.Now().UTC()
	seedRetrievalDocument(t, repo, "doc_deleted", "kb_owned", "usr_owner", service.DocumentStatusReady, &deleted, []string{"ops"}, map[string]any{"region": "east"})
	seedRetrievalDocument(t, repo, "doc_wrong_filter", "kb_owned", "usr_owner", service.DocumentStatusReady, nil, []string{"other"}, map[string]any{"region": "west"})

	index := &retrievalIndex{hits: []service.VectorSearchHit{
		{ID: "point_ready", Score: .91, Payload: map[string]any{"chunk_id": "chunk_doc_ready"}},
		{ID: "point_low", Score: .20, Payload: map[string]any{"chunk_id": "chunk_doc_ready"}},
		{ID: "point_pending", Score: .90, Payload: map[string]any{"chunk_id": "chunk_doc_pending"}},
		{ID: "point_other", Score: .89, Payload: map[string]any{"chunk_id": "chunk_doc_other"}},
		{ID: "point_deleted", Score: .88, Payload: map[string]any{"chunk_id": "chunk_doc_deleted"}},
		{ID: "point_filter", Score: .87, Payload: map[string]any{"chunk_id": "chunk_doc_wrong_filter"}},
	}}
	svc := service.NewKnowledgeService(repo, service.WithVectorIndex(retrievalEmbedder{}, index, "test_chunks"))
	threshold := .5
	result, err := svc.CreateKnowledgeQuery(context.Background(), service.RequestContext{UserID: "usr_owner"}, service.KnowledgeQueryInput{
		Query: " transformer maintenance ", KnowledgeBaseIDs: []string{"kb_owned"}, TopK: 3,
		ScoreThreshold: &threshold, Tags: []string{"ops"}, MetadataFilter: map[string]string{"region": "east"},
	})
	if err != nil {
		t.Fatalf("CreateKnowledgeQuery() error = %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].ChunkID != "chunk_doc_ready" {
		t.Fatalf("results = %+v", result.Results)
	}
	if result.Results[0].ContentPreview == "" || result.Results[0].DocumentName != "doc_ready.md" {
		t.Fatalf("hydrated result = %+v", result.Results[0])
	}
	if index.request.Limit != 3 || index.request.ScoreThreshold != threshold || strings.Join(index.request.Tags, ",") != "ops" || index.request.MetadataFilter["region"] != "east" {
		t.Fatalf("vector request = %+v", index.request)
	}
	if result.Trace.HitCount != 1 || result.Trace.SearchTopK != 3 {
		t.Fatalf("trace = %+v", result.Trace)
	}
}

func TestKnowledgeQueryReranksWithFullContentAndTopN(t *testing.T) {
	repo := repository.NewMemoryRepository()
	seedRetrievalBase(t, repo, "kb_owned", "usr_owner")
	longContent := strings.Repeat("a", 700)
	seedRetrievalDocumentWithContent(t, repo, "doc_one", "kb_owned", "usr_owner", longContent)
	seedRetrievalDocumentWithContent(t, repo, "doc_two", "kb_owned", "usr_owner", "second")
	index := &retrievalIndex{hits: []service.VectorSearchHit{
		{ID: "point_one", Score: .9, Payload: map[string]any{"chunk_id": "chunk_doc_one"}},
		{ID: "point_two", Score: .8, Payload: map[string]any{"chunk_id": "chunk_doc_two"}},
	}}
	reranker := &retrievalReranker{results: []service.RerankResult{{DocumentID: "chunk_doc_two", Score: .99}}}
	svc := service.NewKnowledgeService(repo, service.WithVectorIndex(retrievalEmbedder{}, index), service.WithReranker(reranker))
	topN := 1
	result, err := svc.CreateKnowledgeQuery(context.Background(), service.RequestContext{UserID: "usr_owner", RequestID: "req_full_content"}, service.KnowledgeQueryInput{Query: "query", KnowledgeBaseIDs: []string{"kb_owned"}, TopK: 2, Rerank: true, RerankTopN: &topN})
	if err != nil {
		t.Fatalf("CreateKnowledgeQuery() error = %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].ChunkID != "chunk_doc_two" || result.Results[0].Score != .99 {
		t.Fatalf("reranked results = %+v", result.Results)
	}
	if reranker.request.TopN != 1 || reranker.request.UserID != "usr_owner" || reranker.request.RequestID != "req_full_content" || len(reranker.request.Documents) != 2 || len(reranker.request.Documents[0].Text) != len(longContent) {
		t.Fatalf("rerank request = %+v", reranker.request)
	}
	if result.Trace.QdrantCollection != "knowledge_chunks" {
		t.Fatalf("qdrant collection = %q", result.Trace.QdrantCollection)
	}
}

func TestKnowledgeQueryRerankPartialResultsPreserveVectorCandidates(t *testing.T) {
	repo := repository.NewMemoryRepository()
	seedRetrievalBase(t, repo, "kb_owned", "usr_owner")
	seedRetrievalDocumentWithContent(t, repo, "doc_one", "kb_owned", "usr_owner", "one")
	seedRetrievalDocumentWithContent(t, repo, "doc_two", "kb_owned", "usr_owner", "two")
	index := &retrievalIndex{hits: []service.VectorSearchHit{
		{ID: "point_one", Score: .9, Payload: map[string]any{"chunk_id": "chunk_doc_one"}},
		{ID: "point_two", Score: .8, Payload: map[string]any{"chunk_id": "chunk_doc_two"}},
	}}
	reranker := &retrievalReranker{results: []service.RerankResult{{DocumentID: "chunk_doc_two", Score: .99}}}
	svc := service.NewKnowledgeService(repo, service.WithVectorIndex(retrievalEmbedder{}, index), service.WithReranker(reranker))
	topN := 2
	result, err := svc.CreateKnowledgeQuery(context.Background(), service.RequestContext{UserID: "usr_owner"}, service.KnowledgeQueryInput{Query: "query", KnowledgeBaseIDs: []string{"kb_owned"}, TopK: 2, Rerank: true, RerankTopN: &topN})
	if err != nil {
		t.Fatalf("CreateKnowledgeQuery() error = %v", err)
	}
	if len(result.Results) != 2 || result.Results[0].ChunkID != "chunk_doc_two" || result.Results[1].ChunkID != "chunk_doc_one" {
		t.Fatalf("reranked results = %+v", result.Results)
	}
	if result.Results[1].Score != .9 {
		t.Fatalf("vector fallback score = %v", result.Results[1].Score)
	}
}

func TestKnowledgeQueryCallsAIGatewayReranker(t *testing.T) {
	repo := repository.NewMemoryRepository()
	seedRetrievalBase(t, repo, "kb_owned", "usr_owner")
	seedRetrievalDocumentWithContent(t, repo, "doc_one", "kb_owned", "usr_owner", "one")
	seedRetrievalDocumentWithContent(t, repo, "doc_two", "kb_owned", "usr_owner", "two")
	index := &retrievalIndex{hits: []service.VectorSearchHit{
		{ID: "point_one", Score: .9, Payload: map[string]any{"chunk_id": "chunk_doc_one"}},
		{ID: "point_two", Score: .8, Payload: map[string]any{"chunk_id": "chunk_doc_two"}},
	}}

	called := false
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodPost || r.URL.Path != "/internal/v1/rerankings" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Caller-Service") != "knowledge" || r.Header.Get("X-User-Id") != "usr_owner" || r.Header.Get("X-Request-Id") != "req_rerank" || r.Header.Get("X-Service-Token") != "svc_token" {
			t.Fatalf("headers = %+v", r.Header)
		}
		var body struct {
			ProfileID string `json:"profile_id"`
			Model     string `json:"model"`
			Query     string `json:"query"`
			Documents []struct {
				ID   string `json:"id"`
				Text string `json:"text"`
			} `json:"documents"`
			TopN int `json:"top_n"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.ProfileID != "profile_rerank" || body.Model != "rerank-model" || body.Query != "query" || body.TopN != 2 {
			t.Fatalf("body = %+v", body)
		}
		if len(body.Documents) != 2 || body.Documents[0].ID != "chunk_doc_one" || body.Documents[0].Text != "one" || body.Documents[1].ID != "chunk_doc_two" || body.Documents[1].Text != "two" {
			t.Fatalf("documents = %+v", body.Documents)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","model":"rerank-model","data":[{"index":1,"document_id":"chunk_doc_two","score":0.99},{"index":0,"document_id":"chunk_doc_one","score":0.77}]}`))
	}))
	defer gateway.Close()

	reranker, err := rerankplatform.NewAIGatewayReranker(rerankplatform.AIGatewayConfig{
		BaseURL:      gateway.URL,
		ServiceToken: "svc_token",
		Model:        "rerank-model",
		ProfileID:    "profile_rerank",
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := service.NewKnowledgeService(repo, service.WithVectorIndex(retrievalEmbedder{}, index), service.WithReranker(reranker))
	result, err := svc.CreateKnowledgeQuery(context.Background(), service.RequestContext{UserID: "usr_owner", RequestID: "req_rerank"}, service.KnowledgeQueryInput{Query: "query", KnowledgeBaseIDs: []string{"kb_owned"}, TopK: 2, Rerank: true})
	if err != nil {
		t.Fatalf("CreateKnowledgeQuery() error = %v", err)
	}
	if !called {
		t.Fatal("expected AI Gateway reranker call")
	}
	if len(result.Results) != 2 || result.Results[0].ChunkID != "chunk_doc_two" || result.Results[0].Score != .99 || result.Results[1].ChunkID != "chunk_doc_one" {
		t.Fatalf("reranked results = %+v", result.Results)
	}
}

func TestKnowledgeQueryAIGatewayRerankErrorIsSanitized(t *testing.T) {
	repo := repository.NewMemoryRepository()
	seedRetrievalBase(t, repo, "kb_owned", "usr_owner")
	seedRetrievalDocumentWithContent(t, repo, "doc_one", "kb_owned", "usr_owner", "one")
	index := &retrievalIndex{hits: []service.VectorSearchHit{{ID: "point_one", Score: .8, Payload: map[string]any{"chunk_id": "chunk_doc_one"}}}}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "provider raw secret response", http.StatusBadGateway)
	}))
	defer gateway.Close()
	reranker, err := rerankplatform.NewAIGatewayReranker(rerankplatform.AIGatewayConfig{
		BaseURL:      gateway.URL,
		ServiceToken: "svc_token",
		Model:        "rerank-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := service.NewKnowledgeService(repo, service.WithVectorIndex(retrievalEmbedder{}, index), service.WithReranker(reranker))
	_, err = svc.CreateKnowledgeQuery(context.Background(), service.RequestContext{UserID: "usr_owner", RequestID: "req_rerank"}, service.KnowledgeQueryInput{Query: "query", KnowledgeBaseIDs: []string{"kb_owned"}, TopK: 1, Rerank: true})
	if err == nil || strings.Contains(err.Error(), "provider raw secret response") {
		t.Fatalf("rerank error = %v", err)
	}
}

func TestKnowledgeQueryMetadataFilterMatchesNumericValue(t *testing.T) {
	repo := repository.NewMemoryRepository()
	seedRetrievalBase(t, repo, "kb_owned", "usr_owner")
	seedRetrievalDocument(t, repo, "doc_numeric", "kb_owned", "usr_owner", service.DocumentStatusReady, nil, nil, map[string]any{"revision": 2})
	index := &retrievalIndex{hits: []service.VectorSearchHit{{Score: .9, Payload: map[string]any{"chunk_id": "chunk_doc_numeric"}}}}
	svc := service.NewKnowledgeService(repo, service.WithVectorIndex(retrievalEmbedder{}, index))
	result, err := svc.CreateKnowledgeQuery(context.Background(), service.RequestContext{UserID: "usr_owner"}, service.KnowledgeQueryInput{Query: "query", KnowledgeBaseIDs: []string{"kb_owned"}, TopK: 1, MetadataFilter: map[string]string{"revision": "2"}})
	if err != nil || len(result.Results) != 1 {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
}

func TestKnowledgeQueryCapsResultsAtTopK(t *testing.T) {
	repo := repository.NewMemoryRepository()
	seedRetrievalBase(t, repo, "kb_owned", "usr_owner")
	index := &retrievalIndex{}
	for _, id := range []string{"doc_one", "doc_two", "doc_three"} {
		seedRetrievalDocumentWithContent(t, repo, id, "kb_owned", "usr_owner", id)
		index.hits = append(index.hits, service.VectorSearchHit{ID: "point_" + id, Score: .9, Payload: map[string]any{"chunk_id": "chunk_" + id}})
	}
	svc := service.NewKnowledgeService(repo, service.WithVectorIndex(retrievalEmbedder{}, index))
	result, err := svc.CreateKnowledgeQuery(context.Background(), service.RequestContext{UserID: "usr_owner"}, service.KnowledgeQueryInput{Query: "query", KnowledgeBaseIDs: []string{"kb_owned"}, TopK: 2})
	if err != nil || len(result.Results) != 2 || index.request.Limit != 2 {
		t.Fatalf("result = %+v, request = %+v, error = %v", result, index.request, err)
	}
}

func TestKnowledgeQueryRerankFallbackAndEmptyResults(t *testing.T) {
	repo := repository.NewMemoryRepository()
	seedRetrievalBase(t, repo, "kb_owned", "usr_owner")
	seedRetrievalDocumentWithContent(t, repo, "doc_one", "kb_owned", "usr_owner", "one")
	index := &retrievalIndex{hits: []service.VectorSearchHit{{ID: "point_one", Score: .8, Payload: map[string]any{"chunk_id": "chunk_doc_one"}}}}
	svc := service.NewKnowledgeService(repo, service.WithVectorIndex(retrievalEmbedder{}, index))
	topN := 1
	result, err := svc.CreateKnowledgeQuery(context.Background(), service.RequestContext{UserID: "usr_owner"}, service.KnowledgeQueryInput{Query: "query", KnowledgeBaseIDs: []string{"kb_owned"}, TopK: 2, Rerank: true, RerankTopN: &topN})
	if err != nil || len(result.Results) != 1 {
		t.Fatalf("fallback result = %+v, error = %v", result, err)
	}
	index.hits = nil
	empty, err := svc.CreateKnowledgeQuery(context.Background(), service.RequestContext{UserID: "usr_owner"}, service.KnowledgeQueryInput{Query: "missing", KnowledgeBaseIDs: []string{"kb_owned"}, TopK: 2})
	if err != nil || empty.Results == nil || len(empty.Results) != 0 {
		t.Fatalf("empty result = %+v, error = %v", empty, err)
	}
}

func TestKnowledgeQueryRerankFallbackPreservesVectorOrderAndTopN(t *testing.T) {
	repo := repository.NewMemoryRepository()
	seedRetrievalBase(t, repo, "kb_owned", "usr_owner")
	seedRetrievalDocumentWithContent(t, repo, "doc_one", "kb_owned", "usr_owner", "one")
	seedRetrievalDocumentWithContent(t, repo, "doc_two", "kb_owned", "usr_owner", "two")
	index := &retrievalIndex{hits: []service.VectorSearchHit{
		{ID: "point_one", Score: .9, Payload: map[string]any{"chunk_id": "chunk_doc_one"}},
		{ID: "point_two", Score: .8, Payload: map[string]any{"chunk_id": "chunk_doc_two"}},
	}}
	svc := service.NewKnowledgeService(repo, service.WithVectorIndex(retrievalEmbedder{}, index))
	topN := 1
	result, err := svc.CreateKnowledgeQuery(context.Background(), service.RequestContext{UserID: "usr_owner"}, service.KnowledgeQueryInput{Query: "query", KnowledgeBaseIDs: []string{"kb_owned"}, TopK: 2, Rerank: true, RerankTopN: &topN})
	if err != nil {
		t.Fatalf("CreateKnowledgeQuery() error = %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].ChunkID != "chunk_doc_one" || result.Results[0].Score != .9 {
		t.Fatalf("fallback results = %+v", result.Results)
	}
}

func TestKnowledgeQueryWithoutAccessibleBasesReturnsEmptyResults(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := service.NewKnowledgeService(repo, service.WithVectorIndex(retrievalEmbedder{}, &retrievalIndex{}))
	result, err := svc.CreateKnowledgeQuery(context.Background(), service.RequestContext{UserID: "usr_owner"}, service.KnowledgeQueryInput{Query: "missing", TopK: 2})
	if err != nil || result.Results == nil || len(result.Results) != 0 || result.Trace.HitCount != 0 {
		t.Fatalf("empty result = %+v, error = %v", result, err)
	}
}

func TestKnowledgeQueryValidationAndSanitizedRerankError(t *testing.T) {
	repo := repository.NewMemoryRepository()
	seedRetrievalBase(t, repo, "kb_owned", "usr_owner")
	topN := 3
	svc := service.NewKnowledgeService(repo, service.WithVectorIndex(retrievalEmbedder{}, &retrievalIndex{}))
	_, err := svc.CreateKnowledgeQuery(context.Background(), service.RequestContext{UserID: "usr_owner"}, service.KnowledgeQueryInput{Query: " ", TopK: 2, RerankTopN: &topN})
	if err == nil {
		t.Fatal("expected validation error")
	}

	seedRetrievalDocumentWithContent(t, repo, "doc_one", "kb_owned", "usr_owner", "one")
	reranker := &retrievalReranker{err: errors.New("provider secret response")}
	svc = service.NewKnowledgeService(repo, service.WithVectorIndex(retrievalEmbedder{}, &retrievalIndex{hits: []service.VectorSearchHit{{Score: .8, Payload: map[string]any{"chunk_id": "chunk_doc_one"}}}}), service.WithReranker(reranker))
	_, err = svc.CreateKnowledgeQuery(context.Background(), service.RequestContext{UserID: "usr_owner"}, service.KnowledgeQueryInput{Query: "query", KnowledgeBaseIDs: []string{"kb_owned"}, TopK: 1, Rerank: true})
	if err == nil || strings.Contains(err.Error(), "provider secret response") {
		t.Fatalf("rerank error = %v", err)
	}
}

func seedRetrievalBase(t *testing.T, repo *repository.MemoryRepository, id, owner string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.CreateKnowledgeBase(context.Background(), service.KnowledgeBase{ID: id, Name: id, DocType: "GENERAL", CreatedBy: owner, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
}

func seedRetrievalDocument(t *testing.T, repo *repository.MemoryRepository, id, kbID, owner string, status service.DocumentStatus, deletedAt *time.Time, tags []string, metadata map[string]any) {
	t.Helper()
	now := time.Now().UTC()
	repo.PutDocumentForTest(service.KnowledgeDocument{ID: id, KnowledgeBaseID: kbID, FileID: "file_" + id, Name: id + ".md", Status: status, CreatedBy: owner, CreatedAt: now, DeletedAt: deletedAt, Tags: tags})
	if err := repo.ReplaceDocumentChunks(context.Background(), id, []service.DocumentChunk{{ID: "chunk_" + id, KnowledgeBaseID: kbID, DocumentID: id, Content: "content for " + id, Metadata: metadata, CreatedAt: now}}); err != nil {
		t.Fatal(err)
	}
}

func seedRetrievalDocumentWithContent(t *testing.T, repo *repository.MemoryRepository, id, kbID, owner, content string) {
	t.Helper()
	now := time.Now().UTC()
	repo.PutDocumentForTest(service.KnowledgeDocument{ID: id, KnowledgeBaseID: kbID, FileID: "file_" + id, Name: id + ".md", Status: service.DocumentStatusReady, CreatedBy: owner, CreatedAt: now})
	if err := repo.ReplaceDocumentChunks(context.Background(), id, []service.DocumentChunk{{ID: "chunk_" + id, KnowledgeBaseID: kbID, DocumentID: id, Content: content, CreatedAt: now}}); err != nil {
		t.Fatal(err)
	}
}
