package httpapi

import (
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/knowledge/internal/service"
)

type documentSummary struct {
	ID              string                 `json:"id"`
	KnowledgeBaseID string                 `json:"knowledgeBaseId"`
	Name            string                 `json:"name"`
	ContentType     *string                `json:"contentType,omitempty"`
	SizeBytes       *int64                 `json:"sizeBytes,omitempty"`
	Status          service.DocumentStatus `json:"status"`
	ErrorCode       *string                `json:"errorCode,omitempty"`
	ErrorMessage    *string                `json:"errorMessage,omitempty"`
	ChunkCount      int64                  `json:"chunkCount"`
	Tags            []string               `json:"tags,omitempty"`
	ParserBackend   *string                `json:"parserBackend,omitempty"`
	CreatedBy       string                 `json:"createdBy,omitempty"`
	CreatedAt       time.Time              `json:"createdAt"`
	UpdatedAt       time.Time              `json:"updatedAt"`
	JobID           *string                `json:"jobId,omitempty"`
}

type documentChunkSummary struct {
	ID                 string         `json:"id"`
	KnowledgeBaseID    string         `json:"knowledgeBaseId"`
	DocumentID         string         `json:"documentId"`
	ChunkIndex         int32          `json:"chunkIndex"`
	SectionPath        *string        `json:"sectionPath,omitempty"`
	Content            string         `json:"content"`
	TokenCount         int32          `json:"tokenCount"`
	ChunkType          *string        `json:"chunkType,omitempty"`
	EmbeddingProvider  *string        `json:"embeddingProvider,omitempty"`
	EmbeddingDimension *int32         `json:"embeddingDimension,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	CreatedAt          time.Time      `json:"createdAt"`
}

func documentFromDomain(doc service.KnowledgeDocument) documentSummary {
	tags := append([]string(nil), doc.Tags...)
	return documentSummary{
		ID:              doc.ID,
		KnowledgeBaseID: doc.KnowledgeBaseID,
		Name:            doc.Name,
		ContentType:     doc.ContentType,
		SizeBytes:       doc.SizeBytes,
		Status:          doc.Status,
		ErrorCode:       doc.ErrorCode,
		ErrorMessage:    doc.ErrorMessage,
		ChunkCount:      doc.ChunkCount,
		Tags:            tags,
		ParserBackend:   doc.ParserBackend,
		CreatedBy:       doc.CreatedBy,
		CreatedAt:       doc.CreatedAt,
		UpdatedAt:       doc.UpdatedAt,
		JobID:           doc.CurrentJobID,
	}
}

func documentsFromDomain(items []service.KnowledgeDocument) []documentSummary {
	out := make([]documentSummary, 0, len(items))
	for _, item := range items {
		out = append(out, documentFromDomain(item))
	}
	return out
}

func documentChunkFromDomain(chunk service.DocumentChunk) documentChunkSummary {
	var tokenCount int32
	if chunk.TokenCount != nil {
		tokenCount = *chunk.TokenCount
	}
	return documentChunkSummary{
		ID:                 chunk.ID,
		KnowledgeBaseID:    chunk.KnowledgeBaseID,
		DocumentID:         chunk.DocumentID,
		ChunkIndex:         chunk.ChunkIndex,
		SectionPath:        cloneStringPtr(chunk.SectionPath),
		Content:            chunk.Content,
		TokenCount:         tokenCount,
		ChunkType:          cloneStringPtr(chunk.ChunkType),
		EmbeddingProvider:  cloneStringPtr(chunk.EmbeddingProvider),
		EmbeddingDimension: cloneInt32Ptr(chunk.EmbeddingDimension),
		Metadata:           cloneMetadata(chunk.Metadata),
		CreatedAt:          chunk.CreatedAt,
	}
}

func documentChunksFromDomain(items []service.DocumentChunk) []documentChunkSummary {
	out := make([]documentChunkSummary, 0, len(items))
	for _, item := range items {
		out = append(out, documentChunkFromDomain(item))
	}
	return out
}

func cloneInt32Ptr(value *int32) *int32 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneMetadata(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	clone := make(map[string]any, len(value))
	for key, item := range value {
		clone[key] = item
	}
	return clone
}
