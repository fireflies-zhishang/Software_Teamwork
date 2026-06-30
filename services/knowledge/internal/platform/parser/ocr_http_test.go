package parser_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/knowledge/internal/platform/parser"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/knowledge/internal/service"
)

func TestServiceClientPostsDocumentAndContextHeaders(t *testing.T) {
	var captured *http.Request
	var payload struct {
		DocumentName string `json:"documentName"`
		ContentType  string `json:"contentType"`
		SizeBytes    *int64 `json:"sizeBytes"`
		DataBase64   string `json:"dataBase64"`
	}
	client, err := parser.NewServiceClient(parser.ServiceClientConfig{
		BaseURL:      "https://parser.internal",
		ServiceToken: "secret-token",
		Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			captured = req.Clone(req.Context())
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode request body error = %v", err)
			}
			return jsonResponse(http.StatusOK, `{"data":{"content":"Breaker OCR","title":"Breaker","backend":"ppstructurev3","pages":[{"pageNumber":1,"content":" Page one ","parseStrategy":"ocr","textLayerStatus":"broken","ocrConfidence":0.91,"dpi":180,"warnings":["low_text_quality",""]},{"pageNumber":0,"content":"ignored"},{"pageNumber":2,"content":""}]},"requestId":"req_123"}`), nil
		})},
	})
	if err != nil {
		t.Fatalf("NewServiceClient() error = %v", err)
	}

	result, err := client.Parse(context.Background(), service.ParseInput{
		Name:        "scan.pdf",
		ContentType: "application/pdf",
		Body:        bytes.NewReader([]byte("%PDF")),
		SizeBytes:   4,
		RequestID:   "req_123",
		UserID:      "usr_123",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.Content != "Breaker OCR" {
		t.Fatalf("result = %+v", result)
	}
	if result.Title != "Breaker" || result.Backend != "ppstructurev3" {
		t.Fatalf("result metadata = %+v", result)
	}
	if len(result.Pages) != 1 || result.Pages[0].PageNumber != 1 || result.Pages[0].Content != "Page one" {
		t.Fatalf("pages = %+v", result.Pages)
	}
	if result.Pages[0].ParseStrategy != "ocr" || result.Pages[0].TextLayerStatus != "broken" {
		t.Fatalf("page metadata = %+v", result.Pages[0])
	}
	if result.Pages[0].OCRConfidence == nil || *result.Pages[0].OCRConfidence != 0.91 {
		t.Fatalf("page confidence = %+v", result.Pages[0].OCRConfidence)
	}
	if result.Pages[0].DPI == nil || *result.Pages[0].DPI != 180 {
		t.Fatalf("page dpi = %+v", result.Pages[0].DPI)
	}
	if len(result.Pages[0].Warnings) != 1 || result.Pages[0].Warnings[0] != "low_text_quality" {
		t.Fatalf("page warnings = %+v", result.Pages[0].Warnings)
	}
	if captured.URL.String() != "https://parser.internal/internal/v1/parsed-documents" {
		t.Fatalf("url = %s", captured.URL.String())
	}
	if captured.Header.Get("X-Request-Id") != "req_123" ||
		captured.Header.Get("X-Caller-Service") != "knowledge" ||
		captured.Header.Get("X-User-Id") != "usr_123" ||
		captured.Header.Get("X-Service-Token") != "secret-token" {
		t.Fatalf("headers = %+v", captured.Header)
	}
	if payload.DocumentName != "scan.pdf" || payload.ContentType != "application/pdf" || payload.SizeBytes == nil || *payload.SizeBytes != 4 {
		t.Fatalf("payload = %+v", payload)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload.DataBase64)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	if !bytes.Equal(decoded, []byte("%PDF")) {
		t.Fatalf("decoded payload = %q", string(decoded))
	}
}

func TestServiceClientParseDelegatesWholeDocumentToParserService(t *testing.T) {
	var capturedPath string
	var payload struct {
		DocumentName string `json:"documentName"`
		ContentType  string `json:"contentType"`
		SizeBytes    *int64 `json:"sizeBytes"`
		DataBase64   string `json:"dataBase64"`
	}
	client, err := parser.NewServiceClient(parser.ServiceClientConfig{
		BaseURL: "https://parser.internal",
		Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			capturedPath = req.URL.Path
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode request body error = %v", err)
			}
			return jsonResponse(http.StatusOK, `{"data":{"content":"Remote DOCX text","title":"Remote Title","backend":"paddleocr"},"requestId":"req_123"}`), nil
		})},
	})
	if err != nil {
		t.Fatalf("NewServiceClient() error = %v", err)
	}

	parsed, err := client.Parse(context.Background(), service.ParseInput{
		Name:        "manual.docx",
		ContentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		Body:        bytes.NewReader([]byte("not-a-zip-but-remote-parser-handles-it")),
		SizeBytes:   38,
		RequestID:   "req_123",
		UserID:      "usr_123",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if capturedPath != "/internal/v1/parsed-documents" {
		t.Fatalf("path = %s", capturedPath)
	}
	if payload.DocumentName != "manual.docx" || payload.SizeBytes == nil || *payload.SizeBytes != 38 {
		t.Fatalf("payload = %+v", payload)
	}
	if parsed.Content != "Remote DOCX text" || parsed.Title != "Remote Title" || parsed.Backend != "paddleocr" {
		t.Fatalf("parsed = %+v", parsed)
	}
	if len(parsed.Pages) != 0 {
		t.Fatalf("pages = %+v, want none for legacy parser response", parsed.Pages)
	}
}

func TestServiceClientParsesPageMetadataAndKeepsLegacyFields(t *testing.T) {
	client, err := parser.NewServiceClient(parser.ServiceClientConfig{
		BaseURL: "https://parser.internal",
		Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"data":{"content":"Parsed text","title":"Title","backend":"ppstructurev3","pages":[{"pageNumber":3,"content":" Page three ","parseStrategy":"ocr","textLayerStatus":"broken","ocrConfidence":1.5,"dpi":220,"warnings":["low_ocr_confidence"," "]}]},"requestId":"req_123"}`), nil
		})},
	})
	if err != nil {
		t.Fatalf("NewServiceClient() error = %v", err)
	}

	parsed, err := client.Parse(context.Background(), service.ParseInput{
		Name:        "scan.pdf",
		ContentType: "application/pdf",
		Body:        bytes.NewReader([]byte("%PDF")),
		SizeBytes:   4,
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if parsed.Content != "Parsed text" || parsed.Title != "Title" || parsed.Backend != "ppstructurev3" {
		t.Fatalf("parsed legacy fields = %+v", parsed)
	}
	if len(parsed.Pages) != 1 {
		t.Fatalf("pages = %+v", parsed.Pages)
	}
	page := parsed.Pages[0]
	if page.PageNumber != 3 || page.Content != "Page three" {
		t.Fatalf("page = %+v", page)
	}
	if page.ParseStrategy != "ocr" || page.TextLayerStatus != "broken" {
		t.Fatalf("page metadata = %+v", page)
	}
	if page.OCRConfidence == nil || *page.OCRConfidence != 1 {
		t.Fatalf("page confidence = %+v", page.OCRConfidence)
	}
	if page.DPI == nil || *page.DPI != 220 {
		t.Fatalf("page dpi = %+v", page.DPI)
	}
	if len(page.Warnings) != 1 || page.Warnings[0] != "low_ocr_confidence" {
		t.Fatalf("page warnings = %+v", page.Warnings)
	}
}

func TestServiceClientOmitsUnknownSourceSize(t *testing.T) {
	var payload map[string]any
	client, err := parser.NewServiceClient(parser.ServiceClientConfig{
		BaseURL: "https://parser.internal",
		Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode request body error = %v", err)
			}
			return jsonResponse(http.StatusOK, `{"data":{"content":"streamed text","backend":"document"},"requestId":"req_123"}`), nil
		})},
	})
	if err != nil {
		t.Fatalf("NewServiceClient() error = %v", err)
	}

	parsed, err := client.Parse(context.Background(), service.ParseInput{
		Name:        "streamed.md",
		ContentType: "text/markdown",
		Body:        bytes.NewReader([]byte("# streamed")),
		SizeBytes:   -1,
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsed.Content != "streamed text" {
		t.Fatalf("parsed = %+v", parsed)
	}
	if _, ok := payload["sizeBytes"]; ok {
		t.Fatalf("payload included sizeBytes for unknown source length: %+v", payload)
	}
}

func TestServiceClientRejectsOversizedUnknownSourceWithoutBufferingWholeBody(t *testing.T) {
	client, err := parser.NewServiceClient(parser.ServiceClientConfig{
		BaseURL: "https://parser.internal",
		Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			_, _ = io.Copy(io.Discard, req.Body)
			return jsonResponse(http.StatusOK, `{"data":{"content":"unexpected","backend":"document"},"requestId":"req_123"}`), nil
		})},
	})
	if err != nil {
		t.Fatalf("NewServiceClient() error = %v", err)
	}

	_, err = client.Parse(context.Background(), service.ParseInput{
		Name:        "large.pdf",
		ContentType: "application/pdf",
		Body:        io.LimitReader(zeroReader{}, (8<<20)+1),
		SizeBytes:   -1,
	})
	if err == nil {
		t.Fatal("Parse() error = nil, want oversized document error")
	}
	if containsAny(err.Error(), "large.pdf") {
		t.Fatalf("error leaked sensitive detail: %v", err)
	}
}

func TestServiceClientSanitizesFailure(t *testing.T) {
	client, err := parser.NewServiceClient(parser.ServiceClientConfig{
		BaseURL: "https://parser.internal/private-path",
		Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusBadGateway, `{"error":"secret document text"}`), nil
		})},
	})
	if err != nil {
		t.Fatalf("NewServiceClient() error = %v", err)
	}

	_, err = client.Parse(context.Background(), service.ParseInput{
		Name:        "scan.pdf",
		ContentType: "application/pdf",
		Body:        bytes.NewReader([]byte("secret document text")),
	})
	if err == nil {
		t.Fatal("Parse() error = nil, want error")
	}
	if containsAny(err.Error(), "secret", "private-path", "scan.pdf") {
		t.Fatalf("error leaked sensitive detail: %v", err)
	}
}

func TestServiceClientClassifiesParserValidationFailure(t *testing.T) {
	client, err := parser.NewServiceClient(parser.ServiceClientConfig{
		BaseURL: "https://parser.internal",
		Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusBadRequest, `{"error":{"code":"validation_error","message":"raw secret content"}}`), nil
		})},
	})
	if err != nil {
		t.Fatalf("NewServiceClient() error = %v", err)
	}

	_, err = client.Parse(context.Background(), service.ParseInput{
		Name:        "bad.pdf",
		ContentType: "application/pdf",
		Body:        bytes.NewReader([]byte("raw secret content")),
		SizeBytes:   18,
	})
	if err == nil {
		t.Fatal("Parse() error = nil, want validation error")
	}
	appErr, ok := service.Classify(err)
	if !ok || appErr.Code != service.CodeValidation {
		t.Fatalf("error = %#v, want validation error", err)
	}
	if containsAny(err.Error(), "secret", "bad.pdf") {
		t.Fatalf("error leaked sensitive detail: %v", err)
	}
}

func TestServiceClientRejectsEmptyParsedContent(t *testing.T) {
	client, err := parser.NewServiceClient(parser.ServiceClientConfig{
		BaseURL: "https://parser.internal",
		Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"data":{"content":"   ","backend":"ppstructurev3"},"requestId":"req_123"}`), nil
		})},
	})
	if err != nil {
		t.Fatalf("NewServiceClient() error = %v", err)
	}

	_, err = client.Parse(context.Background(), service.ParseInput{
		Name:        "empty.pdf",
		ContentType: "application/pdf",
		Body:        bytes.NewReader([]byte("%PDF")),
		SizeBytes:   4,
	})
	if err == nil {
		t.Fatal("Parse() error = nil, want empty document error")
	}
	if containsAny(err.Error(), "empty.pdf", "%PDF") {
		t.Fatalf("error leaked sensitive detail: %v", err)
	}
}

