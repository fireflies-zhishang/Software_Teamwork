package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/knowledge/internal/service"
)

type Config struct {
	ServiceVersion string
	Environment    string
	Logger         *slog.Logger
	MaxUploadBytes int64
}

type Server struct {
	knowledge      *service.Service
	serviceVersion string
	environment    string
	logger         *slog.Logger
	maxUploadBytes int64
	mux            *http.ServeMux
}

const defaultMaxUploadBytes = int64(32 << 20)

func NewServer(knowledge *service.Service, cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.MaxUploadBytes <= 0 {
		cfg.MaxUploadBytes = defaultMaxUploadBytes
	}
	s := &Server{
		knowledge:      knowledge,
		serviceVersion: cfg.ServiceVersion,
		environment:    cfg.Environment,
		logger:         cfg.Logger,
		maxUploadBytes: cfg.MaxUploadBytes,
		mux:            http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /readyz", s.handleReady)
	s.mux.HandleFunc("GET /internal/v1/knowledge-bases", s.handleListKnowledgeBases)
	s.mux.HandleFunc("POST /internal/v1/knowledge-bases", s.handleCreateKnowledgeBase)
	s.mux.HandleFunc("GET /internal/v1/knowledge-bases/{knowledgeBaseId}", s.handleGetKnowledgeBase)
	s.mux.HandleFunc("PATCH /internal/v1/knowledge-bases/{knowledgeBaseId}", s.handleUpdateKnowledgeBase)
	s.mux.HandleFunc("DELETE /internal/v1/knowledge-bases/{knowledgeBaseId}", s.handleDeleteKnowledgeBase)
	s.mux.HandleFunc("GET /internal/v1/knowledge-bases/{knowledgeBaseId}/documents", s.handleListDocuments)
	s.mux.HandleFunc("POST /internal/v1/knowledge-bases/{knowledgeBaseId}/documents", s.handleUploadDocument)
	s.mux.HandleFunc("GET /internal/v1/documents/{documentId}", s.handleGetDocument)
	s.mux.HandleFunc("GET /internal/v1/documents/{documentId}/chunks", s.handleListDocumentChunks)
	s.mux.HandleFunc("GET /internal/v1/documents/{documentId}/content", s.handleGetDocumentContent)
	s.mux.HandleFunc("POST /internal/v1/knowledge-queries", s.handleCreateKnowledgeQuery)
	s.mux.HandleFunc("GET /internal/v1/parser-configs", s.handleListParserConfigs)
	s.mux.HandleFunc("POST /internal/v1/parser-configs", s.handleCreateParserConfig)
	s.mux.HandleFunc("GET /internal/v1/parser-configs/{parserConfigId}", s.handleGetParserConfig)
	s.mux.HandleFunc("PATCH /internal/v1/parser-configs/{parserConfigId}", s.handleUpdateParserConfig)
	s.mux.HandleFunc("DELETE /internal/v1/parser-configs/{parserConfigId}", s.handleDeleteParserConfig)
	s.mux.HandleFunc("/", s.handleNotFound)
}

func (s *Server) handleListParserConfigs(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	var enabled *bool
	if raw := strings.TrimSpace(r.URL.Query().Get("enabled")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			writeAppError(w, r, service.ValidationError("request validation failed", map[string]string{"enabled": "must be a boolean"}))
			return
		}
		enabled = &value
	}
	result, err := s.knowledge.ListParserConfigs(r.Context(), reqCtx, enabled)
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, parserConfigsFromDomain(result.Items), requestIDFromContext(r.Context()))
}

func (s *Server) handleCreateParserConfig(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	var body createParserConfigRequest
	if !decodeJSONBody(w, r, &body) {
		return
	}
	result, err := s.knowledge.CreateParserConfig(r.Context(), reqCtx, service.CreateParserConfigInput{Name: body.Name, Backend: body.Backend, Enabled: body.Enabled, IsDefault: body.IsDefault, Concurrency: body.Concurrency, SupportedContentTypes: body.SupportedContentTypes, EndpointURL: body.EndpointURL, DefaultParameters: body.DefaultParameters})
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, parserConfigFromDomain(result), requestIDFromContext(r.Context()))
}

