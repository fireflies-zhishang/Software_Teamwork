package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/qa/internal/repository/sqlc"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/qa/internal/service"
)

func (r *Postgres) ListStreamEvents(ctx context.Context, userID, sessionID, runID string, after int) ([]service.StreamEvent, error) {
	rows, err := r.queries.ListStreamEventsForRun(ctx, sqlc.ListStreamEventsForRunParams{
		ResponseRunID: runID, ConversationID: sessionID, ExternalUserID: userID, AfterSeq: int32(after),
	})
	if err != nil {
		return nil, fmt.Errorf("list stream events: %w", err)
	}
	items := make([]service.StreamEvent, 0, len(rows))
	for _, row := range rows {
		var item service.StreamEvent
		item.EventSeq = int(row.EventSeq)
		item.EventType = row.EventType
		item.CreatedAt = row.CreatedAt
		if err := json.Unmarshal(row.Payload, &item.Payload); err != nil {
			return nil, fmt.Errorf("decode stream event: %w", err)
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		run, err := r.GetResponseRun(ctx, userID, runID)
		if err != nil {
			return nil, err
		}
		if run.SessionID != sessionID {
			return nil, service.NewError(service.CodeNotFound, "response run not found", nil)
		}
	}
	return items, nil
}

const citationSelect = `SELECT ci.id::text,ci.message_id::text,ci.citation_no,COALESCE(ci.external_doc_id,''),ci.doc_name,COALESCE(ci.external_kb_id,''),COALESCE(ci.external_chunk_id,''),COALESCE(ci.section_path,''),COALESCE(ci.quote_text,''),COALESCE(ci.context,''),ci.page_number,ci.score,ci.rerank_score,COALESCE(ci.chunk_type,''),ci.metadata FROM citations ci JOIN messages m ON m.id=ci.message_id JOIN conversations c ON c.id=m.conversation_id`

func (r *Postgres) ListMessageCitations(ctx context.Context, userID, messageID string) ([]service.Citation, error) {
	rows, err := r.pool.Query(ctx, citationSelect+` WHERE ci.message_id::text=$1 AND c.external_user_id=$2 AND c.deleted_at IS NULL ORDER BY ci.citation_no`, messageID, userID)
	if err != nil {
		return nil, fmt.Errorf("list message citations: %w", err)
	}
	defer rows.Close()
	items, err := scanCitations(rows)
	if err != nil {
		return nil, err
	}
	rows.Close()
	if len(items) == 0 {
		if err := r.authorizeMessageForUser(ctx, userID, messageID); err != nil {
			return nil, err
		}
	}
	return items, nil
}
func (r *Postgres) GetCitation(ctx context.Context, userID, id string) (service.Citation, error) {
	return scanCitation(r.pool.QueryRow(ctx, citationSelect+` WHERE ci.id::text=$1 AND c.external_user_id=$2 AND c.deleted_at IS NULL`, id, userID))
}
func (r *Postgres) LookupCitations(ctx context.Context, userID string, ids []string) ([]service.Citation, error) {
	rows, err := r.pool.Query(ctx, citationSelect+` WHERE ci.id::text=ANY($1) AND c.external_user_id=$2 AND c.deleted_at IS NULL ORDER BY array_position($1::text[],ci.id::text)`, ids, userID)
	if err != nil {
		return nil, fmt.Errorf("lookup citations: %w", err)
	}
	defer rows.Close()
	return scanCitations(rows)
}
func scanCitations(rows pgx.Rows) ([]service.Citation, error) {
	items := make([]service.Citation, 0)
	for rows.Next() {
		item, err := scanCitation(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func scanCitation(row rowScanner) (service.Citation, error) {
	var item service.Citation
	var page sql.NullInt32
	var score, rerank sql.NullFloat64
	var metadata []byte
	err := row.Scan(&item.ID, &item.MessageID, &item.CitationNo, &item.DocumentID, &item.DocumentName, &item.KnowledgeBaseID, &item.ChunkID, &item.SectionPath, &item.Text, &item.Context, &page, &score, &rerank, &item.ChunkType, &metadata)
	if errors.Is(err, pgx.ErrNoRows) {
		return service.Citation{}, service.NewError(service.CodeNotFound, "citation not found", err)
	}
	if err != nil {
		return service.Citation{}, fmt.Errorf("scan citation: %w", err)
	}
	if page.Valid {
		v := int(page.Int32)
		item.PageNumber = &v
	}
	if score.Valid {
		item.Score = &score.Float64
	}
	if rerank.Valid {
		item.RerankScore = &rerank.Float64
	}
	if len(metadata) > 0 {
		_ = json.Unmarshal(metadata, &item.Metadata)
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	item.ContentPreview = item.Text
	item.Content = item.Context
	if item.Content == "" {
		item.Content = item.Text
	}
	item.IsSourceAvailable = item.DocumentID != ""
	item.Source = &service.CitationSource{Available: item.IsSourceAvailable}
	if item.IsSourceAvailable {
		item.Source.DownloadEndpoint = "/api/v1/documents/" + item.DocumentID + "/content"
	} else {
		item.Source.Reason = "source_unavailable"
	}
	return item, nil
}

func (r *Postgres) ListToolCalls(ctx context.Context, userID, runID string) ([]service.AgentToolCall, error) {
	rows, err := r.pool.Query(ctx, `SELECT tc.id::text,tc.response_run_id::text,COALESCE(tc.model_invocation_id::text,''),tc.iteration_no,tc.tool_call_id,tc.tool_name,tc.arguments_summary,tc.result_summary,tc.status,COALESCE(tc.latency_ms,0),tc.started_at,tc.finished_at FROM agent_tool_calls tc JOIN response_runs rr ON rr.id=tc.response_run_id JOIN conversations c ON c.id=rr.conversation_id WHERE tc.response_run_id::text=$1 AND c.external_user_id=$2 AND c.deleted_at IS NULL ORDER BY tc.started_at,tc.id`, runID, userID)
	if err != nil {
		return nil, fmt.Errorf("list tool calls: %w", err)
	}
	defer rows.Close()
	items := make([]service.AgentToolCall, 0)
	for rows.Next() {
		var item service.AgentToolCall
		var args, result []byte
		if err := rows.Scan(&item.ID, &item.ResponseRunID, &item.ModelInvocationID, &item.IterationNo, &item.ToolCallID, &item.ToolName, &args, &result, &item.Status, &item.LatencyMS, &item.StartedAt, &item.FinishedAt); err != nil {
			return nil, fmt.Errorf("scan tool call: %w", err)
		}
		_ = json.Unmarshal(args, &item.ArgumentsSummary)
		_ = json.Unmarshal(result, &item.ResultSummary)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if len(items) == 0 {
		if _, err := r.GetResponseRun(ctx, userID, runID); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (r *Postgres) authorizeMessageForUser(ctx context.Context, userID, messageID string) error {
	var authorized bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM messages m
			JOIN conversations c ON c.id = m.conversation_id
			WHERE m.id::text = $1
				AND c.external_user_id = $2
				AND c.deleted_at IS NULL
		)`, messageID, userID).Scan(&authorized)
	if err != nil {
		return fmt.Errorf("authorize message access: %w", err)
	}
	if !authorized {
		return service.NewError(service.CodeNotFound, "message not found", nil)
	}
	return nil
}

func (r *Postgres) GetActiveQAConfigVersion(ctx context.Context) (service.QAConfigVersion, error) {
	return r.getQAConfigVersion(ctx, "", true)
}
func (r *Postgres) getQAConfigVersion(ctx context.Context, id string, active bool) (service.QAConfigVersion, error) {
	query := `SELECT id::text,version_no,top_k,similarity_threshold,use_rerank,COALESCE(rerank_threshold,0),COALESCE(rerank_top_n,0),max_iterations,tool_timeout_seconds,model_timeout_seconds,overall_timeout_seconds,enabled_tool_names,is_active,created_at FROM qa_config_versions WHERE `
	args := []any{}
	if active {
		query += `is_active=true ORDER BY version_no DESC LIMIT 1`
	} else {
		query += `id=$1`
		args = append(args, id)
	}
	var v service.QAConfigVersion
	var tools []byte
	err := r.pool.QueryRow(ctx, query, args...).Scan(&v.ID, &v.VersionNo, &v.Retrieval.TopK, &v.Retrieval.ScoreThreshold, &v.Retrieval.EnableRerank, &v.Retrieval.RerankThreshold, &v.Retrieval.RerankTopN, &v.Agent.MaxIterations, &v.Agent.ToolTimeoutSeconds, &v.Agent.ModelTimeoutSeconds, &v.Agent.OverallTimeoutSeconds, &tools, &v.IsActive, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return service.QAConfigVersion{}, service.NewError(service.CodeNotFound, "QA configuration not found", err)
	}
	if err != nil {
		return service.QAConfigVersion{}, fmt.Errorf("get QA config version: %w", err)
	}
	_ = json.Unmarshal(tools, &v.Agent.EnabledToolNames)
	applyQAConfigVersionCompatibilityFields(&v)
	rows, err := r.pool.Query(ctx, `SELECT external_kb_id,COALESCE(kb_type,''),COALESCE(display_name_snapshot,''),sort_order FROM qa_config_knowledge_bases WHERE config_id=$1 ORDER BY sort_order,external_kb_id`, v.ID)
	if err != nil {
		return service.QAConfigVersion{}, fmt.Errorf("list QA config knowledge bases: %w", err)
	}
	defer rows.Close()
	v.KnowledgeBases = []service.ConfigKnowledgeBase{}
	v.DefaultKnowledgeBaseIDs = []string{}
	for rows.Next() {
		var kb service.ConfigKnowledgeBase
		if err := rows.Scan(&kb.ID, &kb.Type, &kb.DisplayName, &kb.SortOrder); err != nil {
			return service.QAConfigVersion{}, err
		}
		v.KnowledgeBases = append(v.KnowledgeBases, kb)
		v.DefaultKnowledgeBaseIDs = append(v.DefaultKnowledgeBaseIDs, kb.ID)
	}
	return v, rows.Err()
}

func applyQAConfigVersionCompatibilityFields(v *service.QAConfigVersion) {
	v.MaxIterations = v.Agent.MaxIterations
	v.ToolTimeoutSeconds = v.Agent.ToolTimeoutSeconds
	v.ModelTimeoutSeconds = v.Agent.ModelTimeoutSeconds
	v.OverallTimeoutSeconds = v.Agent.OverallTimeoutSeconds
	v.EnabledToolNames = append([]string(nil), v.Agent.EnabledToolNames...)
}

func (r *Postgres) CreateQAConfigVersionResource(ctx context.Context, userID string, input service.CreateQAConfigVersionInput) (service.QAConfigVersion, error) {
	retrieval := input.Retrieval
	if retrieval.TopK == 0 {
		retrieval.TopK = input.TopK
	}
	if retrieval.TopK == 0 {
		retrieval.TopK = 5
	}
	if retrieval.ScoreThreshold == 0 {
		retrieval.ScoreThreshold = input.SimilarityThreshold
	}
	if retrieval.ScoreThreshold == 0 {
		retrieval.ScoreThreshold = .7
	}
	if input.UseRerank {
		retrieval.EnableRerank = true
	}
	if retrieval.RerankThreshold == 0 {
		retrieval.RerankThreshold = input.RerankThreshold
	}
	if retrieval.RerankTopN == 0 {
		retrieval.RerankTopN = input.RerankTopN
	}
	agent := input.Agent
	if agent.MaxIterations == 0 {
		agent.MaxIterations = input.MaxIterations
	}
	if agent.MaxIterations == 0 {
		agent.MaxIterations = 5
	}
	if agent.ToolTimeoutSeconds == 0 {
		agent.ToolTimeoutSeconds = input.ToolTimeoutSeconds
	}
	if agent.ToolTimeoutSeconds == 0 {
		agent.ToolTimeoutSeconds = 10
	}
	if agent.ModelTimeoutSeconds == 0 {
		agent.ModelTimeoutSeconds = input.ModelTimeoutSeconds
	}
	if agent.ModelTimeoutSeconds == 0 {
		agent.ModelTimeoutSeconds = 60
	}
	if agent.OverallTimeoutSeconds == 0 {
		agent.OverallTimeoutSeconds = input.OverallTimeoutSeconds
	}
	if agent.OverallTimeoutSeconds == 0 {
		agent.OverallTimeoutSeconds = 120
	}
	if len(agent.EnabledToolNames) == 0 {
		agent.EnabledToolNames = input.EnabledToolNames
	}
	if agent.EnabledToolNames == nil {
		agent.EnabledToolNames = []string{}
	}
	activate := input.Activate == nil || *input.Activate
	kbs := input.KnowledgeBases
	if len(kbs) == 0 {
		for i, id := range input.DefaultKnowledgeBaseIDs {
			kbs = append(kbs, service.ConfigKnowledgeBase{ID: id, SortOrder: i})
		}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return service.QAConfigVersion{}, fmt.Errorf("begin create QA config: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `LOCK TABLE qa_config_versions IN EXCLUSIVE MODE`); err != nil {
		return service.QAConfigVersion{}, err
	}
	if activate {
		if _, err = tx.Exec(ctx, `UPDATE qa_config_versions SET is_active=false WHERE is_active=true`); err != nil {
			return service.QAConfigVersion{}, err
		}
	}
	tools, _ := json.Marshal(agent.EnabledToolNames)
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO qa_config_versions(version_no,top_k,similarity_threshold,use_rerank,rerank_threshold,rerank_top_n,max_iterations,tool_timeout_seconds,model_timeout_seconds,overall_timeout_seconds,enabled_tool_names,is_active,created_by_user_id) VALUES((SELECT COALESCE(MAX(version_no),0)+1 FROM qa_config_versions),$1,$2,$3,NULLIF($4,0),NULLIF($5,0),$6,$7,$8,$9,$10,$11,$12) RETURNING id::text`, retrieval.TopK, retrieval.ScoreThreshold, retrieval.EnableRerank, retrieval.RerankThreshold, retrieval.RerankTopN, agent.MaxIterations, agent.ToolTimeoutSeconds, agent.ModelTimeoutSeconds, agent.OverallTimeoutSeconds, tools, activate, userID).Scan(&id)
	if err != nil {
		return service.QAConfigVersion{}, fmt.Errorf("insert QA config: %w", err)
	}
	for _, kb := range kbs {
		if _, err = tx.Exec(ctx, `INSERT INTO qa_config_knowledge_bases(config_id,external_kb_id,kb_type,display_name_snapshot,sort_order) VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),$5)`, id, kb.ID, kb.Type, kb.DisplayName, kb.SortOrder); err != nil {
			return service.QAConfigVersion{}, fmt.Errorf("insert QA config knowledge base: %w", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return service.QAConfigVersion{}, err
	}
	return r.getQAConfigVersion(ctx, id, false)
}

func (r *Postgres) GetActiveLLMConfigVersion(ctx context.Context) (service.LLMConfigVersion, error) {
	return r.getLLMConfigVersion(ctx, "", true)
}
func (r *Postgres) getLLMConfigVersion(ctx context.Context, id string, active bool) (service.LLMConfigVersion, error) {
	query := `SELECT id::text,version_no,provider,COALESCE(profile_id,''),model_name,timeout_seconds,temperature,max_tokens,is_active,created_at FROM llm_config_versions WHERE `
	args := []any{}
	if active {
		query += `is_active=true ORDER BY version_no DESC LIMIT 1`
	} else {
		query += `id=$1`
		args = append(args, id)
	}
	var v service.LLMConfigVersion
	err := r.pool.QueryRow(ctx, query, args...).Scan(&v.ID, &v.VersionNo, &v.Provider, &v.ProfileID, &v.ModelName, &v.TimeoutSeconds, &v.Temperature, &v.MaxTokens, &v.IsActive, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, service.NewError(service.CodeNotFound, "LLM configuration not found", err)
	}
	if err != nil {
		return v, fmt.Errorf("get LLM config: %w", err)
	}
	return v, nil
}
func (r *Postgres) CreateLLMConfigVersionResource(ctx context.Context, userID string, input service.CreateLLMConfigVersionInput) (service.LLMConfigVersion, error) {
	if input.TimeoutSeconds == 0 {
		input.TimeoutSeconds = 60
	}
	if input.MaxTokens == 0 {
		input.MaxTokens = 4096
	}
	activate := input.Activate == nil || *input.Activate
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return service.LLMConfigVersion{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `LOCK TABLE llm_config_versions IN EXCLUSIVE MODE`); err != nil {
		return service.LLMConfigVersion{}, err
	}
	if activate {
		if _, err = tx.Exec(ctx, `UPDATE llm_config_versions SET is_active=false WHERE is_active=true`); err != nil {
			return service.LLMConfigVersion{}, err
		}
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO llm_config_versions(version_no,provider,profile_id,model_name,timeout_seconds,temperature,max_tokens,is_active,created_by_user_id) VALUES((SELECT COALESCE(MAX(version_no),0)+1 FROM llm_config_versions),$1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, input.Provider, input.ProfileID, input.ModelName, input.TimeoutSeconds, input.Temperature, input.MaxTokens, activate, userID).Scan(&id)
	if err != nil {
		return service.LLMConfigVersion{}, fmt.Errorf("insert LLM config: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return service.LLMConfigVersion{}, err
	}
	return r.getLLMConfigVersion(ctx, id, false)
}
func (r *Postgres) SaveLLMConnectionTest(ctx context.Context, userID string, result service.LLMProfileTestResult) (service.LLMProfileTestResult, error) {
	_, err := r.pool.Exec(ctx, `INSERT INTO llm_connection_tests(id,external_user_id,success,latency_ms,model_name,error_code,error_message,tested_at) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),$8)`, result.ID, userID, result.Success, result.LatencyMS, result.ModelName, result.ErrorCode, result.ErrorMessage, result.TestedAt)
	if err != nil {
		return service.LLMProfileTestResult{}, fmt.Errorf("save LLM connection test: %w", err)
	}
	return result, nil
}

func (r *Postgres) SaveRetrievalTestRun(ctx context.Context, userID string, input service.RetrievalTestInput, results []service.RetrievalTestResult, duration time.Duration, runErr error) (service.RetrievalTestRun, error) {
	status := "completed"
	errorMessage := ""
	if runErr != nil {
		status = "failed"
		errorMessage = "knowledge retrieval failed"
	}
	overrides, _ := json.Marshal(input.Retrieval)
	var run service.RetrievalTestRun
	err := r.pool.QueryRow(ctx, `INSERT INTO retrieval_test_runs(qa_config_version_id,external_user_id,query,overrides,status,result_count,latency_ms,error_message,completed_at) VALUES(NULLIF($1,'')::uuid,$2,$3,$4,$5,$6,$7,NULLIF($8,''),now()) RETURNING id::text,query,status,COALESCE(result_count,0),COALESCE(latency_ms,0),COALESCE(error_message,''),created_at,completed_at`, input.QAConfigVersionID, userID, input.Question, overrides, status, len(results), duration.Milliseconds(), errorMessage).Scan(&run.ID, &run.Question, &run.Status, &run.ResultCount, &run.LatencyMS, &run.ErrorMessage, &run.CreatedAt, &run.FinishedAt)
	if err != nil {
		return run, fmt.Errorf("save retrieval test run: %w", err)
	}
	run.Query = run.Question
	for i, item := range results {
		item = retrievalResultWithAliases(item)
		results[i] = item
		metadata, _ := json.Marshal(retrievalResultSnapshotMetadata(item))
		_, err = r.pool.Exec(ctx, `INSERT INTO retrieval_test_results(test_run_id,rank_no,external_kb_id,external_doc_id,external_chunk_id,doc_name,text_snapshot,vector_score,rerank_score,metadata) VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,$9,$10)`, run.ID, i+1, item.KnowledgeBaseID, item.DocumentID, item.ChunkID, item.DocumentName, item.ContentPreview, nullableFloat64(item.VectorScore), nullableFloat64(item.RerankScore), metadata)
		if err != nil {
			return run, fmt.Errorf("save retrieval test result: %w", err)
		}
	}
	run.Results = results
	return run, nil
}
func (r *Postgres) GetRetrievalTestRun(ctx context.Context, userID, id string) (service.RetrievalTestRun, error) {
	var run service.RetrievalTestRun
	err := r.pool.QueryRow(ctx, `SELECT id::text,query,status,COALESCE(result_count,0),COALESCE(latency_ms,0),COALESCE(error_message,''),created_at,completed_at FROM retrieval_test_runs WHERE id::text=$1 AND external_user_id=$2`, id, userID).Scan(&run.ID, &run.Question, &run.Status, &run.ResultCount, &run.LatencyMS, &run.ErrorMessage, &run.CreatedAt, &run.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return run, service.NewError(service.CodeNotFound, "retrieval test run not found", err)
	}
	if err != nil {
		return run, fmt.Errorf("get retrieval test run: %w", err)
	}
	run.Query = run.Question
	rows, err := r.pool.Query(ctx, `SELECT rank_no,COALESCE(external_kb_id,''),COALESCE(external_doc_id,''),COALESCE(doc_name,''),COALESCE(external_chunk_id,''),vector_score,rerank_score,COALESCE(text_snapshot,''),metadata FROM retrieval_test_results WHERE test_run_id=$1 ORDER BY rank_no`, id)
	if err != nil {
		return run, err
	}
	defer rows.Close()
	run.Results = []service.RetrievalTestResult{}
	for rows.Next() {
		var item service.RetrievalTestResult
		var vectorScore sql.NullFloat64
		var rerankScore sql.NullFloat64
		var metadata []byte
		if err := rows.Scan(&item.RankNo, &item.KnowledgeBaseID, &item.DocumentID, &item.DocumentName, &item.ChunkID, &vectorScore, &rerankScore, &item.ContentPreview, &metadata); err != nil {
			return run, err
		}
		if vectorScore.Valid {
			item.VectorScore = &vectorScore.Float64
		}
		if rerankScore.Valid {
			item.RerankScore = &rerankScore.Float64
		}
		_ = json.Unmarshal(metadata, &item.Metadata)
		applyRetrievalResultSnapshotMetadata(&item)
		run.Results = append(run.Results, retrievalResultWithAliases(item))
	}
	return run, rows.Err()
}

func retrievalResultSnapshotMetadata(item service.RetrievalTestResult) map[string]any {
	metadata := make(map[string]any, len(item.Metadata)+2)
	for key, value := range item.Metadata {
		metadata[key] = value
	}
	metadata["score"] = item.Score
	if item.SectionPath != "" {
		metadata["sectionPath"] = item.SectionPath
	}
	return metadata
}

func nullableFloat64(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func applyRetrievalResultSnapshotMetadata(item *service.RetrievalTestResult) {
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	if score, ok := floatFromMetadata(item.Metadata["score"]); ok {
		item.Score = score
	} else if item.RerankScore != nil {
		item.Score = *item.RerankScore
	} else if item.VectorScore != nil {
		item.Score = *item.VectorScore
	}
	if item.SectionPath == "" {
		if sectionPath, ok := item.Metadata["sectionPath"].(string); ok {
			item.SectionPath = sectionPath
		}
	}
}

func floatFromMetadata(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		parsed, err := v.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(v, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func retrievalResultWithAliases(item service.RetrievalTestResult) service.RetrievalTestResult {
	if item.DocumentID == "" {
		item.DocumentID = item.DocID
	}
	if item.DocID == "" {
		item.DocID = item.DocumentID
	}
	if item.DocumentName == "" {
		item.DocumentName = item.DocName
	}
	if item.DocName == "" {
		item.DocName = item.DocumentName
	}
	if item.ContentPreview == "" {
		item.ContentPreview = item.Text
	}
	if item.Text == "" {
		item.Text = item.ContentPreview
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	return item
}

func (r *Postgres) GetMetricsOverview(ctx context.Context, days int) (service.MetricsOverview, error) {
	if days <= 0 {
		days = 1
	}
	var v service.MetricsOverview
	err := r.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM messages WHERE role='user'),(SELECT count(*) FROM messages WHERE role='user' AND created_at>=current_date),(SELECT count(*) FROM conversations WHERE deleted_at IS NULL),(SELECT COALESCE(avg(latency_ms),0)::bigint FROM response_runs WHERE started_at>=now()-make_interval(days=>$1)),(SELECT count(DISTINCT external_user_id) FROM conversations WHERE created_at>=current_date)`, days).Scan(&v.TotalQACount, &v.TodayQACount, &v.ConversationCount, &v.AvgLatencyMS, &v.ActiveUsersToday)
	v.TotalQuestionCount = v.TotalQACount
	if err != nil {
		return v, fmt.Errorf("get QA metrics overview: %w", err)
	}
	return v, nil
}
func (r *Postgres) GetMetricsTrend(ctx context.Context, days int) (service.MetricsTrend, error) {
	if days <= 0 {
		days = 30
	}
	rows, err := r.pool.Query(ctx, `WITH dates AS(SELECT generate_series(current_date-($1-1),current_date,'1 day')::date d) SELECT d::text,count(m.id) FROM dates LEFT JOIN messages m ON m.role='user' AND m.created_at>=d AND m.created_at<d+1 GROUP BY d ORDER BY d`, days)
	if err != nil {
		return service.MetricsTrend{}, err
	}
	defer rows.Close()
	v := service.MetricsTrend{Days: days, Points: []service.MetricsTrendPoint{}}
	for rows.Next() {
		var p service.MetricsTrendPoint
		if err := rows.Scan(&p.Date, &p.Count); err != nil {
			return v, err
		}
		p.QuestionCount = p.Count
		v.Points = append(v.Points, p)
	}
	return v, rows.Err()
}
func (r *Postgres) GetTopQueries(ctx context.Context, days, limit int) ([]service.TopQuery, error) {
	if days <= 0 {
		days = 7
	}
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.pool.Query(ctx, `SELECT cb.content,count(*),COALESCE(avg(rr.latency_ms),0)::bigint,max(m.created_at) FROM messages m JOIN message_content_blocks cb ON cb.message_id=m.id AND cb.block_order=0 LEFT JOIN response_runs rr ON rr.user_message_id=m.id WHERE m.role='user' AND m.created_at>=now()-make_interval(days=>$1) GROUP BY cb.content ORDER BY count(*) DESC,max(m.created_at) DESC LIMIT $2`, days, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []service.TopQuery{}
	for rows.Next() {
		var v service.TopQuery
		if err := rows.Scan(&v.Query, &v.Count, &v.AvgLatencyMS, &v.LastAskedAt); err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, rows.Err()
}
func (r *Postgres) GetIntentDistribution(ctx context.Context, days int) ([]service.IntentDistribution, error) {
	if days <= 0 {
		days = 7
	}
	rows, err := r.pool.Query(ctx, `SELECT COALESCE(intent,'unknown'),count(*) FROM messages WHERE role='user' AND created_at>=now()-make_interval(days=>$1) GROUP BY COALESCE(intent,'unknown') ORDER BY count(*) DESC`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []service.IntentDistribution{}
	total := 0
	for rows.Next() {
		var v service.IntentDistribution
		if err := rows.Scan(&v.Intent, &v.Count); err != nil {
			return nil, err
		}
		v.Label = intentLabel(v.Intent)
		total += v.Count
		items = append(items, v)
	}
	for i := range items {
		if total > 0 {
			items[i].Percent = math.Round(float64(items[i].Count)*1000/float64(total)) / 10
		}
	}
	return items, rows.Err()
}
func intentLabel(value string) string {
	switch value {
	case "knowledge_qa":
		return "知识问答"
	case "general_chat":
		return "一般对话"
	case "report_generation":
		return "报告生成"
	case "data_analysis":
		return "数据分析"
	default:
		return "未知"
	}
}
