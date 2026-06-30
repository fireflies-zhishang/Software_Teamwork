package parser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/knowledge/internal/service"
)

type ServiceClientConfig struct {
	BaseURL       string
	ServiceToken  string
	CallerService string
	Timeout       time.Duration
	Client        *http.Client
}

type ServiceClient struct {
	baseURL       string
	serviceToken  string
	callerService string
	client        *http.Client
}

const maxParserPayloadBytes = 8 << 20

var errParserDocumentTooLarge = errors.New("document is too large for parser")

func NewServiceClient(cfg ServiceClientConfig) (*ServiceClient, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("parser service base URL is required")
	}
	caller := strings.TrimSpace(cfg.CallerService)
	if caller == "" {
		caller = "knowledge"
	}
	client := cfg.Client
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	} else {
		copied := *client
		client = &copied
	}
	// Parser requests may include service credentials and document bytes. Treat
	// redirects as an error response so custom headers cannot be forwarded to
	// another host.
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &ServiceClient{
		baseURL:       baseURL,
		serviceToken:  strings.TrimSpace(cfg.ServiceToken),
		callerService: caller,
		client:        client,
	}, nil
}

func (c *ServiceClient) Parse(ctx context.Context, input service.ParseInput) (service.ParsedDocument, error) {
	if input.Body == nil {
		return service.ParsedDocument{}, fmt.Errorf("document body is required")
	}
	var sizeBytes *int64
	if input.SizeBytes >= 0 {
		if input.SizeBytes > maxParserPayloadBytes {
			return service.ParsedDocument{}, fmt.Errorf("document is too large for parser")
		}
		sizeBytes = &input.SizeBytes
	}
	parsed, err := c.parseStream(ctx, parserRequestMeta{
		DocumentName: strings.TrimSpace(input.Name),
		ContentType:  strings.TrimSpace(input.ContentType),
		SizeBytes:    sizeBytes,
	}, input.Body, input.RequestID, input.UserID)
	if err != nil {
		if errors.Is(err, errParserDocumentTooLarge) {
			return service.ParsedDocument{}, errParserDocumentTooLarge
		}
		if _, ok := service.Classify(err); ok {
			return service.ParsedDocument{}, err
		}
		return service.ParsedDocument{}, service.DependencyError("document parser service failed", err)
	}
	content := strings.TrimSpace(parsed.Content)
	if content == "" {
		return service.ParsedDocument{}, fmt.Errorf("document is empty")
	}
	return service.ParsedDocument{
		Content: content,
		Title:   strings.TrimSpace(parsed.Title),
		Backend: strings.TrimSpace(parsed.Backend),
		Pages:   normalizeParsedPages(parsed.Pages),
	}, nil
}

