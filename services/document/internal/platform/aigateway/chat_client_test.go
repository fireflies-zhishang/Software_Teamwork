package aigateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/document/internal/service"
)

func TestChatClientCreatesCompletionWithInternalHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/v1/chat/completions" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Service-Token") != "service-token" {
			t.Fatalf("X-Service-Token = %q", r.Header.Get("X-Service-Token"))
		}
		if r.Header.Get("X-Caller-Service") != "document" {
			t.Fatalf("X-Caller-Service = %q", r.Header.Get("X-Caller-Service"))
		}
		if r.Header.Get("X-Request-Id") != "req-chat" {
			t.Fatalf("X-Request-Id = %q", r.Header.Get("X-Request-Id"))
		}
		if r.Header.Get("X-User-Id") != "user-chat" {
			t.Fatalf("X-User-Id = %q", r.Header.Get("X-User-Id"))
		}
		var body struct {
			Model     string                `json:"model"`
			ProfileID string                `json:"profile_id"`
			Messages  []service.ChatMessage `json:"messages"`
			Stream    bool                  `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "model-default" || body.ProfileID != "profile-default" || body.Stream {
			t.Fatalf("request body = %+v", body)
		}
		if len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.Messages[0].Content != "生成大纲" {
			t.Fatalf("messages = %+v", body.Messages)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl_1",
			"object":  "chat.completion",
			"created": 1782631200,
			"model":   "model-default",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "{\"sections\":[{\"title\":\"总述\"}]}",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 20,
				"total_tokens":      30,
			},
		})
	}))
	defer server.Close()

	client, err := NewChatClient(server.URL, "service-token", "profile-default", "model-default", server.Client())
	if err != nil {
		t.Fatalf("NewChatClient() error = %v", err)
	}
	resp, err := client.CreateChatCompletion(context.Background(), service.RequestContext{
		RequestID: "req-chat",
		UserID:    "user-chat",
	}, service.ChatCompletionRequest{
		Messages: []service.ChatMessage{{Role: "user", Content: "生成大纲"}},
	})
	if err != nil {
		t.Fatalf("CreateChatCompletion() error = %v", err)
	}
	if resp.Content != "{\"sections\":[{\"title\":\"总述\"}]}" || resp.FinishReason != "stop" || resp.Usage.TotalTokens != 30 {
		t.Fatalf("completion response = %+v", resp)
	}
}

func TestChatClientSanitizesDownstreamError(t *testing.T) {
	rawBody := `{"error":{"message":"provider failed with sk-secret and https://provider.internal/v1","type":"upstream_error"}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(rawBody))
	}))
	defer server.Close()

	client, err := NewChatClient(server.URL, "service-token", "profile-default", "model-default", server.Client())
	if err != nil {
		t.Fatalf("NewChatClient() error = %v", err)
	}
	_, err = client.CreateChatCompletion(context.Background(), service.RequestContext{}, service.ChatCompletionRequest{
		Messages: []service.ChatMessage{{Role: "user", Content: "prompt must stay local"}},
	})
	if err == nil {
		t.Fatal("CreateChatCompletion() error = nil, want dependency error")
	}
	if strings.Contains(err.Error(), "sk-secret") || strings.Contains(err.Error(), "provider.internal") || strings.Contains(err.Error(), "prompt must stay local") {
		t.Fatalf("error leaked sensitive data: %v", err)
	}
	appErr, ok := service.Classify(err)
	if !ok || appErr.Code != service.CodeDependency {
		t.Fatalf("error = %#v, want dependency error", err)
	}
}
