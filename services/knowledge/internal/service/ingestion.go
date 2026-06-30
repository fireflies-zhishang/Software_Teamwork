package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var errVectorCleanupFailed = errors.New("vector cleanup failed")

func (s *Service) ProcessIngestionTask(ctx context.Context, reqCtx RequestContext, task DocumentIngestionTask) (ProcessingJob, error) {
	normalized, err := normalizeIngestionTask(task)
	if err != nil {
		return ProcessingJob{}, err
	}
	reqCtx.RequestID = strings.TrimSpace(firstNonEmpty(reqCtx.RequestID, normalized.RequestID))
	reqCtx.UserID = strings.TrimSpace(firstNonEmpty(reqCtx.UserID, normalized.UserID))
	if strings.TrimSpace(reqCtx.CallerService) == "" {
		reqCtx.CallerService = "knowledge"
	}
	scope := AccessScope{UserID: reqCtx.UserID}
	if scope.UserID == "" {
		return ProcessingJob{}, UnauthorizedError()
	}

	job, err := s.repo.GetProcessingJob(ctx, normalized.JobID)
	if err != nil {
		return ProcessingJob{}, repositoryError(err)
	}
	if !isIngestionJobType(job.JobType) {
		return ProcessingJob{}, ConflictError("job type is not supported by ingestion pipeline", nil)
	}
	if job.DocumentID == nil || strings.TrimSpace(*job.DocumentID) == "" {
		return ProcessingJob{}, ConflictError("job has no document", nil)
	}
	if job.KnowledgeBaseID != normalized.KnowledgeBaseID || strings.TrimSpace(*job.DocumentID) != normalized.DocumentID {
		return ProcessingJob{}, ConflictError("worker payload does not match job", nil)
	}
	now := s.now()
	staleRunningBefore := runningStaleBefore(now, s.runningLease)
	if job.Status == JobStatusFailed && hasExhaustedJobAttempts(job) {
		return job, ConflictError("job has reached max attempts", nil)
	}
	if job.Status == JobStatusRunning && isStaleRunningJob(job, staleRunningBefore) && hasExhaustedJobAttempts(job) {
		failed, failErr := s.failProcessing(ctx, job, normalized.DocumentID, string(CodeDependency), "ingestion job reached max attempts")
		if failErr != nil {
			return failed, failErr
		}
		return failed, ConflictError("job has reached max attempts", nil)
	}
	if job.Status == JobStatusRunning && !isStaleRunningJob(job, staleRunningBefore) {
		return job, DependencyError("job is already running", nil)
	}
	if job.Status == JobStatusSucceeded {
		if s.vectorIndex != nil {
			if err := s.vectorIndex.DeleteStaleDocumentPoints(ctx, normalized.DocumentID, ingestionAttemptID(job)); err != nil {
				return job, DependencyError("stale vector cleanup failed", err)
			}
		}
		return job, nil
	}
	if job.Status != JobStatusQueued && job.Status != JobStatusFailed && job.Status != JobStatusRunning {
		return job, ConflictError("job is not ready to run", nil)
	}

	doc, err := s.repo.GetDocument(ctx, normalized.DocumentID, scope)
	if err != nil {
		return ProcessingJob{}, repositoryError(err)
	}
	if doc.KnowledgeBaseID != normalized.KnowledgeBaseID {
		return ProcessingJob{}, ConflictError("worker payload does not match document", nil)
	}
	kb, err := s.repo.GetKnowledgeBase(ctx, doc.KnowledgeBaseID, AccessScope{CanReadAll: true})
	if err != nil {
		return ProcessingJob{}, repositoryError(err)
	}

	startedAt := now
	parsingStage := "parsing"
	job, err = s.repo.ClaimProcessingJob(ctx, job.ID, JobStateUpdate{
		Status:             JobStatusRunning,
		CurrentStage:       &parsingStage,
		ProgressPercent:    20,
		StartedAt:          &startedAt,
		UpdatedAt:          startedAt,
		StaleRunningBefore: staleRunningBefore,
	})
	if err != nil {
		if errors.Is(err, ErrConflict) {
			latest, latestErr := s.repo.GetProcessingJob(ctx, job.ID)
			if latestErr != nil {
				return job, DependencyError("job state update failed", latestErr)
			}
			if latest.Status == JobStatusRunning {
				return latest, DependencyError("job is already running", err)
			}
			if latest.Status == JobStatusFailed && hasExhaustedJobAttempts(latest) {
				return latest, ConflictError("job has reached max attempts", err)
			}
			return latest, ConflictError("job is not ready to run", err)
		}
		return ProcessingJob{}, DependencyError("job state update failed", err)
	}
	if s.source == nil || s.parser == nil || s.chunker == nil {
		return s.failProcessingAndReturn(ctx, job, normalized.DocumentID, string(CodeDependency), "processing pipeline is not configured",
			DependencyError("processing pipeline is not configured", nil))
	}
	if doc.FileRef == nil || strings.TrimSpace(*doc.FileRef) == "" {
		return s.failProcessingAndReturn(ctx, job, doc.ID, string(CodeDependency), "document source is not configured",
			DependencyError("document source is not configured", nil))
	}
	if _, err := s.repo.UpdateDocumentProcessingState(ctx, doc.ID, DocumentStateUpdate{
		Status:    DocumentStatusParsing,
		UpdatedAt: startedAt,
	}); err != nil {
		return s.failProcessingAndReturn(ctx, job, doc.ID, string(CodeDependency), "document state update failed",
			DependencyError("document state update failed", err))
	}

	source, err := s.source.ReadSource(ctx, reqCtx, strings.TrimSpace(*doc.FileRef))
	if err != nil {
		return s.failProcessingAndReturn(ctx, job, doc.ID, string(CodeDependency), "source content read failed",
			DependencyError("source content read failed", err))
	}
	defer source.Body.Close()

	contentType := strings.TrimSpace(source.ContentType)
	if contentType == "" && doc.ContentType != nil {
		contentType = strings.TrimSpace(*doc.ContentType)
	}
	parsed, err := s.parser.Parse(ctx, ParseInput{
		Name:        doc.Name,
		ContentType: contentType,
		Body:        source.Body,
		SizeBytes:   source.SizeBytes,
		RequestID:   reqCtx.RequestID,
		UserID:      reqCtx.UserID,
	})
	if err != nil {
		code := "parse_failed"
		message := "document parsing failed"
		if appErr, ok := Classify(err); ok && appErr.Code == CodeDependency {
			code = string(CodeDependency)
		}
		if code == string(CodeDependency) {
			return s.failProcessingAndReturn(ctx, job, doc.ID, code, message, DependencyError(message, err))
		}
		return s.failProcessingAndReturn(ctx, job, doc.ID, code, message,
			ValidationError(message, map[string]string{"file": "could not be parsed"}))
	}
	parserBackend := strings.TrimSpace(parsed.Backend)
	var parserBackendPtr *string
	if parserBackend != "" {
		parserBackendPtr = &parserBackend
	}

	chunkingAt := s.now()
	chunkingStage := "chunking"
	job, err = s.repo.UpdateJobState(ctx, job.ID, JobStateUpdate{
		Status:           JobStatusRunning,
		CurrentStage:     &chunkingStage,
		ProgressPercent:  60,
		UpdatedAt:        chunkingAt,
		ExpectedAttempts: &job.Attempts,
	})
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return job, ConflictError("job attempt is no longer active", err)
		}
		return s.failProcessingAndReturn(ctx, job, doc.ID, string(CodeDependency), "job state update failed",
			DependencyError("job state update failed", err))
	}
	if _, err := s.repo.UpdateDocumentProcessingState(ctx, doc.ID, DocumentStateUpdate{
		Status:    DocumentStatusChunking,
		UpdatedAt: chunkingAt,
	}); err != nil {
		return s.failProcessingAndReturn(ctx, job, doc.ID, string(CodeDependency), "document state update failed",
			DependencyError("document state update failed", err))
	}

	chunkSpecs, err := s.chunker.Chunk(ctx, ChunkInput{
		Content:  parsed.Content,
		Strategy: kb.ChunkStrategy,
	})
	if err != nil {
		return s.failProcessingAndReturn(ctx, job, doc.ID, "chunk_failed", "document chunking failed",
			ValidationError("document chunking failed", map[string]string{"content": "could not be chunked"}))
	}
	chunks := make([]DocumentChunk, 0, len(chunkSpecs))
	for index, spec := range chunkSpecs {
		chunkID := s.newID("chunk")
		tokenCount := int32(spec.TokenCount)
		metadata := cloneMetadata(spec.Metadata)
		addParsedPageMetadata(metadata, spec.Content, parsed.Pages)
		chunks = append(chunks, DocumentChunk{
			ID:              chunkID,
			KnowledgeBaseID: doc.KnowledgeBaseID,
			DocumentID:      doc.ID,
			ChunkIndex:      int32(index),
			SectionPath:     spec.SectionPath,
			Content:         spec.Content,
			TokenCount:      &tokenCount,
			ChunkType:       spec.ChunkType,
			Metadata:        metadata,
			CreatedAt:       s.now(),
		})
	}

	if s.embedder != nil && s.vectorIndex != nil {
		embeddingAt := s.now()
		embeddingStage := "embedding"
		job, err = s.repo.UpdateJobState(ctx, job.ID, JobStateUpdate{
			Status:           JobStatusRunning,
			CurrentStage:     &embeddingStage,
			ProgressPercent:  80,
			UpdatedAt:        embeddingAt,
			ExpectedAttempts: &job.Attempts,
		})
		if err != nil {
			if errors.Is(err, ErrConflict) {
				return job, ConflictError("job attempt is no longer active", err)
			}
			return s.failProcessingAndReturn(ctx, job, doc.ID, string(CodeDependency), "job state update failed",
				DependencyError("job state update failed", err))
		}
		if _, err := s.repo.UpdateDocumentProcessingState(ctx, doc.ID, DocumentStateUpdate{
			Status:    DocumentStatusEmbedding,
			UpdatedAt: embeddingAt,
		}); err != nil {
			return s.failProcessingAndReturn(ctx, job, doc.ID, string(CodeDependency), "document state update failed",
				DependencyError("document state update failed", err))
		}
		if err := s.embedAndIndex(ctx, reqCtx, job, doc, chunks); err != nil {
			if errors.Is(err, ErrConflict) {
				return job, ConflictError("job attempt is no longer active", err)
			}
			if errors.Is(err, errVectorCleanupFailed) {
				return job, DependencyError("stale vector cleanup failed", err)
			}
			message := sanitizeProcessingFailureMessage(err)
			return s.failProcessingAndReturn(ctx, job, doc.ID, classifyProcessingDependencyCode(err), message,
				DependencyError(message, err))
		}
	}

	finishedAt := s.now()
	completed, err := s.repo.CompleteIngestion(ctx, CompleteIngestionRecord{
		DocumentID:       doc.ID,
		JobID:            job.ID,
		ExpectedAttempts: &job.Attempts,
		ParserBackend:    parserBackendPtr,
		Chunks:           chunks,
		UpdatedAt:        finishedAt,
		FinishedAt:       finishedAt,
	})
	if err != nil {
		if errors.Is(err, ErrConflict) {
			if s.vectorIndex != nil {
				if cleanupErr := s.vectorIndex.DeleteByDocumentIngestionAttempt(ctx, doc.ID, ingestionAttemptID(job)); cleanupErr != nil {
					return job, DependencyError("stale vector cleanup failed", cleanupErr)
				}
			}
			return job, ConflictError("job attempt is no longer active", err)
		}
		if s.vectorIndex != nil {
			if cleanupErr := s.vectorIndex.DeleteByDocumentIngestionAttempt(ctx, doc.ID, ingestionAttemptID(job)); cleanupErr != nil {
				return job, DependencyError("stale vector cleanup failed", cleanupErr)
			}
		}
		return s.failProcessingAndReturn(ctx, job, doc.ID, string(CodeDependency), "ingestion completion failed",
			DependencyError("ingestion completion failed", err))
	}
	if s.vectorIndex != nil && len(chunks) > 0 {
		if err := s.vectorIndex.DeleteStaleDocumentPoints(ctx, doc.ID, ingestionAttemptID(job)); err != nil {
			return completed, DependencyError("stale vector cleanup failed", err)
		}
	}
	return completed, nil
}

