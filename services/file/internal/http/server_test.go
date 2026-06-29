package httpapi_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	filehttp "github.com/Sakayori-Iroha-168/Software_Teamwork/services/file/internal/http"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/file/internal/platform/storage"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/file/internal/repository"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/file/internal/service"
)

func TestHealthReturnsEnvelope(t *testing.T) {
	server := newHTTPTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-Id", "req_health")
	res := httptest.NewRecorder()

	server.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d", res.Code)
	}
	var body successBody
	decodeJSON(t, res.Body, &body)
	if body.RequestID != "req_health" {
		t.Fatalf("requestId = %q", body.RequestID)
	}
}

func TestFileRoutesCreateGetContentAndDelete(t *testing.T) {
	server := newHTTPTestServer(t)
	content := "content"
	checksum := sha256Hex(content)
	create := newMultipartUploadRequest(t, "/internal/v1/files", "..\\policy.pdf", "application/pdf", content, nil, checksum)
	setInternalCaller(create)
	res := httptest.NewRecorder()
	server.ServeHTTP(res, create)
	if res.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", res.Code, res.Body.String())
	}

	bodyText := res.Body.String()
	assertNotContainsSensitiveStorage(t, bodyText)
	var created fileResponseBody
	decodeJSON(t, strings.NewReader(bodyText), &created)
	if created.RequestID != "req_test" {
		t.Fatalf("requestId = %q", created.RequestID)
	}
	if created.Data.ID == "" || !strings.HasPrefix(created.Data.ID, "file_") {
		t.Fatalf("file id = %q", created.Data.ID)
	}
	if created.Data.Filename != "policy.pdf" {
		t.Fatalf("filename = %q", created.Data.Filename)
	}
	if created.Data.ContentType != "application/pdf" || created.Data.SizeBytes != int64(len(content)) || created.Data.ChecksumSHA256 == nil || *created.Data.ChecksumSHA256 != checksum {
		t.Fatalf("file metadata = %+v", created.Data)
	}
	if created.Data.DeletedAt != nil {
		t.Fatalf("deletedAt = %v", *created.Data.DeletedAt)
	}

	getReq := internalRequest(http.MethodGet, "/internal/v1/files/"+created.Data.ID, nil)
	getRes := httptest.NewRecorder()
	server.ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRes.Code, getRes.Body.String())
	}
	assertNotContainsSensitiveStorage(t, getRes.Body.String())

	contentReq := internalRequest(http.MethodGet, "/internal/v1/files/"+created.Data.ID+"/content", nil)
	contentRes := httptest.NewRecorder()
	server.ServeHTTP(contentRes, contentReq)
	if contentRes.Code != http.StatusOK {
		t.Fatalf("content status = %d, body = %s", contentRes.Code, contentRes.Body.String())
	}
	if got := contentRes.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("content type = %q", got)
	}
	if got := contentRes.Header().Get("Content-Length"); got != "7" {
		t.Fatalf("content length = %q", got)
	}
	contentDisposition := contentRes.Header().Get("Content-Disposition")
	if !strings.Contains(contentDisposition, "attachment") || strings.ContainsAny(contentDisposition, "\r\n") || strings.Contains(contentDisposition, "..") {
		t.Fatalf("content disposition = %q", contentDisposition)
	}
	if got := contentRes.Body.String(); got != content {
		t.Fatalf("content body = %q", got)
	}

	deleteReq := internalRequest(http.MethodDelete, "/internal/v1/files/"+created.Data.ID, nil)
	deleteRes := httptest.NewRecorder()
	server.ServeHTTP(deleteRes, deleteReq)
	if deleteRes.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleteRes.Code, deleteRes.Body.String())
	}

	getAfterDelete := internalRequest(http.MethodGet, "/internal/v1/files/"+created.Data.ID, nil)
	getAfterDeleteRes := httptest.NewRecorder()
	server.ServeHTTP(getAfterDeleteRes, getAfterDelete)
	if getAfterDeleteRes.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d", getAfterDeleteRes.Code)
	}

	contentAfterDelete := internalRequest(http.MethodGet, "/internal/v1/files/"+created.Data.ID+"/content", nil)
	contentAfterDeleteRes := httptest.NewRecorder()
	server.ServeHTTP(contentAfterDeleteRes, contentAfterDelete)
	if contentAfterDeleteRes.Code != http.StatusNotFound {
		t.Fatalf("content after delete status = %d", contentAfterDeleteRes.Code)
	}
}

