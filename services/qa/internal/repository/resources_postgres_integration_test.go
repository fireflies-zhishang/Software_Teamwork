package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/qa/internal/service"
)

func TestDocumentedResourceRoundTrip(t *testing.T) {
	databaseURL := os.Getenv("QA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("QA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	repo, err := NewPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	now := time.Now().UTC()
	suffix := uint64(now.UnixNano()) & 0xffffffffffff
	conversationID := integrationUUID(suffix)
	userMessageID := integrationUUID(suffix + 1)
	assistantMessageID := integrationUUID(suffix + 2)
	conversation := service.Conversation{ID: conversationID, OwnerUserID: "integration-user", Title: "contract", Status: "active", CreatedAt: now, UpdatedAt: now}
	if _, err = repo.CreateConversation(ctx, conversation); err != nil {
		t.Fatal(err)
	}
	run, err := repo.AppendMessages(ctx, "integration-user", conversationID, service.ResponseRunStart{RequestID: "req-integration", MaxIterations: 5}, service.Message{ID: userMessageID, ConversationID: conversationID, Role: "user", Content: "question", Status: "completed", CreatedAt: now}, service.Message{ID: assistantMessageID, ConversationID: conversationID, Role: "assistant", Status: "streaming", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	events := []service.StreamEvent{{EventSeq: 1, EventType: "agent.iteration.started", Payload: map[string]any{"iterationNo": 1}, CreatedAt: now}, {EventSeq: 2, EventType: "tool.started", Payload: map[string]any{"iterationNo": 1, "toolCallId": "call-1", "tool": "search_knowledge"}, CreatedAt: now}, {EventSeq: 3, EventType: "tool.completed", Payload: map[string]any{"iterationNo": 1, "toolCallId": "call-1", "tool": "search_knowledge"}, CreatedAt: now.Add(time.Millisecond)}}
	if err = repo.SaveStreamEvents(ctx, "integration-user", run.ID, events); err != nil {
		t.Fatal(err)
	}
	invocationID, err := repo.SaveModelInvocation(ctx, "integration-user", service.ModelInvocation{
		ResponseRunID: run.ID, IterationNo: 1, Provider: "ai-gateway", ProfileID: "default",
		ModelName: "deepseek-v4-pro", FinishReason: "stop", Status: "completed",
		StartedAt: now, FinishedAt: ptrTime(now.Add(time.Millisecond)), LatencyMS: 1,
	})
	if err != nil || invocationID == "" {
		t.Fatalf("invocation=%q err=%v", invocationID, err)
	}
	rows, err := repo.queries.ListModelInvocationsByRun(ctx, run.ID, "integration-user")
	if err != nil || len(rows) != 1 || rows[0].Status != "completed" {
		t.Fatalf("invocations=%+v err=%v", rows, err)
	}
	replayed, err := repo.ListStreamEvents(ctx, "integration-user", conversationID, run.ID, 0)
	if err != nil || len(replayed) != 3 {
		t.Fatalf("events=%d err=%v", len(replayed), err)
	}
	calls, err := repo.ListToolCalls(ctx, "integration-user", run.ID)
	if err != nil || len(calls) != 1 || calls[0].Status != "completed" {
		t.Fatalf("calls=%+v err=%v", calls, err)
	}
	cancelled, err := repo.CancelResponseRun(ctx, "integration-user", run.ID)
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatalf("run=%+v err=%v", cancelled, err)
	}
	_, err = repo.FinalizeResponseRun(ctx, "integration-user", service.ResponseRunFinalization{
		RunID: run.ID,
		AssistantMessage: service.Message{
			ID:             assistantMessageID,
			ConversationID: conversationID,
			Role:           "assistant",
			Content:        "late answer should not win",
			Status:         "completed",
			CreatedAt:      now,
		},
		Status:            "completed",
		TerminationReason: "completed",
		CurrentIteration:  1,
		CompletedAt:       now.Add(2 * time.Millisecond),
	})
	if appErr, ok := service.Classify(err); !ok || appErr.Code != service.CodeConflict {
		t.Fatalf("finalize cancelled run err=%v, want conflict", err)
	}
	messages, err := repo.ListMessages(ctx, "integration-user", conversationID, service.MessageListOptions{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages.Items) != 2 || messages.Items[1].Status != "cancelled" || messages.Items[1].Content == "late answer should not win" {
		t.Fatalf("cancelled assistant message was overwritten: %+v", messages.Items)
	}
	cancelledSteps := []service.ReasoningStep{{
		ID:        integrationUUID(suffix + 3),
		MessageID: assistantMessageID,
		Type:      "generation",
		Title:     "Generate answer",
		Summary:   "Model call was cancelled by the user.",
		Status:    "failed",
		CreatedAt: now.Add(3 * time.Millisecond),
	}}
	if err = repo.SaveReasoningSteps(ctx, "integration-user", assistantMessageID, cancelledSteps); err != nil {
		t.Fatalf("save cancelled reasoning steps: %v", err)
	}
	cancelledEvents := []service.StreamEvent{
		{EventSeq: 1, EventType: "message.created", Payload: map[string]any{"responseRunId": run.ID, "userMessageId": userMessageID, "assistantMessageId": assistantMessageID, "status": "running"}, CreatedAt: now},
		{EventSeq: 2, EventType: "agent.iteration.started", Payload: map[string]any{"responseRunId": run.ID, "iterationNo": 1}, CreatedAt: now.Add(time.Millisecond)},
		{EventSeq: 3, EventType: "reasoning.step", Payload: map[string]any{"type": "generation", "label": "Generate answer", "status": "failed", "detail": "Model call was cancelled by the user."}, CreatedAt: now.Add(2 * time.Millisecond)},
		{EventSeq: 4, EventType: "error", Payload: map[string]any{"responseRunId": run.ID, "code": "dependency_error", "message": "answer generation was cancelled"}, CreatedAt: now.Add(3 * time.Millisecond)},
	}
	if err = repo.SaveStreamEvents(ctx, "integration-user", run.ID, cancelledEvents); err != nil {
		t.Fatalf("save cancelled stream events: %v", err)
	}
	replayed, err = repo.ListStreamEvents(ctx, "integration-user", conversationID, run.ID, 0)
	if err != nil || len(replayed) != len(cancelledEvents) || replayed[len(replayed)-1].EventType != "error" {
		t.Fatalf("cancelled events=%+v err=%v", replayed, err)
	}
	messagesWithThinking, err := repo.ListMessages(ctx, "integration-user", conversationID, service.MessageListOptions{Page: 1, PageSize: 10, IncludeThinking: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(messagesWithThinking.Items) != 2 || len(messagesWithThinking.Items[1].Thinking) != 1 || messagesWithThinking.Items[1].Thinking[0].Status != "failed" {
		t.Fatalf("cancelled reasoning steps not replayable: %+v", messagesWithThinking.Items)
	}
	qaConfig, err := repo.CreateQAConfigVersionResource(ctx, "integration-user", service.CreateQAConfigVersionInput{TopK: 7, MaxIterations: 6, KnowledgeBases: []service.ConfigKnowledgeBase{{ID: "kb-1"}}})
	if err != nil || qaConfig.Retrieval.TopK != 7 || qaConfig.MaxIterations != 6 || qaConfig.Agent.MaxIterations != 6 {
		t.Fatalf("qa config=%+v err=%v", qaConfig, err)
	}
	llmConfig, err := repo.CreateLLMConfigVersionResource(ctx, "integration-user", service.CreateLLMConfigVersionInput{Provider: "ai-gateway", ProfileID: "profile-chat", ModelName: "model", TimeoutSeconds: 30, MaxTokens: 512})
	if err != nil || llmConfig.ProfileID != "profile-chat" {
		t.Fatalf("llm config=%+v err=%v", llmConfig, err)
	}
	retrieval, err := repo.SaveRetrievalTestRun(ctx, "integration-user", service.RetrievalTestInput{Question: "query"}, []service.RetrievalTestResult{{KnowledgeBaseID: "kb-1", DocumentID: "doc-1", ChunkID: "chunk-1", VectorScore: .9, ContentPreview: "preview", Metadata: map[string]any{}}}, time.Millisecond, nil)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.GetRetrievalTestRun(ctx, "integration-user", retrieval.ID)
	if err != nil || len(loaded.Results) != 1 {
		t.Fatalf("retrieval=%+v err=%v", loaded, err)
	}
	if _, err = repo.GetMetricsOverview(ctx, 1); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerAuthorizationBoundaries(t *testing.T) {
	databaseURL := os.Getenv("QA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("QA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	repo, err := NewPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	now := time.Now().UTC()
	suffix := uint64(now.UnixNano()) & 0xffffffffffff
	conversationID := integrationUUID(suffix)
	userMessageID := integrationUUID(suffix + 1)
	assistantMessageID := integrationUUID(suffix + 2)
	citationID := integrationUUID(suffix + 3)
	ownerID := "authorization-owner"
	otherUserID := "authorization-other-user"
	conversation := service.Conversation{
		ID: conversationID, OwnerUserID: ownerID, Title: "private session",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}
	if _, err = repo.CreateConversation(ctx, conversation); err != nil {
		t.Fatal(err)
	}
	run, err := repo.AppendMessages(ctx, ownerID, conversationID,
		service.ResponseRunStart{RequestID: "req-authorization", MaxIterations: 5},
		service.Message{ID: userMessageID, ConversationID: conversationID, Role: "user", Content: "private question", Status: "completed", CreatedAt: now},
		service.Message{ID: assistantMessageID, ConversationID: conversationID, Role: "assistant", Content: "private answer", Status: "generating", CreatedAt: now},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.pool.Exec(ctx, `INSERT INTO citations(id,message_id,citation_no,doc_name) VALUES($1,$2,1,$3)`, citationID, assistantMessageID, "private source"); err != nil {
		t.Fatal(err)
	}

	_, err = repo.GetConversation(ctx, otherUserID, conversationID)
	requireServiceCode(t, err, service.CodeForbidden)
	_, err = repo.UpdateConversation(ctx, otherUserID, conversation)
	requireServiceCode(t, err, service.CodeForbidden)
	requireServiceCode(t, repo.DeleteConversation(ctx, otherUserID, conversationID), service.CodeForbidden)
	_, err = repo.ListMessages(ctx, otherUserID, conversationID, service.MessageListOptions{Page: 1, PageSize: 50})
	requireServiceCode(t, err, service.CodeForbidden)

	_, err = repo.GetResponseRun(ctx, otherUserID, run.ID)
	requireServiceCode(t, err, service.CodeNotFound)
	_, err = repo.CancelResponseRun(ctx, otherUserID, run.ID)
	requireServiceCode(t, err, service.CodeNotFound)
	_, err = repo.ListStreamEvents(ctx, otherUserID, conversationID, run.ID, 0)
	requireServiceCode(t, err, service.CodeNotFound)
	_, err = repo.ListToolCalls(ctx, otherUserID, run.ID)
	requireServiceCode(t, err, service.CodeNotFound)
	_, err = repo.ListMessageCitations(ctx, otherUserID, assistantMessageID)
	requireServiceCode(t, err, service.CodeNotFound)
	_, err = repo.GetCitation(ctx, otherUserID, citationID)
	requireServiceCode(t, err, service.CodeNotFound)
	lookup, err := repo.LookupCitations(ctx, otherUserID, []string{citationID})
	if err != nil || len(lookup) != 0 {
		t.Fatalf("cross-user citation lookup=%+v err=%v", lookup, err)
	}

	if _, err = repo.CancelResponseRun(ctx, ownerID, run.ID); err != nil {
		t.Fatal(err)
	}
	_, err = repo.CancelResponseRun(ctx, ownerID, run.ID)
	requireServiceCode(t, err, service.CodeConflict)
	if err = repo.DeleteConversation(ctx, ownerID, conversationID); err != nil {
		t.Fatal(err)
	}
	_, err = repo.GetConversation(ctx, ownerID, conversationID)
	requireServiceCode(t, err, service.CodeNotFound)
	_, err = repo.GetConversation(ctx, ownerID, integrationUUID(suffix+4))
	requireServiceCode(t, err, service.CodeNotFound)
}

func integrationUUID(value uint64) string { return fmt.Sprintf("00000000-0000-4000-8000-%012x", value) }

func ptrTime(value time.Time) *time.Time { return &value }

func requireServiceCode(t *testing.T, err error, want service.Code) {
	t.Helper()
	appErr, ok := service.Classify(err)
	if !ok || appErr.Code != want {
		t.Fatalf("error=%v, want code %q", err, want)
	}
}