func (s *Server) handleGetParserConfig(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	result, err := s.knowledge.GetParserConfig(r.Context(), reqCtx, r.PathValue("parserConfigId"))
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, parserConfigFromDomain(result), requestIDFromContext(r.Context()))
}

func (s *Server) handleUpdateParserConfig(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	var body updateParserConfigRequest
	if !decodeJSONBody(w, r, &body) {
		return
	}
	endpointURL, ok, err := parseNullableString(body.EndpointURL)
	if err != nil {
		writeAppError(w, r, service.ValidationError("request validation failed", map[string]string{"endpointUrl": "must be a string or null"}))
		return
	}
	var endpointURLPatch **string
	if ok {
		endpointURLPatch = &endpointURL
	}
	result, err := s.knowledge.UpdateParserConfig(r.Context(), reqCtx, service.UpdateParserConfigInput{ID: r.PathValue("parserConfigId"), Name: body.Name, Backend: body.Backend, Enabled: body.Enabled, IsDefault: body.IsDefault, Concurrency: body.Concurrency, SupportedContentTypes: body.SupportedContentTypes, EndpointURL: endpointURLPatch, DefaultParameters: body.DefaultParameters})
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, parserConfigFromDomain(result), requestIDFromContext(r.Context()))
}
func (s *Server) handleDeleteParserConfig(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	if err := s.knowledge.DeleteParserConfig(r.Context(), reqCtx, r.PathValue("parserConfigId")); err != nil {
		writeAppError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
	if requestID == "" {
		requestID = newRequestID()
	}

	ctx := contextWithRequestID(r.Context(), requestID)
	r = r.WithContext(ctx)
	w.Header().Set("X-Request-Id", requestID)

	recorder := &statusRecorder{ResponseWriter: w}
	start := time.Now()
	defer func() {
		if recovered := recover(); recovered != nil {
			s.logger.ErrorContext(ctx, "http panic recovered", "service", "knowledge", "request_id", requestID, "operation", "http_request")
			writeAppError(recorder, r, service.NewError(service.CodeInternal, "internal server error", nil))
		}
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		if status >= http.StatusInternalServerError {
			s.logger.ErrorContext(ctx, "http request failed", "service", "knowledge", "request_id", requestID, "method", r.Method, "path", r.URL.Path, "status", status, "duration_ms", time.Since(start).Milliseconds())
		}
	}()

	s.mux.ServeHTTP(recorder, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "knowledge",
		"status":  "ok",
	}, requestIDFromContext(r.Context()))
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service":     "knowledge",
		"status":      "ready",
		"version":     s.serviceVersion,
		"environment": s.environment,
	}, requestIDFromContext(r.Context()))
}

func (s *Server) handleListKnowledgeBases(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	page, ok := parsePage(w, r)
	if !ok {
		return
	}
	list, err := s.knowledge.ListKnowledgeBases(r.Context(), reqCtx, service.ListKnowledgeBasesInput{Page: page})
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writePageJSON(w, http.StatusOK, knowledgeBasesFromDomain(list.Items), list.Page, requestIDFromContext(r.Context()))
}

func (s *Server) handleCreateKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	var payload createKnowledgeBaseRequest
	if !decodeJSONBody(w, r, &payload) {
		return
	}
	input := service.CreateKnowledgeBaseInput{
		Name:        payload.Name,
		Description: payload.Description,
		DocType:     payload.DocType,
	}
	if payload.ID != nil {
		input.ID = *payload.ID
	}
	if payload.ChunkStrategy != nil {
		input.ChunkStrategy = cloneRaw(*payload.ChunkStrategy)
	}
	if payload.RetrievalStrategy != nil {
		input.RetrievalStrategy = cloneRaw(*payload.RetrievalStrategy)
	}
	kb, err := s.knowledge.CreateKnowledgeBase(r.Context(), reqCtx, input)
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, knowledgeBaseFromDomain(kb), requestIDFromContext(r.Context()))
}

