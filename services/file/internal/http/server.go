package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/file/internal/service"
)

const defaultMaxUploadBytes = int64(32 << 20)

type Config struct {
	MaxUploadBytes int64
	Logger         *slog.Logger
}

type Server struct {
	documents      *service.Service
	maxUploadBytes int64
	logger         *slog.Logger
	mux            *http.ServeMux
}

func NewServer(documents *service.Service, cfg Config) *Server {
	if cfg.MaxUploadBytes <= 0 {
		cfg.MaxUploadBytes = defaultMaxUploadBytes
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &Server{
		documents:      documents,
		maxUploadBytes: cfg.MaxUploadBytes,
		logger:         cfg.Logger,
		mux:            http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /readyz", s.handleReady)
	s.mux.HandleFunc("POST /internal/v1/files", s.handleCreateFile)
	s.mux.HandleFunc("GET /internal/v1/files/{fileId}", s.handleGetFile)
	s.mux.HandleFunc("DELETE /internal/v1/files/{fileId}", s.handleDeleteFile)
	s.mux.HandleFunc("GET /internal/v1/files/{fileId}/content", s.handleGetFileContent)
	s.mux.HandleFunc("POST /internal/v1/knowledge-bases/{knowledgeBaseId}/documents", s.handleUploadDocument)
	s.mux.HandleFunc("GET /internal/v1/documents/{documentId}", s.handleGetDocument)
	s.mux.HandleFunc("PATCH /internal/v1/documents/{documentId}", s.handleUpdateDocument)
	s.mux.HandleFunc("DELETE /internal/v1/documents/{documentId}", s.handleDeleteDocument)
	s.mux.HandleFunc("GET /internal/v1/documents/{documentId}/content", s.handleGetDocumentContent)
	s.mux.HandleFunc("/", s.handleNotFound)
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
			s.logger.ErrorContext(ctx, "http panic recovered", "service", "file", "request_id", requestID, "operation", "http_request")
			writeAppError(recorder, r, service.NewError(service.CodeInternal, "internal server error", nil))
		}
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		if status >= http.StatusInternalServerError {
			s.logger.ErrorContext(ctx, "http request failed", "service", "file", "request_id", requestID, "method", r.Method, "path", r.URL.Path, "status", status, "duration_ms", time.Since(start).Milliseconds())
		}
	}()

	s.mux.ServeHTTP(recorder, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"service": "file", "status": "ok"}, requestIDFromContext(r.Context()))
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"service": "file", "status": "ready"}, requestIDFromContext(r.Context()))
}

func (s *Server) handleCreateFile(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.internalContext(w, r)
	if !ok {
		return
	}

	file, header, checksum, ok := s.parseMultipartFile(w, r)
	if !ok {
		return
	}
	defer file.Close()

	created, err := s.documents.CreateFile(r.Context(), reqCtx, service.CreateFileInput{
		FileName:       header.Filename,
		ContentType:    header.Header.Get("Content-Type"),
		SizeBytes:      header.Size,
		ChecksumSHA256: checksum,
		Content:        file,
	})
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, fileObjectFromDomain(created), requestIDFromContext(r.Context()))
}

func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.internalContext(w, r)
	if !ok {
		return
	}
	file, err := s.documents.GetFile(r.Context(), reqCtx, r.PathValue("fileId"))
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, fileObjectFromDomain(file), requestIDFromContext(r.Context()))
}

func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.internalContext(w, r)
	if !ok {
		return
	}
	if err := s.documents.DeleteFile(r.Context(), reqCtx, r.PathValue("fileId")); err != nil {
		writeAppError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetFileContent(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.internalContext(w, r)
	if !ok {
		return
	}
	content, err := s.documents.GetFileContent(r.Context(), reqCtx, r.PathValue("fileId"))
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	defer content.Body.Close()
	writeContent(w, content.ContentType, content.File.Filename, content.SizeBytes, content.Body)
}

func (s *Server) handleUploadDocument(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}

	file, header, _, ok := s.parseMultipartFile(w, r)
	if !ok {
		return
	}
	defer file.Close()

	var tags []string
	if r.MultipartForm != nil {
		tags = append(tags, r.MultipartForm.Value["tags"]...)
	}

	doc, err := s.documents.UploadDocument(r.Context(), reqCtx, service.UploadDocumentInput{
		KnowledgeBaseID: r.PathValue("knowledgeBaseId"),
		FileName:        header.Filename,
		ContentType:     header.Header.Get("Content-Type"),
		SizeBytes:       header.Size,
		Tags:            tags,
		Content:         file,
	})
	if err != nil {
		writeAppError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, documentSummaryFromDomain(doc), requestIDFromContext(r.Context()))
}