func TestFileCreateRejectsChecksumMismatch(t *testing.T) {
	server := newHTTPTestServer(t)
	req := newMultipartUploadRequest(t, "/internal/v1/files", "policy.pdf", "application/pdf", "content", nil, strings.Repeat("0", 64))
	setInternalCaller(req)
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var errBody errorResponseBody
	decodeJSON(t, res.Body, &errBody)
	if errBody.Error.Code != "validation_error" || errBody.Error.Fields["checksumSha256"] == "" {
		t.Fatalf("error body = %+v", errBody)
	}
}

func TestFileCreateRejectsInvalidChecksum(t *testing.T) {
	server := newHTTPTestServer(t)
	req := newMultipartUploadRequest(t, "/internal/v1/files", "policy.pdf", "application/pdf", "content", nil, "not-a-checksum")
	setInternalCaller(req)
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var errBody errorResponseBody
	decodeJSON(t, res.Body, &errBody)
	if errBody.Error.Fields["checksumSha256"] == "" {
		t.Fatalf("error body = %+v", errBody)
	}
}

func TestFileCreateRejectsEmptyFile(t *testing.T) {
	server := newHTTPTestServer(t)
	req := newMultipartUploadRequest(t, "/internal/v1/files", "empty.txt", "text/plain", "", nil, "")
	setInternalCaller(req)
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var errBody errorResponseBody
	decodeJSON(t, res.Body, &errBody)
	if errBody.Error.Fields["file"] == "" {
		t.Fatalf("error body = %+v", errBody)
	}
}

func TestFileCreateRejectsOversizedMultipart(t *testing.T) {
	server := newLimitedHTTPTestServer(t, 4)
	req := newMultipartUploadRequest(t, "/internal/v1/files", "large.txt", "text/plain", "too large", nil, "")
	setInternalCaller(req)
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var errBody errorResponseBody
	decodeJSON(t, res.Body, &errBody)
	if !strings.Contains(errBody.Error.Fields["file"], "exceeds") {
		t.Fatalf("error body = %+v", errBody)
	}
}

