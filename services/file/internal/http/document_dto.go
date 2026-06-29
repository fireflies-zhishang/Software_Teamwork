package httpapi

import (
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/file/internal/service"
)

type documentSummary struct {
	ID              string                 `json:"id"`
	KnowledgeBaseID string                 `json:"knowledgeBaseId"`
	Name            string                 `json:"name"`
	Status          service.DocumentStatus `json:"status"`
	Tags            []string               `json:"tags,omitempty"`
	ErrorMessage    *string                `json:"errorMessage,omitempty"`
	CreatedAt       string                 `json:"createdAt"`
	ContentType     string                 `json:"contentType"`
	SizeBytes       int64                  `json:"sizeBytes"`
}

type fileObjectResponse struct {
	ID             string  `json:"id"`
	Filename       string  `json:"filename"`
	ContentType    string  `json:"contentType"`
	SizeBytes      int64   `json:"sizeBytes"`
	ChecksumSHA256 *string `json:"checksumSha256"`
	CreatedAt      string  `json:"createdAt"`
	DeletedAt      *string `json:"deletedAt"`
}

func documentSummaryFromDomain(doc service.Document) documentSummary {
	return documentSummary{
		ID:              doc.ID,
		KnowledgeBaseID: doc.KnowledgeBaseID,
		Name:            doc.Name,
		Status:          doc.Status,
		Tags:            append([]string(nil), doc.Tags...),
		ErrorMessage:    doc.ErrorMessage,
		CreatedAt:       doc.CreatedAt.UTC().Format(time.RFC3339),
		ContentType:     doc.ContentType,
		SizeBytes:       doc.SizeBytes,
	}
}

func fileObjectFromDomain(file service.FileObject) fileObjectResponse {
	var checksum *string
	if file.ChecksumSHA256 != "" {
		value := file.ChecksumSHA256
		checksum = &value
	}
	var deletedAt *string
	if file.DeletedAt != nil {
		value := file.DeletedAt.UTC().Format(time.RFC3339)
		deletedAt = &value
	}
	return fileObjectResponse{
		ID:             file.ID,
		Filename:       file.Filename,
		ContentType:    file.ContentType,
		SizeBytes:      file.SizeBytes,
		ChecksumSHA256: checksum,
		CreatedAt:      file.CreatedAt.UTC().Format(time.RFC3339),
		DeletedAt:      deletedAt,
	}
}