func (s *Server) handleGetKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	kb, err := s.knowledge.GetKnowledgeBase(r.Context(), reqCtx, r.PathValue("knowledgeBaseId"))
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, knowledgeBaseFromDomain(kb), requestIDFromContext(r.Context()))
}

func (s *Server) handleUpdateKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	var payload updateKnowledgeBaseRequest
	if !decodeJSONBody(w, r, &payload) {
		return
	}
	input := service.UpdateKnowledgeBaseInput{
		ID:          r.PathValue("knowledgeBaseId"),
		Name:        payload.Name,
		Description: payload.Description,
		DocType:     payload.DocType,
	}
	if payload.ChunkStrategy != nil {
		value := json.RawMessage(cloneRaw(*payload.ChunkStrategy))
		input.ChunkStrategy = &value
	}
	if payload.RetrievalStrategy != nil {
		value := json.RawMessage(cloneRaw(*payload.RetrievalStrategy))
		input.RetrievalStrategy = &value
	}
	kb, err := s.knowledge.UpdateKnowledgeBase(r.Context(), reqCtx, input)
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, knowledgeBaseFromDomain(kb), requestIDFromContext(r.Context()))
}

func (s *Server) handleDeleteKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	if err := s.knowledge.DeleteKnowledgeBase(r.Context(), reqCtx, r.PathValue("knowledgeBaseId")); err != nil {
		writeAppError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	page, ok := parsePage(w, r)
	if !ok {
		return
	}
	var status *service.DocumentStatus
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		value := service.DocumentStatus(raw)
		status = &value
	}
	list, err := s.knowledge.ListDocuments(r.Context(), reqCtx, service.ListDocumentsInput{
		KnowledgeBaseID: r.PathValue("knowledgeBaseId"),
		Status:          status,
		Page:            page,
	})
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writePageJSON(w, http.StatusOK, documentsFromDomain(list.Items), list.Page, requestIDFromContext(r.Context()))
}

func (s *Server) handleUploadDocument(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	file, header, tags, ok := s.parseDocumentUpload(w, r)
	if !ok {
		return
	}
	defer file.Close()

	contentType := ""
	if header != nil {
		contentType = strings.TrimSpace(header.Header.Get("Content-Type"))
	}
	doc, err := s.knowledge.UploadDocument(r.Context(), reqCtx, service.UploadDocumentInput{
		KnowledgeBaseID: r.PathValue("knowledgeBaseId"),
		File: service.UploadedFile{
			Filename:    header.Filename,
			ContentType: contentType,
			SizeBytes:   header.Size,
			Content:     file,
		},
		Tags: tags,
	})
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, documentFromDomain(doc), requestIDFromContext(r.Context()))
}

func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	doc, err := s.knowledge.GetDocument(r.Context(), reqCtx, r.PathValue("documentId"))
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, documentFromDomain(doc), requestIDFromContext(r.Context()))
}

func (s *Server) handleListDocumentChunks(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	page, ok := parsePage(w, r)
	if !ok {
		return
	}
	list, err := s.knowledge.ListChunks(r.Context(), reqCtx, service.ListChunksInput{
		DocumentID: r.PathValue("documentId"),
		Page:       page,
	})
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writePageJSON(w, http.StatusOK, documentChunksFromDomain(list.Items), list.Page, requestIDFromContext(r.Context()))
}

func (s *Server) handleGetDocumentContent(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	content, err := s.knowledge.GetDocumentContent(r.Context(), reqCtx, r.PathValue("documentId"))
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	defer content.Body.Close()

	w.Header().Set("Content-Type", content.ContentType)
	if content.SizeBytes > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(content.SizeBytes, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, content.Body)
}

func (s *Server) handleCreateKnowledgeQuery(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	var payload knowledgeQueryRequest
	if !decodeJSONBody(w, r, &payload) {
		return
	}
	query, err := s.knowledge.CreateKnowledgeQuery(r.Context(), reqCtx, service.KnowledgeQueryInput{
		Query:            payload.Query,
		KnowledgeBaseIDs: payload.KnowledgeBaseIDs,
		TopK:             payload.TopK,
		ScoreThreshold:   payload.ScoreThreshold,
		Tags:             payload.Tags,
		MetadataFilter:   payload.MetadataFilter,
		Rerank:           payload.Rerank,
		RerankTopN:       payload.RerankTopN,
	})
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, knowledgeQueryFromDomain(query), requestIDFromContext(r.Context()))
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeAppError(w, r, service.NotFoundError("route not found"))
}