func (s *Service) ProcessIngestionJob(ctx context.Context, reqCtx RequestContext, jobID string) (ProcessingJob, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return ProcessingJob{}, ValidationError("worker payload validation failed", map[string]string{"jobId": "is required"})
	}
	job, err := s.repo.GetProcessingJob(ctx, jobID)
	if err != nil {
		return ProcessingJob{}, repositoryError(err)
	}
	if job.DocumentID == nil || strings.TrimSpace(*job.DocumentID) == "" {
		return ProcessingJob{}, ConflictError("job has no document", nil)
	}
	return s.ProcessIngestionTask(ctx, reqCtx, DocumentIngestionTask{
		RequestID:       reqCtx.RequestID,
		JobID:           job.ID,
		DocumentID:      strings.TrimSpace(*job.DocumentID),
		KnowledgeBaseID: job.KnowledgeBaseID,
		UserID:          reqCtx.UserID,
	})
}

func (s *Service) GetJob(ctx context.Context, reqCtx RequestContext, id string) (ProcessingJob, error) {
	scope, err := readScope(reqCtx)
	if err != nil {
		return ProcessingJob{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ProcessingJob{}, ValidationError("request validation failed", map[string]string{"jobId": "is required"})
	}
	job, err := s.repo.GetProcessingJob(ctx, id)
	if err != nil {
		return ProcessingJob{}, repositoryError(err)
	}
	if job.DocumentID != nil && strings.TrimSpace(*job.DocumentID) != "" {
		if _, err := s.repo.GetDocument(ctx, strings.TrimSpace(*job.DocumentID), scope); err != nil {
			return ProcessingJob{}, repositoryError(err)
		}
		return job, nil
	}
	if _, err := s.repo.GetKnowledgeBase(ctx, job.KnowledgeBaseID, scope); err != nil {
		return ProcessingJob{}, repositoryError(err)
	}
	return job, nil
}

func (s *Service) ListChunks(ctx context.Context, reqCtx RequestContext, input ListChunksInput) (ChunkList, error) {
	scope, err := readScope(reqCtx)
	if err != nil {
		return ChunkList{}, err
	}
	documentID := strings.TrimSpace(input.DocumentID)
	if documentID == "" {
		return ChunkList{}, ValidationError("request validation failed", map[string]string{"documentId": "is required"})
	}
	page, err := normalizePage(input.Page)
	if err != nil {
		return ChunkList{}, err
	}
	chunks, err := s.repo.ListChunks(ctx, documentID, scope, page)
	if err != nil {
		return ChunkList{}, repositoryError(err)
	}
	return chunks, nil
}

func (s *Service) embedAndIndex(ctx context.Context, reqCtx RequestContext, job ProcessingJob, doc KnowledgeDocument, chunks []DocumentChunk) error {
	texts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		texts = append(texts, chunk.Content)
	}
	result, err := s.embedder.Embed(ctx, EmbeddingRequest{
		Texts:     texts,
		RequestID: reqCtx.RequestID,
		UserID:    reqCtx.UserID,
	})
	if err != nil {
		return err
	}
	if len(result.Vectors) != len(chunks) {
		return fmt.Errorf("embedding result count mismatch")
	}
	ingestionAttempt := ingestionAttemptID(job)
	points := make([]VectorPoint, 0, len(chunks))
	for index := range chunks {
		pointID := stableVectorPointID(chunks[index].ID)
		dimension := int32(result.Dimension)
		chunks[index].QdrantPointID = &pointID
		chunks[index].EmbeddingProvider = &result.Provider
		chunks[index].EmbeddingModel = &result.Model
		chunks[index].EmbeddingDimension = &dimension
		points = append(points, VectorPoint{
			ID:     pointID,
			Vector: append([]float32(nil), result.Vectors[index]...),
			Payload: map[string]any{
				"knowledge_base_id":           chunks[index].KnowledgeBaseID,
				VectorPayloadDocumentID:       chunks[index].DocumentID,
				"chunk_id":                    chunks[index].ID,
				"chunk_index":                 chunks[index].ChunkIndex,
				"chunk_type":                  derefString(chunks[index].ChunkType),
				"section_path":                derefString(chunks[index].SectionPath),
				"tags":                        append([]string(nil), doc.Tags...),
				"metadata":                    cloneMetadata(chunks[index].Metadata),
				"job_id":                      job.ID,
				"job_attempt":                 job.Attempts,
				VectorPayloadIngestionAttempt: ingestionAttempt,
			},
		})
	}
	if err := s.fenceIngestionAttempt(ctx, job, 90); err != nil {
		return err
	}
	if err := s.vectorIndex.Upsert(ctx, points); err != nil {
		return err
	}
	if err := s.fenceIngestionAttempt(ctx, job, 95); err != nil {
		if cleanupErr := s.vectorIndex.DeleteByDocumentIngestionAttempt(ctx, doc.ID, ingestionAttempt); cleanupErr != nil {
			return fmt.Errorf("%w: %w", errVectorCleanupFailed, cleanupErr)
		}
		return err
	}
	return nil
}

