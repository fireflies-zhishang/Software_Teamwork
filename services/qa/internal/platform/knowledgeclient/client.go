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

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/qa/internal/service"
)

type Client struct {
	endpoint     string
	serviceToken string
	http         *http.Client
}

func New(baseURL, serviceToken string, timeout time.Duration) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("knowledge service URL must be absolute http(s)")
	}
	if strings.TrimSpace(serviceToken) == "" {
		return nil, errors.New("service token is required")
	}
	if timeout <= 0 {
		return nil, errors.New("knowledge request timeout must be positive")
	}
	return &Client{endpoint: strings.TrimRight(parsed.String(), "/") + "/internal/v1/knowledge-queries", serviceToken: serviceToken, http: &http.Client{Timeout: timeout}}, nil
}

func (c *Client) Retrieve(ctx context.Context, userID string, input service.RetrievalTestInput) ([]service.RetrievalTestResult, error) {
	payload := map[string]any{"query": input.Question, "knowledgeBaseIds": input.KnowledgeBaseIDs}
	retrieval := input.Retrieval
	if retrieval.TopK == 0 {
		retrieval = input.Overrides
	}
	if retrieval.TopK > 0 {
		payload["topK"] = retrieval.TopK
	}
	if retrieval.ScoreThreshold > 0 {
		payload["scoreThreshold"] = retrieval.ScoreThreshold
	}
	payload["rerank"] = retrieval.EnableRerank
	if retrieval.RerankTopN > 0 {
		payload["rerankTopN"] = retrieval.RerankTopN
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode knowledge query: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create knowledge query: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Token", c.serviceToken)
	req.Header.Set("X-Caller-Service", "qa")
	req.Header.Set("X-User-Id", userID)
	if requestID := service.RequestIDFromContext(ctx); requestID != "" {
		req.Header.Set("X-Request-Id", requestID)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call knowledge service: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code, message := decodeErrorCode(resp.Body)
		if code == "forbidden" || (code == "not_found" && len(input.KnowledgeBaseIDs) > 0 && strings.Contains(message, "resource not found")) {
			return nil, service.NewError(service.CodeForbidden, "knowledge base access is forbidden", nil)
		}
		return nil, service.NewError(service.CodeDependency, "knowledge retrieval failed", fmt.Errorf("knowledge service returned HTTP %d", resp.StatusCode))
	}
	var decoded struct {
		Data struct {
			Results []struct {
				Score           float64        `json:"score"`
				VectorScore     *float64       `json:"vectorScore"`
				RerankScore     *float64       `json:"rerankScore"`
				KnowledgeBaseID string         `json:"knowledgeBaseId"`
				DocumentID      string         `json:"documentId"`
				ChunkID         string         `json:"chunkId"`
				DocumentName    string         `json:"documentName"`
				SectionPath     string         `json:"sectionPath"`
				ContentPreview  string         `json:"contentPreview"`
				ChunkIndex      *int           `json:"chunkIndex"`
				ChunkType       *string        `json:"chunkType"`
				Tags            []string       `json:"tags"`
				Metadata        map[string]any `json:"metadata"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode knowledge response: %w", err)
	}
	results := make([]service.RetrievalTestResult, 0, len(decoded.Data.Results))
	for i, item := range decoded.Data.Results {
		var vectorScore *float64
		if item.VectorScore != nil {
			score := *item.VectorScore
			vectorScore = &score
		} else if !retrieval.EnableRerank {
			score := item.Score
			vectorScore = &score
		}
		rerankScore := item.RerankScore
		if rerankScore == nil && retrieval.EnableRerank {
			score := item.Score
			rerankScore = &score
		}
		metadata := sanitizedMetadata(item.Metadata)
		if item.ChunkIndex != nil {
			metadata["chunkIndex"] = *item.ChunkIndex
		}
		if item.ChunkType != nil && strings.TrimSpace(*item.ChunkType) != "" {
			metadata["chunkType"] = strings.TrimSpace(*item.ChunkType)
		}
		if len(item.Tags) > 0 {
			metadata["tags"] = append([]string(nil), item.Tags...)
		}
		results = append(results, service.RetrievalTestResult{RankNo: i + 1, KnowledgeBaseID: item.KnowledgeBaseID, DocumentID: item.DocumentID, DocID: item.DocumentID, ChunkID: item.ChunkID, DocumentName: item.DocumentName, DocName: item.DocumentName, SectionPath: item.SectionPath, Score: item.Score, VectorScore: vectorScore, RerankScore: rerankScore, ContentPreview: item.ContentPreview, Text: item.ContentPreview, Metadata: metadata})
	}
	return results, nil
}

func decodeErrorCode(body io.Reader) (string, string) {
	var decoded struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 4096)).Decode(&decoded); err != nil {
		return "", ""
	}
	return strings.TrimSpace(decoded.Error.Code), strings.TrimSpace(decoded.Error.Message)
}

func sanitizedMetadata(input map[string]any) map[string]any {
	metadata := map[string]any{}
	for key, value := range input {
		switch key {
		case "vector", "embedding", "payload", "prompt", "internalUrl", "objectKey":
			continue
		default:
			metadata[key] = value
		}
	}
	return metadata
}