func TestServiceClientDoesNotFollowRedirectWithServiceToken(t *testing.T) {
	requests := []*http.Request{}
	client, err := parser.NewServiceClient(parser.ServiceClientConfig{
		BaseURL:      "https://parser.internal",
		ServiceToken: "secret-token",
		Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests = append(requests, req.Clone(req.Context()))
			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusFound,
					Header:     http.Header{"Location": []string{"https://evil.internal/steal"}},
					Body:       io.NopCloser(bytes.NewBufferString("redirect")),
					Request:    req,
				}, nil
			}
			return jsonResponse(http.StatusOK, `{"data":{"content":"redirected","backend":"paddleocr"},"requestId":"req_123"}`), nil
		})},
	})
	if err != nil {
		t.Fatalf("NewServiceClient() error = %v", err)
	}

	_, err = client.Parse(context.Background(), service.ParseInput{
		Name:        "scan.pdf",
		ContentType: "application/pdf",
		Body:        bytes.NewReader([]byte("%PDF")),
	})
	if err == nil {
		t.Fatal("Parse() error = nil, want redirect status error")
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want no redirected request", len(requests))
	}
	if containsAny(err.Error(), "secret", "evil", "scan.pdf") {
		t.Fatalf("error leaked sensitive detail: %v", err)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func containsAny(value string, forbidden ...string) bool {
	for _, item := range forbidden {
		if item != "" && bytes.Contains([]byte(value), []byte(item)) {
			return true
		}
	}
	return false
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