func TestFileCreateRejectsMalformedMultipart(t *testing.T) {
	server := newHTTPTestServer(t)
	req := internalRequest(http.MethodPost, "/internal/v1/files", strings.NewReader("not multipart"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=missing")
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var errBody errorResponseBody
	decodeJSON(t, res.Body, &errBody)
	if errBody.Error.Code != "validation_error" || errBody.Error.Fields["file"] == "" {
		t.Fatalf("error body = %+v", errBody)
	}
}

func TestFileCreateRequiresInternalCaller(t *testing.T) {
	server := newHTTPTestServer(t)
	req := unauthenticatedMultipartUploadRequest(t, "/internal/v1/files", "policy.pdf", "application/pdf", "content")
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestUploadGetPatchAndContent(t *testing.T) {
	server := newHTTPTestServer(t)

	upload := newMultipartUploadRequest(t, "/internal/v1/knowledge-bases/kb_123/documents", "policy.pdf", "application/pdf", "content", []string{"policy", "inspection"}, "")
	res := httptest.NewRecorder()
	server.ServeHTTP(res, upload)
	if res.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, body = %s", res.Code, res.Body.String())
	}

	var uploadBody documentResponseBody
	decodeJSON(t, res.Body, &uploadBody)
	if uploadBody.RequestID != "req_test" {
		t.Fatalf("upload requestId = %q", uploadBody.RequestID)
	}
	if uploadBody.Data.ID == "" || uploadBody.Data.KnowledgeBaseID != "kb_123" || uploadBody.Data.Status != "uploaded" {
		t.Fatalf("upload data = %+v", uploadBody.Data)
	}
	if uploadBody.Data.Name != "policy.pdf" {
		t.Fatalf("upload name = %q", uploadBody.Data.Name)
	}
	if uploadBody.Data.ContentType != "application/pdf" || uploadBody.Data.SizeBytes != int64(len("content")) {
		t.Fatalf("upload file metadata = %+v", uploadBody.Data)
	}
	if got, want := strings.Join(uploadBody.Data.Tags, ","), "policy,inspection"; got != want {
		t.Fatalf("upload tags = %q", got)
	}

	getReq := authorizedRequest(http.MethodGet, "/internal/v1/documents/"+uploadBody.Data.ID, nil)
	getRes := httptest.NewRecorder()
	server.ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRes.Code, getRes.Body.String())
	}

	patchReq := authorizedRequest(http.MethodPatch, "/internal/v1/documents/"+uploadBody.Data.ID, strings.NewReader(`{"tags":["updated"]}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchRes := httptest.NewRecorder()
	server.ServeHTTP(patchRes, patchReq)
	if patchRes.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", patchRes.Code, patchRes.Body.String())
	}
	var patchBody documentResponseBody
	decodeJSON(t, patchRes.Body, &patchBody)
	if got, want := strings.Join(patchBody.Data.Tags, ","), "updated"; got != want {
		t.Fatalf("patch tags = %q", got)
	}

	contentReq := authorizedRequest(http.MethodGet, "/internal/v1/documents/"+uploadBody.Data.ID+"/content", nil)
	contentRes := httptest.NewRecorder()
	server.ServeHTTP(contentRes, contentReq)
	if contentRes.Code != http.StatusOK {
		t.Fatalf("content status = %d, body = %s", contentRes.Code, contentRes.Body.String())
	}
	if got := contentRes.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("content type = %q", got)
	}
	if got := contentRes.Body.String(); got != "content" {
		t.Fatalf("content body = %q", got)
	}
}

func TestUploadRequiresFile(t *testing.T) {
	server := newHTTPTestServer(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("tags", "policy"); err != nil {
		t.Fatalf("WriteField() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	req := authorizedRequest(http.MethodPost, "/internal/v1/knowledge-bases/kb_123/documents", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var errBody errorResponseBody
	decodeJSON(t, res.Body, &errBody)
	if errBody.Error.Code != "validation_error" || errBody.Error.RequestID != "req_test" {
		t.Fatalf("error body = %+v", errBody)
	}
}

func TestBusinessRoutesRequireGatewayUser(t *testing.T) {
	server := newHTTPTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/internal/v1/documents/doc_123", nil)
	req.Header.Set("X-Request-Id", "req_no_user")
	res := httptest.NewRecorder()

	server.ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", res.Code)
	}
	var errBody errorResponseBody
	decodeJSON(t, res.Body, &errBody)
	if errBody.Error.Code != "unauthorized" {
		t.Fatalf("error code = %q", errBody.Error.Code)
	}
}

func TestBusinessRoutesRequirePermission(t *testing.T) {
	server := newHTTPTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/internal/v1/documents/doc_123", nil)
	req.Header.Set("X-Request-Id", "req_no_permission")
	req.Header.Set("X-User-Id", "usr_123")
	res := httptest.NewRecorder()

	server.ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d", res.Code)
	}
	var errBody errorResponseBody
	decodeJSON(t, res.Body, &errBody)
	if errBody.Error.Code != "forbidden" {
		t.Fatalf("error code = %q", errBody.Error.Code)
	}
}

func TestDeleteHidesLaterContentReads(t *testing.T) {
	server := newHTTPTestServer(t)
	upload := newMultipartUploadRequest(t, "/internal/v1/knowledge-bases/kb_123/documents", "policy.pdf", "application/pdf", "content", nil, "")
	uploadRes := httptest.NewRecorder()
	server.ServeHTTP(uploadRes, upload)
	if uploadRes.Code != http.StatusCreated {
		t.Fatalf("upload status = %d", uploadRes.Code)
	}
	var uploadBody documentResponseBody
	decodeJSON(t, uploadRes.Body, &uploadBody)

	deleteReq := authorizedRequest(http.MethodDelete, "/internal/v1/documents/"+uploadBody.Data.ID, nil)
	deleteRes := httptest.NewRecorder()
	server.ServeHTTP(deleteRes, deleteReq)
	if deleteRes.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", deleteRes.Code)
	}

	contentReq := authorizedRequest(http.MethodGet, "/internal/v1/documents/"+uploadBody.Data.ID+"/content", nil)
	contentRes := httptest.NewRecorder()
	server.ServeHTTP(contentRes, contentReq)
	if contentRes.Code != http.StatusNotFound {
		t.Fatalf("content status = %d", contentRes.Code)
	}
}

func newHTTPTestServer(t *testing.T) http.Handler {
	t.Helper()
	return newLimitedHTTPTestServer(t, 1024*1024)
}

func newLimitedHTTPTestServer(t *testing.T, maxUploadBytes int64) http.Handler {
	t.Helper()
	repo := repository.NewMemoryRepository()
	store := storage.NewMemoryStore()
	documents := service.New(repo, store)
	return filehttp.NewServer(documents, filehttp.Config{MaxUploadBytes: maxUploadBytes})
}

func newMultipartUploadRequest(t *testing.T, target string, filename string, contentType string, content string, tags []string, checksum string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": filename}))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	partHeader.Set("Content-Type", contentType)
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}
	if _, err := io.Copy(part, strings.NewReader(content)); err != nil {
		t.Fatalf("Copy() error = %v", err)
	}
	if checksum != "" {
		if err := writer.WriteField("checksumSha256", checksum); err != nil {
			t.Fatalf("WriteField(checksumSha256) error = %v", err)
		}
	}
	for _, tag := range tags {
		if err := writer.WriteField("tags", tag); err != nil {
			t.Fatalf("WriteField() error = %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	req := authorizedRequest(http.MethodPost, target, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func unauthenticatedMultipartUploadRequest(t *testing.T, target string, filename string, contentType string, content string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": filename}))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	partHeader.Set("Content-Type", contentType)
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		t.Fatalf("CreatePart() error = %v", err)
	}
	if _, err := io.Copy(part, strings.NewReader(content)); err != nil {
		t.Fatalf("Copy() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, target, &body)
	req.Header.Set("X-Request-Id", "req_test")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}
func authorizedRequest(method string, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	req.Header.Set("X-Request-Id", "req_test")
	req.Header.Set("X-User-Id", "usr_123")
	req.Header.Set("X-User-Roles", "admin")
	req.Header.Set("X-User-Permissions", "document:read,document:upload,document:update,document:delete")
	return req
}

func internalRequest(method string, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	req.Header.Set("X-Request-Id", "req_test")
	setInternalCaller(req)
	return req
}

func setInternalCaller(req *http.Request) {
	req.Header.Set("X-Caller-Service", "knowledge")
	req.Header.Set("X-Service-Token", "test-token")
}

func decodeJSON(t *testing.T, reader io.Reader, target any) {
	t.Helper()
	if err := json.NewDecoder(reader).Decode(target); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func assertNotContainsSensitiveStorage(t *testing.T, body string) {
	t.Helper()
	for _, forbidden := range []string{"objectKey", "storageObjectKey", "storageBucket", "bucket", "files/", "minio", "accessKey", "secretKey"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
}

type successBody struct {
	Data      map[string]string `json:"data"`
	RequestID string            `json:"requestId"`
}

type fileResponseBody struct {
	Data struct {
		ID             string  `json:"id"`
		Filename       string  `json:"filename"`
		ContentType    string  `json:"contentType"`
		SizeBytes      int64   `json:"sizeBytes"`
		ChecksumSHA256 *string `json:"checksumSha256"`
		CreatedAt      string  `json:"createdAt"`
		DeletedAt      *string `json:"deletedAt"`
	} `json:"data"`
	RequestID string `json:"requestId"`
}

type documentResponseBody struct {
	Data struct {
		ID              string   `json:"id"`
		KnowledgeBaseID string   `json:"knowledgeBaseId"`
		Name            string   `json:"name"`
		Status          string   `json:"status"`
		Tags            []string `json:"tags"`
		CreatedAt       string   `json:"createdAt"`
		ContentType     string   `json:"contentType"`
		SizeBytes       int64    `json:"sizeBytes"`
	} `json:"data"`
	RequestID string `json:"requestId"`
}

type errorResponseBody struct {
	Error struct {
		Code      string            `json:"code"`
		Message   string            `json:"message"`
		RequestID string            `json:"requestId"`
		Fields    map[string]string `json:"fields"`
	} `json:"error"`
}
