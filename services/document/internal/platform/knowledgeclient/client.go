package knowledgeclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/document/internal/service"
)

const defaultTimeout = 10 * time.Second

type Client struct {
	endpoint     string
	serviceToken string
	httpClient   *http.Client
}

func New(baseURL, serviceToken string, httpClient *http.Client) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("DOCUMENT_KNOWLEDGE_SERVICE_URL must be an absolute http(s) URL")
	}
	if parsed.User != nil {
		return nil, errors.New("DOCUMENT_KNOWLEDGE_SERVICE_URL must not contain credentials")
	}
	if strings.TrimSpace(serviceToken) == "" {
		return nil, errors.New("DOCUMENT_KNOWLEDGE_SERVICE_TOKEN is required when DOCUMENT_KNOWLEDGE_SERVICE_URL is set")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{
		endpoint:     strings.TrimRight(parsed.String(), "/") + "/internal/v1/knowledge-queries",
		serviceToken: strings.TrimSpace(serviceToken),
		httpClient:   httpClient,
	}, nil
}

func (c *Client) RetrieveReportContext(ctx context.Context, reqCtx service.RequestContext, input service.ReportKnowledgeRetrievalInput) ([]service.ReportKnowledgeSnippet, error) {
	if strings.TrimSpace(input.Query) == "" {
		return nil, service.ValidationError(map[string]string{"query": "is required"})
	}
	if len(input.KnowledgeBaseIDs) == 0 {
		return []service.ReportKnowledgeSnippet{}, nil
	}
	body, err := json.Marshal(knowledgeQueryRequest{
		Query:            input.Query,
		KnowledgeBaseIDs: input.KnowledgeBaseIDs,
		TopK:             input.TopK,
		ScoreThreshold:   input.ScoreThreshold,
		Rerank:           input.Rerank,
		RerankTopN:       input.RerankTopN,
	})
	if err != nil {
		return nil, service.NewError(service.CodeInternal, "encode knowledge query request", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, service.NewError(service.CodeDependency, "build knowledge query request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Service-Token", c.serviceToken)
	req.Header.Set("X-Caller-Service", "document")
	if strings.TrimSpace(reqCtx.RequestID) != "" {
		req.Header.Set("X-Request-Id", strings.TrimSpace(reqCtx.RequestID))
	}
	if strings.TrimSpace(reqCtx.UserID) != "" {
		req.Header.Set("X-User-Id", strings.TrimSpace(reqCtx.UserID))
	}
	if len(reqCtx.Roles) > 0 {
		req.Header.Set("X-User-Roles", strings.Join(reqCtx.Roles, ","))
	}
	if len(reqCtx.Permissions) > 0 {
		req.Header.Set("X-User-Permissions", strings.Join(reqCtx.Permissions, ","))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, service.NewError(service.CodeDependency, "knowledge retrieval failed", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, service.NewError(service.CodeDependency, "knowledge retrieval failed", fmt.Errorf("status %d", resp.StatusCode))
	}
	var decoded knowledgeQueryResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&decoded); err != nil {
		return nil, service.NewError(service.CodeDependency, "decode knowledge retrieval response", err)
	}
	results := make([]service.ReportKnowledgeSnippet, 0, len(decoded.Data.Results))
	for _, item := range decoded.Data.Results {
		results = append(results, service.ReportKnowledgeSnippet{
			Score:           item.Score,
			KnowledgeBaseID: item.KnowledgeBaseID,
			DocumentID:      item.DocumentID,
			ChunkID:         item.ChunkID,
			DocumentName:    item.DocumentName,
			SectionPath:     item.SectionPath,
			ContentPreview:  item.ContentPreview,
		})
	}
	return results, nil
}

type knowledgeQueryRequest struct {
	Query            string   `json:"query"`
	KnowledgeBaseIDs []string `json:"knowledgeBaseIds,omitempty"`
	TopK             int      `json:"topK,omitempty"`
	ScoreThreshold   *float64 `json:"scoreThreshold,omitempty"`
	Rerank           bool     `json:"rerank,omitempty"`
	RerankTopN       *int     `json:"rerankTopN,omitempty"`
}

type knowledgeQueryResponse struct {
	Data struct {
		Results []struct {
			Score           float64 `json:"score"`
			KnowledgeBaseID string  `json:"knowledgeBaseId"`
			DocumentID      string  `json:"documentId"`
			ChunkID         string  `json:"chunkId"`
			DocumentName    string  `json:"documentName"`
			SectionPath     string  `json:"sectionPath"`
			ContentPreview  string  `json:"contentPreview"`
		} `json:"results"`
	} `json:"data"`
}