func (s *Service) fenceIngestionAttempt(ctx context.Context, job ProcessingJob, progress int32) error {
	embeddingStage := "embedding"
	_, err := s.repo.UpdateJobState(ctx, job.ID, JobStateUpdate{
		Status:           JobStatusRunning,
		CurrentStage:     &embeddingStage,
		ProgressPercent:  progress,
		UpdatedAt:        s.now(),
		ExpectedAttempts: &job.Attempts,
	})
	return err
}

func (s *Service) failProcessingAndReturn(ctx context.Context, job ProcessingJob, documentID string, code string, message string, processingErr error) (ProcessingJob, error) {
	failed, err := s.failProcessing(ctx, job, documentID, code, message)
	if err != nil {
		return failed, err
	}
	return failed, processingErr
}

func (s *Service) failProcessing(ctx context.Context, job ProcessingJob, documentID string, code string, message string) (ProcessingJob, error) {
	now := s.now()
	if err := s.repo.MarkDocumentJobFailed(ctx, documentID, job.ID, &job.Attempts, code, message, now); err != nil {
		if errors.Is(err, ErrConflict) {
			return job, ConflictError("job attempt is no longer active", err)
		}
		return job, DependencyError("failed to persist ingestion failure state", err)
	}
	failed, err := s.repo.GetProcessingJob(ctx, job.ID)
	if err != nil {
		return job, DependencyError("failed to reload ingestion failure state", err)
	}
	return failed, nil
}