func (s *Server) gatewayContext(w http.ResponseWriter, r *http.Request) (service.RequestContext, bool) {
	reqCtx := service.RequestContext{
		RequestID:      requestIDFromContext(r.Context()),
		UserID:         strings.TrimSpace(r.Header.Get("X-User-Id")),
		CallerService:  strings.TrimSpace(r.Header.Get("X-Caller-Service")),
		ServiceToken:   strings.TrimSpace(r.Header.Get("X-Service-Token")),
		Roles:          splitCSV(r.Header.Get("X-User-Roles")),
		Permissions:    splitCSV(r.Header.Get("X-User-Permissions")),
		ForwardedFor:   strings.TrimSpace(r.Header.Get("X-Forwarded-For")),
		ForwardedProto: strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")),
	}
	if reqCtx.UserID == "" {
		writeAppError(w, r, service.UnauthorizedError())
		return service.RequestContext{}, false
	}
	return reqCtx, true
}

func (s *Server) parseDocumentUpload(w http.ResponseWriter, r *http.Request) (multipart.File, *multipart.FileHeader, []string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUploadBytes)
	if err := r.ParseMultipartForm(s.maxUploadBytes); err != nil {
		fieldMessage := "multipart form is invalid"
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			fieldMessage = "exceeds maximum upload size"
		}
		writeAppError(w, r, service.ValidationError("request validation failed", map[string]string{"file": fieldMessage}))
		return nil, nil, nil, false
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeAppError(w, r, service.ValidationError("request validation failed", map[string]string{"file": "is required"}))
		return nil, nil, nil, false
	}
	if header == nil || header.Size == 0 {
		_ = file.Close()
		writeAppError(w, r, service.ValidationError("request validation failed", map[string]string{"file": "must not be empty"}))
		return nil, nil, nil, false
	}

	tags, err := parseUploadTags(r)
	if err != nil {
		_ = file.Close()
		writeAppError(w, r, service.ValidationError("request validation failed", map[string]string{"tags": "must be repeated strings or a JSON string array"}))
		return nil, nil, nil, false
	}
	return file, header, tags, true
}

func parseUploadTags(r *http.Request) ([]string, error) {
	if r.MultipartForm == nil {
		return nil, nil
	}
	values := r.MultipartForm.Value["tags"]
	tags := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			var parsed []string
			if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
				return nil, err
			}
			tags = append(tags, parsed...)
			continue
		}
		tags = append(tags, trimmed)
	}
	return tags, nil
}

func parsePage(w http.ResponseWriter, r *http.Request) (service.PageInput, bool) {
	var fields map[string]string
	page := parsePositiveIntParam(r, "page", &fields)
	pageSize := parsePositiveIntParam(r, "pageSize", &fields)
	if len(fields) > 0 {
		writeAppError(w, r, service.ValidationError("request validation failed", fields))
		return service.PageInput{}, false
	}
	return service.PageInput{Page: page, PageSize: pageSize}, true
}

func parsePositiveIntParam(r *http.Request, name string, fields *map[string]string) int {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		if *fields == nil {
			*fields = map[string]string{}
		}
		(*fields)[name] = "must be an integer"
		return 0
	}
	return value
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeAppError(w, r, service.ValidationError("request validation failed", map[string]string{"body": "must be a valid JSON object"}))
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeAppError(w, r, service.ValidationError("request validation failed", map[string]string{"body": "must contain only one JSON object"}))
		return false
	}
	return true
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func cloneRaw(value []byte) []byte {
	if value == nil {
		return nil
	}
	return append([]byte(nil), value...)
}

func newRequestID() string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "req_" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	}
	return "req_" + hex.EncodeToString(buf[:])
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
