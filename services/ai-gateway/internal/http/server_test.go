package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/ai-gateway/internal/middleware"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/ai-gateway/internal/provider"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/ai-gateway/internal/service"
)

func TestModelProfileRequiresServiceToken(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/internal/v1/model-profiles", nil)
	req.Header.Set("X-Caller-Service", "gateway")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestModelProfileRequiresCallerService(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/internal/v1/model-profiles", nil)
	req.Header.Set("X-Service-Token", "service-token")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestModelProfileRejectsUnknownCallerService(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/internal/v1/model-profiles", nil)
	req.Header.Set("X-Service-Token", "service-token")
	req.Header.Set("X-Caller-Service", "unknown-service")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"code":"forbidden"`)) {
		t.Fatalf("body = %s, want forbidden error", rec.Body.String())
	}
}

func TestCreateModelProfileDoesNotReturnAPIKey(t *testing.T) {
	server := newTestServer(t)
	body := `{"name":"default-chat","purpose":"chat","provider":"siliconflow","baseUrl":"https://api.siliconflow.cn/v1","model":"Qwen","apiKey":"sk-secret-value","enabled":true,"isDefault":true}`
	req := authedRequest(http.MethodPost, "/internal/v1/model-profiles", strings.NewReader(body))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("sk-secret-value")) || bytes.Contains(rec.Body.Bytes(), []byte("apiKey\"")) {
		t.Fatalf("response leaked api key: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("apiKeyConfigured")) {
		t.Fatalf("response missing apiKeyConfigured: %s", rec.Body.String())
	}
}

func TestInvalidJSONReturnsSecretSafeError(t *testing.T) {
	server := newTestServer(t)
	req := authedRequest(http.MethodPost, "/internal/v1/model-profiles", strings.NewReader(`{"apiKey":"sk-secret"`))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("sk-secret")) {
		t.Fatalf("error leaked request body: %s", rec.Body.String())
	}
}

func TestReadyReturnsDegradedWhenProfilesMissing(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("degraded")) {
		t.Fatalf("ready body = %s", rec.Body.String())
	}
}

func TestModelInvocationRoutesReturnNotImplemented(t *testing.T) {
	server := newTestServer(t)
	paths := []string{
		"/internal/v1/embeddings",
		"/internal/v1/rerankings",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := authedRequest(http.MethodPost, path, strings.NewReader(`{}`))
			rec := httptest.NewRecorder()

			server.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotImplemented {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if !bytes.Contains(rec.Body.Bytes(), []byte(`"type":"not_implemented_error"`)) {
				t.Fatalf("body = %s, want OpenAI-style not implemented error", rec.Body.String())
			}
			if bytes.Contains(rec.Body.Bytes(), []byte(`"data"`)) || bytes.Contains(rec.Body.Bytes(), []byte(`"requestId"`)) {
				t.Fatalf("body = %s, model invocation errors must not use project envelope", rec.Body.String())
			}
		})
	}
}

func TestCreateChatCompletionWithFakeProvider(t *testing.T) {
	var providerRequest []byte
	fakeProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("provider path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-secret-value" {
			t.Fatalf("provider auth = %q", got)
		}
		var err error
		providerRequest, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read provider request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","object":"chat.completion","created":1782631200,"model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_2","type":"function","function":{"name":"search","arguments":"{\"q\":\"safe\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	}))
	defer fakeProvider.Close()

	server := newTestServerWithChatProvider(t, provider.NewHTTPChatClient(fakeProvider.Client()))
	createBody := `{"name":"default-chat","purpose":"chat","provider":"openai_compatible","baseUrl":"` + fakeProvider.URL + `/v1","model":"provider-model","apiKey":"sk-secret-value","enabled":true,"isDefault":true,"supportsStreaming":true}`
	createReq := authedRequest(http.MethodPost, "/internal/v1/model-profiles", strings.NewReader(createBody))
	createRec := httptest.NewRecorder()
	server.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create profile status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	body := `{"model":"alias","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"search","arguments":"{\"q\":\"secret\"}"}}]},{"role":"tool","tool_call_id":"call_1","content":"secret prompt text"}],"tools":[{"type":"function","function":{"name":"search","parameters":{"type":"object"}}}],"tool_choice":"auto","parallel_tool_calls":true}`
	req := authedRequest(http.MethodPost, "/internal/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("X-Caller-Service", "qa")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(`"data"`)) || bytes.Contains(rec.Body.Bytes(), []byte(`"requestId"`)) {
		t.Fatalf("chat completion success must not use project envelope: %s", rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("sk-secret-value")) || bytes.Contains(rec.Body.Bytes(), []byte("secret prompt text")) {
		t.Fatalf("chat completion response leaked sensitive data: %s", rec.Body.String())
	}
	if !bytes.Contains(providerRequest, []byte(`"tools"`)) ||
		!bytes.Contains(providerRequest, []byte(`"parallel_tool_calls":true`)) ||
		!bytes.Contains(providerRequest, []byte(`"tool_calls"`)) ||
		!bytes.Contains(providerRequest, []byte(`"tool_call_id":"call_1"`)) {
		t.Fatalf("provider request did not pass through function calling fields: %s", string(providerRequest))
	}
	if !bytes.Contains(providerRequest, []byte(`"model":"provider-model"`)) {
		t.Fatalf("provider request did not use profile model: %s", string(providerRequest))
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"tool_calls"`)) {
		t.Fatalf("provider tool-call response was not returned: %s", rec.Body.String())
	}
}