func normalizeIngestionTask(task DocumentIngestionTask) (DocumentIngestionTask, error) {
	task.RequestID = strings.TrimSpace(task.RequestID)
	task.JobID = strings.TrimSpace(task.JobID)
	task.DocumentID = strings.TrimSpace(task.DocumentID)
	task.KnowledgeBaseID = strings.TrimSpace(task.KnowledgeBaseID)
	task.UserID = strings.TrimSpace(task.UserID)
	fields := map[string]string{}
	if task.RequestID == "" {
		fields["requestId"] = "is required"
	}
	if task.JobID == "" {
		fields["jobId"] = "is required"
	}
	if task.DocumentID == "" {
		fields["documentId"] = "is required"
	}
	if task.KnowledgeBaseID == "" {
		fields["knowledgeBaseId"] = "is required"
	}
	if task.UserID == "" {
		fields["userId"] = "is required"
	}
	if len(fields) > 0 {
		return DocumentIngestionTask{}, ValidationError("worker payload validation failed", fields)
	}
	return task, nil
}

func isIngestionJobType(jobType string) bool {
	switch strings.TrimSpace(jobType) {
	case JobTypeIngest, LegacyJobTypeIngestion:
		return true
	default:
		return false
	}
}

func stableVectorPointID(chunkID string) string {
	sum := sha256.Sum256([]byte(chunkID))
	encoded := hex.EncodeToString(sum[:16])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}

