package httpapi

import "strings"

type routeSpec struct {
	Method            string
	Pattern           string
	Owner             string
	OperationID       string
	DownstreamPattern string
	NotImplemented    bool
	StreamResponse    bool
	AdminPermissions  []string
}

var modelProfileAdminPermissions = []string{"system:admin", "admin:model-profile:write"}
var parserConfigAdminPermissions = []string{"system:admin", "knowledge:admin", "admin:parser-config:write"}

var activeProxyRoutes = []routeSpec{
	{Method: "GET", Pattern: "/api/v1/knowledge-bases", Owner: "knowledge", OperationID: "listKnowledgeBases"},
	{Method: "POST", Pattern: "/api/v1/knowledge-bases", Owner: "knowledge", OperationID: "createKnowledgeBase"},
	{Method: "GET", Pattern: "/api/v1/knowledge-bases/{knowledgeBaseId}", Owner: "knowledge", OperationID: "getKnowledgeBase"},
	{Method: "PATCH", Pattern: "/api/v1/knowledge-bases/{knowledgeBaseId}", Owner: "knowledge", OperationID: "updateKnowledgeBase"},
	{Method: "DELETE", Pattern: "/api/v1/knowledge-bases/{knowledgeBaseId}", Owner: "knowledge", OperationID: "deleteKnowledgeBase"},
	{Method: "GET", Pattern: "/api/v1/knowledge-bases/{knowledgeBaseId}/documents", Owner: "knowledge", OperationID: "listKnowledgeBaseDocuments"},
	{Method: "POST", Pattern: "/api/v1/knowledge-bases/{knowledgeBaseId}/documents", Owner: "knowledge", OperationID: "uploadKnowledgeBaseDocument"},
	{Method: "GET", Pattern: "/api/v1/documents/{documentId}", Owner: "knowledge", OperationID: "getDocument"},
	{Method: "PATCH", Pattern: "/api/v1/documents/{documentId}", Owner: "knowledge", OperationID: "updateDocument", NotImplemented: true},
	{Method: "DELETE", Pattern: "/api/v1/documents/{documentId}", Owner: "knowledge", OperationID: "deleteDocument", NotImplemented: true},
	{Method: "GET", Pattern: "/api/v1/documents/{documentId}/chunks", Owner: "knowledge", OperationID: "listDocumentChunks"},
	{Method: "GET", Pattern: "/api/v1/documents/{documentId}/content", Owner: "knowledge", OperationID: "getDocumentContent"},
	{Method: "POST", Pattern: "/api/v1/knowledge-queries", Owner: "knowledge", OperationID: "createKnowledgeQuery", DownstreamPattern: "/internal/v1/knowledge-queries"},
	{Method: "GET", Pattern: "/api/v1/admin/model-profiles", Owner: "ai-gateway", OperationID: "listAdminModelProfiles", DownstreamPattern: "/internal/v1/model-profiles", AdminPermissions: modelProfileAdminPermissions},
	{Method: "POST", Pattern: "/api/v1/admin/model-profiles", Owner: "ai-gateway", OperationID: "createAdminModelProfile", DownstreamPattern: "/internal/v1/model-profiles", AdminPermissions: modelProfileAdminPermissions},
	{Method: "GET", Pattern: "/api/v1/admin/model-profiles/{profileId}", Owner: "ai-gateway", OperationID: "getAdminModelProfile", DownstreamPattern: "/internal/v1/model-profiles/{profileId}", AdminPermissions: modelProfileAdminPermissions},
	{Method: "PATCH", Pattern: "/api/v1/admin/model-profiles/{profileId}", Owner: "ai-gateway", OperationID: "updateAdminModelProfile", DownstreamPattern: "/internal/v1/model-profiles/{profileId}", AdminPermissions: modelProfileAdminPermissions},
	{Method: "DELETE", Pattern: "/api/v1/admin/model-profiles/{profileId}", Owner: "ai-gateway", OperationID: "deleteAdminModelProfile", DownstreamPattern: "/internal/v1/model-profiles/{profileId}", AdminPermissions: modelProfileAdminPermissions},
	{Method: "GET", Pattern: "/api/v1/admin/parser-configs", Owner: "knowledge", OperationID: "listAdminParserConfigs", DownstreamPattern: "/internal/v1/parser-configs", AdminPermissions: parserConfigAdminPermissions},
	{Method: "POST", Pattern: "/api/v1/admin/parser-configs", Owner: "knowledge", OperationID: "createAdminParserConfig", DownstreamPattern: "/internal/v1/parser-configs", AdminPermissions: parserConfigAdminPermissions},
	{Method: "GET", Pattern: "/api/v1/admin/parser-configs/{parserConfigId}", Owner: "knowledge", OperationID: "getAdminParserConfig", DownstreamPattern: "/internal/v1/parser-configs/{parserConfigId}", AdminPermissions: parserConfigAdminPermissions},
	{Method: "PATCH", Pattern: "/api/v1/admin/parser-configs/{parserConfigId}", Owner: "knowledge", OperationID: "updateAdminParserConfig", DownstreamPattern: "/internal/v1/parser-configs/{parserConfigId}", AdminPermissions: parserConfigAdminPermissions},
	{Method: "DELETE", Pattern: "/api/v1/admin/parser-configs/{parserConfigId}", Owner: "knowledge", OperationID: "deleteAdminParserConfig", DownstreamPattern: "/internal/v1/parser-configs/{parserConfigId}", AdminPermissions: parserConfigAdminPermissions},
	{Method: "GET", Pattern: "/api/v1/report-types", Owner: "document", OperationID: "listReportTypes"},
	{Method: "GET", Pattern: "/api/v1/report-templates", Owner: "document", OperationID: "listReportTemplates"},
	{Method: "POST", Pattern: "/api/v1/report-templates", Owner: "document", OperationID: "createReportTemplate"},
	{Method: "GET", Pattern: "/api/v1/report-templates/{reportTemplateId}", Owner: "document", OperationID: "getReportTemplate"},
	{Method: "PATCH", Pattern: "/api/v1/report-templates/{reportTemplateId}", Owner: "document", OperationID: "updateReportTemplate"},
	{Method: "DELETE", Pattern: "/api/v1/report-templates/{reportTemplateId}", Owner: "document", OperationID: "deleteReportTemplate"},
	{Method: "GET", Pattern: "/api/v1/report-templates/{reportTemplateId}/structure", Owner: "document", OperationID: "getReportTemplateStructure"},
	{Method: "PATCH", Pattern: "/api/v1/report-templates/{reportTemplateId}/structure", Owner: "document", OperationID: "updateReportTemplateStructure"},
	{Method: "GET", Pattern: "/api/v1/report-materials", Owner: "document", OperationID: "listReportMaterials"},
	{Method: "POST", Pattern: "/api/v1/report-materials", Owner: "document", OperationID: "createReportMaterial"},
	{Method: "GET", Pattern: "/api/v1/report-materials/{materialId}", Owner: "document", OperationID: "getReportMaterial"},
	{Method: "DELETE", Pattern: "/api/v1/report-materials/{materialId}", Owner: "document", OperationID: "deleteReportMaterial"},
	{Method: "GET", Pattern: "/api/v1/reports", Owner: "document", OperationID: "listReports"},
	{Method: "POST", Pattern: "/api/v1/reports", Owner: "document", OperationID: "createReport"},
	{Method: "GET", Pattern: "/api/v1/reports/{reportId}", Owner: "document", OperationID: "getReport"},
	{Method: "PATCH", Pattern: "/api/v1/reports/{reportId}", Owner: "document", OperationID: "updateReport"},
	{Method: "DELETE", Pattern: "/api/v1/reports/{reportId}", Owner: "document", OperationID: "deleteReport"},
	{Method: "GET", Pattern: "/api/v1/reports/{reportId}/outlines", Owner: "document", OperationID: "listReportOutlines"},
	{Method: "POST", Pattern: "/api/v1/reports/{reportId}/outlines", Owner: "document", OperationID: "createReportOutline"},
	{Method: "GET", Pattern: "/api/v1/reports/{reportId}/outlines/{outlineId}", Owner: "document", OperationID: "getReportOutline"},
	{Method: "PATCH", Pattern: "/api/v1/reports/{reportId}/outlines/{outlineId}", Owner: "document", OperationID: "updateReportOutline"},
	{Method: "DELETE", Pattern: "/api/v1/reports/{reportId}/outlines/{outlineId}/sections/{sectionId}", Owner: "document", OperationID: "deleteReportOutlineSection"},
	{Method: "GET", Pattern: "/api/v1/reports/{reportId}/sections", Owner: "document", OperationID: "listReportSections"},
	{Method: "POST", Pattern: "/api/v1/reports/{reportId}/sections", Owner: "document", OperationID: "createReportSection"},
	{Method: "GET", Pattern: "/api/v1/reports/{reportId}/sections/{sectionId}", Owner: "document", OperationID: "getReportSection"},
	{Method: "PATCH", Pattern: "/api/v1/reports/{reportId}/sections/{sectionId}", Owner: "document", OperationID: "updateReportSection"},
	{Method: "GET", Pattern: "/api/v1/reports/{reportId}/sections/{sectionId}/versions", Owner: "document", OperationID: "listReportSectionVersions"},
	{Method: "POST", Pattern: "/api/v1/reports/{reportId}/sections/{sectionId}/versions", Owner: "document", OperationID: "createReportSectionVersion"},
	{Method: "GET", Pattern: "/api/v1/reports/{reportId}/jobs", Owner: "document", OperationID: "listReportJobs"},
	{Method: "POST", Pattern: "/api/v1/reports/{reportId}/jobs", Owner: "document", OperationID: "createReportJob"},
	{Method: "GET", Pattern: "/api/v1/report-jobs/{jobId}", Owner: "document", OperationID: "getReportJob"},
	{Method: "GET", Pattern: "/api/v1/report-jobs/{jobId}/attempts", Owner: "document", OperationID: "listReportJobAttempts"},
	{Method: "POST", Pattern: "/api/v1/report-jobs/{jobId}/attempts", Owner: "document", OperationID: "createReportJobAttempt"},
	{Method: "GET", Pattern: "/api/v1/reports/{reportId}/events", Owner: "document", OperationID: "listReportEvents"},
	{Method: "GET", Pattern: "/api/v1/report-files", Owner: "document", OperationID: "listReportFiles"},
	{Method: "POST", Pattern: "/api/v1/report-files", Owner: "document", OperationID: "createReportFile"},
	{Method: "GET", Pattern: "/api/v1/report-files/{reportFileId}", Owner: "document", OperationID: "getReportFile"},
	{Method: "GET", Pattern: "/api/v1/report-files/{reportFileId}/content", Owner: "document", OperationID: "getReportFileContent"},
	{Method: "GET", Pattern: "/api/v1/report-statistics/overview", Owner: "document", OperationID: "getReportStatisticsOverview"},
	{Method: "GET", Pattern: "/api/v1/report-statistics/daily", Owner: "document", OperationID: "listDailyReportStatistics"},
	{Method: "GET", Pattern: "/api/v1/report-operation-logs", Owner: "document", OperationID: "listReportOperationLogs"},
	{Method: "GET", Pattern: "/api/v1/report-settings", Owner: "document", OperationID: "getReportSettings"},
	{Method: "PATCH", Pattern: "/api/v1/report-settings", Owner: "document", OperationID: "updateReportSettings"},
	{Method: "GET", Pattern: "/api/v1/qa-sessions", Owner: "qa", OperationID: "listQASessions"},
	{Method: "POST", Pattern: "/api/v1/qa-sessions", Owner: "qa", OperationID: "createQASession"},
	{Method: "GET", Pattern: "/api/v1/qa-sessions/{sessionId}", Owner: "qa", OperationID: "getQASession"},
	{Method: "PATCH", Pattern: "/api/v1/qa-sessions/{sessionId}", Owner: "qa", OperationID: "updateQASession"},
	{Method: "DELETE", Pattern: "/api/v1/qa-sessions/{sessionId}", Owner: "qa", OperationID: "deleteQASession"},
	{Method: "GET", Pattern: "/api/v1/qa-sessions/{sessionId}/messages", Owner: "qa", OperationID: "listQAMessages"},
	{Method: "POST", Pattern: "/api/v1/qa-sessions/{sessionId}/messages", Owner: "qa", OperationID: "createQAMessage", StreamResponse: true},
	{Method: "GET", Pattern: "/api/v1/qa-sessions/{sessionId}/events", Owner: "qa", OperationID: "listQAStreamEvents"},
	{Method: "GET", Pattern: "/api/v1/response-runs/{responseRunId}", Owner: "qa", OperationID: "getQAResponseRun"},
	{Method: "PATCH", Pattern: "/api/v1/response-runs/{responseRunId}", Owner: "qa", OperationID: "updateQAResponseRun"},
	{Method: "GET", Pattern: "/api/v1/response-runs/{responseRunId}/tool-calls", Owner: "qa", OperationID: "listQAResponseRunToolCalls"},
	{Method: "GET", Pattern: "/api/v1/messages/{messageId}/citations", Owner: "qa", OperationID: "listQAMessageCitations"},
	{Method: "GET", Pattern: "/api/v1/citations/{citationId}", Owner: "qa", OperationID: "getQACitation"},
	{Method: "POST", Pattern: "/api/v1/citation-lookups", Owner: "qa", OperationID: "createQACitationLookup"},
	{Method: "GET", Pattern: "/api/v1/qa-config-versions/current", Owner: "qa", OperationID: "getCurrentQAConfigVersion"},
	{Method: "POST", Pattern: "/api/v1/qa-config-versions", Owner: "qa", OperationID: "createQAConfigVersion"},
	{Method: "GET", Pattern: "/api/v1/llm-config-versions/current", Owner: "qa", OperationID: "getCurrentQALLMConfigVersion"},
	{Method: "POST", Pattern: "/api/v1/llm-config-versions", Owner: "qa", OperationID: "createQALLMConfigVersion"},
	{Method: "POST", Pattern: "/api/v1/llm-connection-tests", Owner: "qa", OperationID: "createQALLMConnectionTest"},
	{Method: "POST", Pattern: "/api/v1/retrieval-test-runs", Owner: "qa", OperationID: "createQARetrievalTestRun"},
	{Method: "GET", Pattern: "/api/v1/retrieval-test-runs/{testRunId}", Owner: "qa", OperationID: "getQARetrievalTestRun"},
	{Method: "GET", Pattern: "/api/v1/qa-metrics/overview", Owner: "qa", OperationID: "getQAMetricsOverview"},
	{Method: "GET", Pattern: "/api/v1/qa-metrics/trend", Owner: "qa", OperationID: "getQAMetricsTrend"},
	{Method: "GET", Pattern: "/api/v1/qa-metrics/top-queries", Owner: "qa", OperationID: "listQATopQueries"},
	{Method: "GET", Pattern: "/api/v1/qa-metrics/intent-distribution", Owner: "qa", OperationID: "listQAIntentDistribution"},
}

var activeDirectRoutes = []routeSpec{
	{Method: "GET", Pattern: "/healthz", Owner: "gateway", OperationID: "getHealthz"},
	{Method: "GET", Pattern: "/readyz", Owner: "gateway", OperationID: "getReadyz"},
	{Method: "POST", Pattern: "/api/v1/users", Owner: "auth", OperationID: "createUser"},
	{Method: "POST", Pattern: "/api/v1/sessions", Owner: "auth", OperationID: "createSession"},
	{Method: "DELETE", Pattern: "/api/v1/sessions/current", Owner: "auth", OperationID: "deleteCurrentSession"},
	{Method: "GET", Pattern: "/api/v1/users/me", Owner: "auth", OperationID: "getCurrentUser"},
}

func activeOperationCount() int {
	return len(activeDirectRoutes) + len(activeProxyRoutes)
}

func (route routeSpec) requiresAdmin() bool {
	return strings.HasPrefix(route.Pattern, "/api/v1/admin/")
}