func (c *ServiceClient) parseStream(
	ctx context.Context,
	meta parserRequestMeta,
	body io.Reader,
	requestID string,
	userID string,
) (parsedDocument, error) {
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		errCh <- writeParserRequest(pw, meta, body)
	}()
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/internal/v1/parsed-documents",
		pr,
	)
	if err != nil {
		_ = pr.Close()
		writeErr := <-errCh
		if writeErr != nil {
			return parsedDocument{}, writeErr
		}
		return parsedDocument{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Caller-Service", c.callerService)
	if strings.TrimSpace(requestID) != "" {
		req.Header.Set("X-Request-Id", strings.TrimSpace(requestID))
	}
	if strings.TrimSpace(userID) != "" {
		req.Header.Set("X-User-Id", strings.TrimSpace(userID))
	}
	if c.serviceToken != "" {
		req.Header.Set("X-Service-Token", c.serviceToken)
	}

	res, err := c.client.Do(req)
	if err != nil {
		_ = pr.CloseWithError(err)
		writeErr := <-errCh
		if writeErr != nil {
			return parsedDocument{}, writeErr
		}
		return parsedDocument{}, fmt.Errorf("parser service request failed")
	}
	_ = pr.CloseWithError(io.ErrClosedPipe)
	writeErr := <-errCh
	if writeErr != nil && !errors.Is(writeErr, io.ErrClosedPipe) {
		_ = res.Body.Close()
		return parsedDocument{}, writeErr
	}
	return decodeParserResponse(res)
}

func writeParserRequest(w *io.PipeWriter, meta parserRequestMeta, body io.Reader) error {
	meta = parserRequestMeta{
		DocumentName: strings.TrimSpace(meta.DocumentName),
		ContentType:  strings.TrimSpace(meta.ContentType),
		SizeBytes:    meta.SizeBytes,
	}
	prefix, err := json.Marshal(meta)
	if err != nil {
		return closePipeWithError(w, err)
	}
	if len(prefix) > 0 && prefix[len(prefix)-1] == '}' {
		prefix = prefix[:len(prefix)-1]
	}
	if _, err := w.Write(prefix); err != nil {
		return err
	}
	if len(prefix) > 1 {
		if _, err := io.WriteString(w, ","); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, `"dataBase64":"`); err != nil {
		return err
	}
	counter := &limitCounter{limit: maxParserPayloadBytes}
	encoder := base64.NewEncoder(base64.StdEncoding, w)
	_, copyErr := io.Copy(encoder, io.TeeReader(body, counter))
	closeErr := encoder.Close()
	if copyErr != nil {
		err := copyErr
		if counter.exceeded {
			err = errParserDocumentTooLarge
		}
		return closePipeWithError(w, err)
	}
	if closeErr != nil {
		return closePipeWithError(w, closeErr)
	}
	if _, err := io.WriteString(w, `"}`); err != nil {
		return err
	}
	return w.Close()
}

func decodeParserResponse(res *http.Response) (parsedDocument, error) {
	defer res.Body.Close()
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1024))
		switch res.StatusCode {
		case http.StatusBadRequest, http.StatusRequestEntityTooLarge:
			return parsedDocument{}, service.ValidationError("document parsing failed", map[string]string{"file": "could not be parsed"})
		default:
			return parsedDocument{}, service.DependencyError("parser service failed", nil)
		}
	}
	var decoded parserResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, maxParserPayloadBytes+1)).Decode(&decoded); err != nil {
		return parsedDocument{}, fmt.Errorf("parser service response could not be decoded")
	}
	if len(decoded.Data.Content) > maxParserPayloadBytes {
		return parsedDocument{}, fmt.Errorf("parser service response is too large")
	}
	return decoded.Data, nil
}

func closePipeWithError(w *io.PipeWriter, err error) error {
	_ = w.CloseWithError(err)
	return err
}

type limitCounter struct {
	limit    int64
	read     int64
	exceeded bool
}

func (c *limitCounter) Write(p []byte) (int, error) {
	c.read += int64(len(p))
	if c.read > c.limit {
		c.exceeded = true
		return 0, errParserDocumentTooLarge
	}
	return len(p), nil
}

type parserRequestMeta struct {
	DocumentName string `json:"documentName,omitempty"`
	ContentType  string `json:"contentType,omitempty"`
	SizeBytes    *int64 `json:"sizeBytes,omitempty"`
}

type parserResponse struct {
	Data parsedDocument `json:"data"`
}

type parsedDocument struct {
	Content string       `json:"content"`
	Title   string       `json:"title"`
	Backend string       `json:"backend"`
	Pages   []parsedPage `json:"pages"`
}

type parsedPage struct {
	PageNumber      int      `json:"pageNumber"`
	Content         string   `json:"content"`
	ParseStrategy   string   `json:"parseStrategy"`
	TextLayerStatus string   `json:"textLayerStatus"`
	OCRConfidence   *float64 `json:"ocrConfidence"`
	DPI             *int     `json:"dpi"`
	Warnings        []string `json:"warnings"`
}

func normalizeParsedPages(pages []parsedPage) []service.ParsedPage {
	if len(pages) == 0 {
		return nil
	}
	normalized := make([]service.ParsedPage, 0, len(pages))
	for _, page := range pages {
		content := strings.TrimSpace(page.Content)
		if page.PageNumber <= 0 || content == "" {
			continue
		}
		normalized = append(normalized, service.ParsedPage{
			PageNumber:      page.PageNumber,
			Content:         content,
			ParseStrategy:   strings.TrimSpace(page.ParseStrategy),
			TextLayerStatus: strings.TrimSpace(page.TextLayerStatus),
			OCRConfidence:   normalizeConfidence(page.OCRConfidence),
			DPI:             normalizeDPI(page.DPI),
			Warnings:        normalizeWarnings(page.Warnings),
		})
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizeConfidence(value *float64) *float64 {
	if value == nil {
		return nil
	}
	normalized := *value
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 1 {
		normalized = 1
	}
	return &normalized
}

func normalizeDPI(value *int) *int {
	if value == nil || *value <= 0 {
		return nil
	}
	normalized := *value
	return &normalized
}

func normalizeWarnings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		warning := strings.TrimSpace(value)
		if warning != "" {
			normalized = append(normalized, warning)
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}