func TestCreateChatCompletionStreamWithFakeProvider(t *testing.T) {
	fakeProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl_chunk\",\"object\":\"chat.completion.chunk\",\"created\":1782631200,\"model\":\"provider-model\",\"provider_trace\":\"raw-provider-secret\",\"choices\":[{\"index\":0,\"provider_debug\":\"raw-provider-secret\",\"delta\":{\"provider_context\":\"raw-provider-secret\",\"tool_calls\":[{\"id\":\"call_1\",\"type\":\"function\",\"provider_extra\":\"raw-provider-secret\",\"function\":{\"name\":\"search\",\"arguments\":\"{\\\"q\\\":\\\"x\\\"}\",\"provider_meta\":\"raw-provider-secret\"}}]},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer fakeProvider.Close()

	server := newTestServerWithChatProvider(t, provider.NewHTTPChatClient(fakeProvider.Client()))
	createBody := `{"name":"default-chat","purpose":"chat","provider":"local_compatible","baseUrl":"` + fakeProvider.URL + `/v1","model":"provider-model","apiKey":"sk-stream-secret","enabled":true,"isDefault":true,"supportsStreaming":true}`
	createReq := authedRequest(http.MethodPost, "/internal/v1/model-profiles", strings.NewReader(createBody))
	createRec := httptest.NewRecorder()
	server.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create profile status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	body := `{"model":"alias","stream":true,"messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"search","arguments":"{\"q\":\"x\"}"}}]},{"role":"tool","tool_call_id":"call_1","content":"tool result secret"}]}`
	req := authedRequest(http.MethodPost, "/internal/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("X-Caller-Service", "document")
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q", got)
	}
	if !strings.Contains(rec.Body.String(), "delta") || !strings.Contains(rec.Body.String(), "tool_calls") || !strings.Contains(rec.Body.String(), "[DONE]") {
		t.Fatalf("stream body missing tool-call delta or DONE: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "requestId") || strings.Contains(rec.Body.String(), "sk-stream-secret") || strings.Contains(rec.Body.String(), "tool result secret") {
		t.Fatalf("stream response leaked envelope or sensitive data: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "raw-provider-secret") || strings.Contains(rec.Body.String(), "provider_trace") {
		t.Fatalf("stream response leaked provider private fields: %s", rec.Body.String())
	}
}

func TestCreateChatCompletionStreamWithoutDoneRecordsFailure(t *testing.T) {
	fakeProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl_chunk\",\"object\":\"chat.completion.chunk\",\"created\":1782631200,\"model\":\"provider-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n"))
	}))
	defer fakeProvider.Close()

	server, repo := newTestServerWithChatProviderAndRepo(t, provider.NewHTTPChatClient(fakeProvider.Client()))
	createBody := `{"name":"default-chat","purpose":"chat","provider":"openai_compatible","baseUrl":"` + fakeProvider.URL + `/v1","model":"provider-model","apiKey":"sk-stream-secret","enabled":true,"isDefault":true,"supportsStreaming":true}`
	createReq := authedRequest(http.MethodPost, "/internal/v1/model-profiles", strings.NewReader(createBody))
	createRec := httptest.NewRecorder()
	server.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create profile status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	body := `{"model":"alias","stream":true,"messages":[{"role":"user","content":"secret prompt text"}]}`
	req := authedRequest(http.MethodPost, "/internal/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("X-Caller-Service", "qa")
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "[DONE]") {
		t.Fatalf("stream body synthesized DONE for incomplete provider stream: %s", rec.Body.String())
	}
	if len(repo.invocations) != 1 || len(repo.attempts) != 1 {
		t.Fatalf("recorded invocations=%d attempts=%d, want 1/1", len(repo.invocations), len(repo.attempts))
	}
	if repo.invocations[0].Status != service.InvocationFailed || repo.attempts[0].Status != service.InvocationFailed {
		t.Fatalf("stream status invocation=%s attempt=%s, want failed", repo.invocations[0].Status, repo.attempts[0].Status)
	}
	if repo.invocations[0].NormalizedErrorCode != "dependency_error" {
		t.Fatalf("NormalizedErrorCode = %q, want dependency_error", repo.invocations[0].NormalizedErrorCode)
	}
}