func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	doc, err := s.documents.GetDocument(r.Context(), reqCtx, r.PathValue("documentId"))
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, documentSummaryFromDomain(doc), requestIDFromContext(r.Context()))
}

func (s *Server) handleUpdateDocument(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()

	var payload struct {
		Tags []string `json:"tags"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeAppError(w, r, service.ValidationError("request validation failed", map[string]string{"body": "must be a valid JSON object"}))
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeAppError(w, r, service.ValidationError("request validation failed", map[string]string{"body": "must contain only one JSON object"}))
		return
	}

	doc, err := s.documents.UpdateDocument(r.Context(), reqCtx, service.UpdateDocumentInput{
		DocumentID: r.PathValue("documentId"),
		Tags:       payload.Tags,
	})
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, documentSummaryFromDomain(doc), requestIDFromContext(r.Context()))
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	if err := s.documents.DeleteDocument(r.Context(), reqCtx, r.PathValue("documentId")); err != nil {
		writeAppError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetDocumentContent(w http.ResponseWriter, r *http.Request) {
	reqCtx, ok := s.gatewayContext(w, r)
	if !ok {
		return
	}
	content, err := s.documents.GetDocumentContent(r.Context(), reqCtx, r.PathValue("documentId"))
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	defer content.Body.Close()
	writeContent(w, content.ContentType, content.Document.Name, content.SizeBytes, content.Body)
}

func (s *Server) parseMultipartFile(w http.ResponseWriter, r *http.Request) (multipart.File, *multipart.FileHeader, string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUploadBytes)
	if err := r.ParseMultipartForm(s.maxUploadBytes); err != nil {
		fieldMessage := "multipart form is invalid"
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			fieldMessage = "exceeds maximum upload size"
		}
		writeAppError(w, r, service.ValidationError("request validation failed", map[string]string{"file": fieldMessage}))
		return nil, nil, "", false
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeAppError(w, r, service.ValidationError("request validation failed", map[string]string{"file": "is required"}))
		return nil, nil, "", false
	}
	checksum := ""
	if r.MultipartForm != nil {
		checksum = strings.TrimSpace(firstValue(r.MultipartForm.Value["checksumSha256"]))
	}
	return file, header, checksum, true
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeAppError(w, r, service.NotFoundError("route not found"))
}

func (s *Server) gatewayContext(w http.ResponseWriter, r *http.Request) (service.RequestContext, bool) {
	reqCtx := requestContextFromHeaders(r)
	if strings.TrimSpace(reqCtx.UserID) == "" {
		writeAppError(w, r, service.UnauthorizedError())
		return service.RequestContext{}, false
	}
	return reqCtx, true
}

func (s *Server) internalContext(w http.ResponseWriter, r *http.Request) (service.RequestContext, bool) {
	reqCtx := requestContextFromHeaders(r)
	if strings.TrimSpace(reqCtx.CallerService) == "" && strings.TrimSpace(reqCtx.UserID) == "" {
		writeAppError(w, r, service.UnauthorizedError())
		return service.RequestContext{}, false
	}
	return reqCtx, true
}

func requestContextFromHeaders(r *http.Request) service.RequestContext {
	return service.RequestContext{
		RequestID:      requestIDFromContext(r.Context()),
		UserID:         strings.TrimSpace(r.Header.Get("X-User-Id")),
		CallerService:  strings.TrimSpace(r.Header.Get("X-Caller-Service")),
		ServiceToken:   strings.TrimSpace(r.Header.Get("X-Service-Token")),
		Roles:          splitCSV(r.Header.Get("X-User-Roles")),
		Permissions:    splitCSV(r.Header.Get("X-User-Permissions")),
		ForwardedFor:   strings.TrimSpace(r.Header.Get("X-Forwarded-For")),
		ForwardedProto: strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")),
	}
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

func writeContent(w http.ResponseWriter, contentType string, filename string, sizeBytes int64, body io.Reader) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": safeFilename(filename)}))
	if sizeBytes >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(sizeBytes, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

func firstValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func safeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == 0 {
			return -1
		}
		return r
	}, strings.TrimSpace(name))
	if name == "" {
		return "file"
	}
	return name
}

func newRequestID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "req_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return "req_" + hex.EncodeToString(bytes)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(body)
}