func ingestionAttemptID(job ProcessingJob) string {
	return fmt.Sprintf("%s:%d", strings.TrimSpace(job.ID), job.Attempts)
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func addParsedPageMetadata(metadata map[string]any, chunkContent string, pages []ParsedPage) {
	if metadata == nil || len(pages) == 0 {
		return
	}
	pageNumber := 0
	if len(pages) == 1 {
		pageNumber = pages[0].PageNumber
	} else {
		chunkText := strings.TrimSpace(chunkContent)
		if chunkText == "" {
			return
		}
		for _, page := range pages {
			if page.PageNumber <= 0 {
				continue
			}
			if strings.Contains(page.Content, chunkText) {
				pageNumber = page.PageNumber
				break
			}
		}
	}
	if pageNumber <= 0 {
		return
	}
	metadata["page_start"] = pageNumber
	metadata["page_end"] = pageNumber
	metadata["source_pages"] = []int{pageNumber}
	for _, page := range pages {
		if page.PageNumber != pageNumber {
			continue
		}
		if strings.TrimSpace(page.ParseStrategy) != "" {
			metadata["parse_strategy"] = strings.TrimSpace(page.ParseStrategy)
		}
		if strings.TrimSpace(page.TextLayerStatus) != "" {
			metadata["text_layer_status"] = strings.TrimSpace(page.TextLayerStatus)
		}
		if page.OCRConfidence != nil {
			metadata["ocr_confidence"] = *page.OCRConfidence
		}
		if page.DPI != nil {
			metadata["dpi"] = *page.DPI
		}
		if len(page.Warnings) > 0 {
			metadata["parse_warnings"] = append([]string(nil), page.Warnings...)
		}
		return
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func classifyProcessingDependencyCode(err error) string {
	if appErr, ok := Classify(err); ok {
		return string(appErr.Code)
	}
	return string(CodeDependency)
}

func hasExhaustedJobAttempts(job ProcessingJob) bool {
	return job.MaxAttempts > 0 && job.Attempts >= job.MaxAttempts
}

func runningStaleBefore(now time.Time, lease time.Duration) *time.Time {
	if lease <= 0 {
		return nil
	}
	cutoff := now.Add(-lease)
	return &cutoff
}

func isStaleRunningJob(job ProcessingJob, staleBefore *time.Time) bool {
	if job.Status != JobStatusRunning || staleBefore == nil {
		return false
	}
	if job.UpdatedAt.Before(*staleBefore) {
		return true
	}
	return false
}

func sanitizeProcessingFailureMessage(err error) string {
	if appErr, ok := Classify(err); ok && appErr.Code != "" {
		switch appErr.Code {
		case CodeValidation:
			return "document processing failed"
		default:
			return appErr.Message
		}
	}
	return "document processing failed"
}