func TestCreateChatCompletionStreamRejectsNonContractBodyWithoutLeak(t *testing.T) {
	fakeProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"raw provider secret body"}`))
	}))
	defer fakeProvider.Close()

	server, repo := newTestServerWithChatProviderAndRepo(t, provider.NewHTTPChatClient(fakeProvider.Client()))
	createBody := `{"name":"default-chat","purpose":"chat","provider":"openai_compatible","baseUrl":"` + fakeProvider.URL + `/v1","model":"provider-model","apiKey":"sk-stream-secret","enabled":true,"isDefault":true,"supportsStreaming":true}`
	createReq := authedRequest(http.MethodPost, "/internal/v1/model-profiles", strings.NewReader(createBody))
	createRec := httptest.NewRecorder()
	server.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create profile status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	body := `{"model":"alias","stream":true,"messages":[{"role":"user","content":"secret prompt text"}]}`
	req := authedRequest(http.MethodPost, "/internal/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("X-Caller-Service", "qa")
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "raw provider secret body") || strings.Contains(rec.Body.String(), "secret prompt text") {
		t.Fatalf("stream error leaked provider or prompt body: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"type":"upstream_error"`) {
		t.Fatalf("body = %s, want OpenAI-style upstream error", rec.Body.String())
	}
	if len(repo.invocations) != 1 || repo.invocations[0].Status != service.InvocationFailed {
		t.Fatalf("recorded invocation = %+v, want failed", repo.invocations)
	}
}

func TestModelInvocationRoutesRejectUnknownCallerService(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("X-Service-Token", "service-token")
	req.Header.Set("X-Caller-Service", "unknown-service")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"type":"permission_error"`)) {
		t.Fatalf("body = %s, want OpenAI-style permission error", rec.Body.String())
	}
}

func TestModelInvocationRoutesRequireServiceToken(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("X-Caller-Service", "qa")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"type":"authentication_error"`)) {
		t.Fatalf("body = %s, want OpenAI-style auth error", rec.Body.String())
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return newTestServerWithChatProvider(t, nil)
}

func newTestServerWithChatProvider(t *testing.T, chatProvider service.ChatProvider) *Server {
	t.Helper()
	server, _ := newTestServerWithChatProviderAndRepo(t, chatProvider)
	return server
}

func newTestServerWithChatProviderAndRepo(t *testing.T, chatProvider service.ChatProvider) (*Server, *memoryRepository) {
	t.Helper()
	tokenHash := sha256.Sum256([]byte("service-token"))
	auth, err := middleware.NewServiceTokenAuthenticator([]string{"sha256:" + hex.EncodeToString(tokenHash[:])})
	if err != nil {
		t.Fatalf("NewServiceTokenAuthenticator() error = %v", err)
	}
	encryptor, err := service.NewCredentialEncryptor([]byte("12345678901234567890123456789012"), "local-v1")
	if err != nil {
		t.Fatalf("NewCredentialEncryptor() error = %v", err)
	}
	repo := newMemoryRepository()
	server := NewServer(Config{
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Profiles:      service.NewWithChatProvider(repo, encryptor, 60000, chatProvider),
		Authenticator: auth,
	})
	return server, repo
}

func authedRequest(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	req.Header.Set("X-Service-Token", "service-token")
	req.Header.Set("X-Caller-Service", "gateway")
	req.Header.Set("Content-Type", "application/json")
	return req
}
