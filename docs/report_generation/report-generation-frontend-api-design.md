# 报告生成模块前端 API 接口设计

## 1. 文档说明

本文基于最新版《报告生成接口文档 api_interfaces(1).md》和当前《报告生成模块前端原型-大项目集成版.html》整理，用于指导前端对接 gateway RESTful API。

结论：当前前端业务流程与最新版后端接口不冲突。此前冲突主要是旧动作路径命名，如 `/outline/generate`、`/content/generate`、`/exports`、`/generation-tasks` 等。前端实现应全部改用最新版 RESTful 资源路径。

## 2. 总体约定

### 2.1 Base URL

```text
/api/v1
```

前端只调用 gateway，不直接调用 `document` 服务内部地址。

### 2.2 请求头

| Header | 必填 | 来源 | 说明 |
|---|---:|---|---|
| `Authorization` | 是 | 大项目登录态 | `Bearer <accessToken>` |
| `X-Request-Id` | 否 | 前端生成或 gateway 生成 | 链路追踪 ID |

### 2.3 响应 envelope

单资源：

```ts
type ApiResponse<T> = {
  data: T;
  requestId: string;
};
```

分页：

```ts
type PageResponse<T> = {
  data: T[];
  page: {
    page: number;
    pageSize: number;
    total: number;
  };
  requestId: string;
};
```

错误：

```ts
type ApiErrorResponse = {
  error: {
    code:
      | "validation_error"
      | "unauthorized"
      | "forbidden"
      | "not_found"
      | "conflict"
      | "rate_limited"
      | "dependency_error"
      | "internal_error";
    message: string;
    requestId: string;
    fields?: Record<string, string>;
  };
};
```

### 2.4 前端 API 层建议

建议在 React 项目中按 feature-first 组织：

```text
src/features/report-generation/
  api/
    reportApi.ts
    reportTemplateApi.ts
    reportMaterialApi.ts
    reportJobApi.ts
    reportFileApi.ts
    reportStatisticsApi.ts
    types.ts
  hooks/
    useReportDraft.ts
    useReportJobs.ts
    useReportRecords.ts
  pages/
  components/
```

浏览器代码通过统一 `gatewayClient` 发请求，组件不直接拼 fetch。

## 3. 页面与接口映射

| 前端页面/区域 | 用户动作 | 前端 API 函数 | 后端接口 |
|---|---|---|---|
| 报告生成-步骤 1 | 查询报告类型 | `listReportTypes()` | `GET /report-types` |
| 报告生成-步骤 1 | 查询可用模板 | `listReportTemplates(params)` | `GET /report-templates` |
| 报告生成-步骤 1 | 查询可用素材 | `listReportMaterials(params)` | `GET /report-materials` |
| 报告生成-步骤 1 | 保存报告草稿 | `createReportDraft(payload)` | `POST /reports` |
| 报告生成-步骤 1 | 生成大纲 | `createReportJob(reportId, payload)` | `POST /reports/{reportId}/jobs`，`jobType=outline_generation` |
| 报告生成-步骤 2 | 查询大纲版本 | `listReportOutlines(reportId)` | `GET /reports/{reportId}/outlines` |
| 报告生成-步骤 2 | 保存大纲章节树 | `updateReportOutline(reportId, outlineId, payload)` | `PATCH /reports/{reportId}/outlines/{outlineId}` |
| 报告生成-步骤 2 | 删除大纲章节 | `deleteOutlineSection(reportId, outlineId, sectionId)` | `DELETE /reports/{reportId}/outlines/{outlineId}/sections/{sectionId}` |
| 报告生成-步骤 2 | 重新生成大纲 | `createReportJob(reportId, payload)` | `POST /reports/{reportId}/jobs`，`jobType=outline_regeneration` |
| 报告生成-步骤 3 | 生成完整报告 | `createReportJob(reportId, payload)` | `POST /reports/{reportId}/jobs`，`jobType=content_generation` |
| 报告生成-步骤 3 | 查询生成任务 | `getReportJob(jobId)` | `GET /report-jobs/{jobId}` |
| 报告生成-步骤 3 | 重试失败任务 | `createReportJobAttempt(jobId)` | `POST /report-jobs/{jobId}/attempts` |
| 正文编辑 | 查询章节正文 | `listReportSections(reportId)` | `GET /reports/{reportId}/sections` |
| 正文编辑 | 保存正文/表格 | `updateReportSection(reportId, sectionId, payload)` | `PATCH /reports/{reportId}/sections/{sectionId}` |
| 正文编辑 | 单章节重新生成 | `createSectionVersion(reportId, sectionId, payload)` 或 `createReportJob()` | `POST /reports/{reportId}/sections/{sectionId}/versions` 或 `POST /reports/{reportId}/jobs` |
| 导出 | 创建 DOCX 文件 | `createReportFile(payload)` | `POST /report-files` |
| 导出 | 查询文件元数据 | `getReportFile(reportFileId)` | `GET /report-files/{reportFileId}` |
| 导出 | 下载文件内容 | `downloadReportFileContent(reportFileId)` | `GET /report-files/{reportFileId}/content` |
| 报告记录 | 分页查询报告 | `listReports(params)` | `GET /reports` |
| 报告记录 | 删除报告 | `deleteReport(reportId)` | `DELETE /reports/{reportId}` |
| 模板管理 | 上传模板 | `uploadReportTemplate(formData)` | `POST /report-templates` |
| 模板管理 | 更新/删除模板 | `updateReportTemplate()` / `deleteReportTemplate()` | `PATCH/DELETE /report-templates/{reportTemplateId}` |
| 模板可视化编辑器 | 查询/保存结构 | `getReportTemplateStructure()` / `updateReportTemplateStructure()` | `GET/PATCH /report-templates/{reportTemplateId}/structure` |
| 素材管理 | 上传/删除素材 | `uploadReportMaterial()` / `deleteReportMaterial()` | `POST/DELETE /report-materials` |
| 统计监控 | 统计概览 | `getReportStatisticsOverview()` | `GET /report-statistics/overview` |
| 统计监控 | 30 天趋势 | `getReportStatisticsDaily({ days: 30 })` | `GET /report-statistics/daily?days=30` |
| 操作日志 | 查询日志 | `listReportOperationLogs(params)` | `GET /report-operation-logs` |

## 4. 核心类型设计

```ts
export type ReportTypeCode = "summer_peak_inspection" | "coal_inventory_audit";

export type ReportStatus =
  | "draft"
  | "outline_generating"
  | "outline_generated"
  | "content_generating"
  | "generated"
  | "exported"
  | "failed"
  | "deleted";

export type ReportJobType =
  | "outline_generation"
  | "outline_regeneration"
  | "content_generation"
  | "content_regeneration"
  | "section_regeneration"
  | "report_file_creation";

export type ReportJobStatus =
  | "pending"
  | "running"
  | "succeeded"
  | "partial_succeeded"
  | "failed"
  | "canceled";

export type ReportType = {
  code: ReportTypeCode;
  name: string;
  description: string;
  enabled: boolean;
};

export type ReportTemplate = {
  id: string;
  templateName: string;
  reportType: ReportTypeCode;
  description?: string;
  enabled: boolean;
  version?: number;
  updatedAt?: string;
};

export type ReportMaterial = {
  id: string;
  materialName: string;
  category?: string;
  description?: string;
  enabled?: boolean;
  updatedAt?: string;
};

export type Report = {
  id: string;
  name: string;
  reportType: ReportTypeCode;
  templateId: string;
  topic: string;
  specialty?: string;
  businessObject?: string;
  year: number;
  status: ReportStatus | string;
  createdAt?: string;
  updatedAt?: string;
  latestJobId?: string;
  latestReportFileId?: string;
};

export type OutlineSection = {
  id?: string;
  clientSectionId?: string;
  title: string;
  level: number;
  numbering?: string;
  sortOrder?: number;
  requirements?: string;
  children?: OutlineSection[];
};

export type ReportOutline = {
  id: string;
  reportId: string;
  source: "manual" | "ai";
  sections: OutlineSection[];
  version?: number;
  createdAt?: string;
  updatedAt?: string;
};

export type ReportSection = {
  id: string;
  reportId: string;
  title: string;
  numbering?: string;
  content: string;
  tables: unknown[];
  generationStatus?: ReportJobStatus;
  updatedAt?: string;
};

export type ReportJob = {
  id: string;
  reportId: string;
  jobType: ReportJobType;
  status: ReportJobStatus;
  progress?: {
    completedSections?: number;
    totalSections?: number;
    percent?: number;
  };
  error?: {
    code: string;
    message: string;
  } | null;
  createdAt?: string;
  updatedAt?: string;
};

export type ReportFile = {
  id: string;
  reportId: string;
  format: "docx";
  fileName?: string;
  status?: "pending" | "running" | "succeeded" | "failed";
  jobId?: string;
  createdAt?: string;
};
```

## 5. API 函数设计

### 5.1 报告类型、模板、素材

```ts
export function listReportTypes(): Promise<ApiResponse<ReportType[]>>;

export function listReportTemplates(params?: {
  page?: number;
  pageSize?: number;
  reportType?: ReportTypeCode;
  enabled?: boolean;
}): Promise<PageResponse<ReportTemplate>>;

export function getReportTemplate(reportTemplateId: string): Promise<ApiResponse<ReportTemplate>>;

export function uploadReportTemplate(formData: FormData): Promise<ApiResponse<ReportTemplate>>;

export function updateReportTemplate(
  reportTemplateId: string,
  payload: Partial<Pick<ReportTemplate, "templateName" | "description" | "enabled">>
): Promise<ApiResponse<ReportTemplate>>;

export function deleteReportTemplate(reportTemplateId: string): Promise<ApiResponse<{ id: string }>>;

export function getReportTemplateStructure(
  reportTemplateId: string
): Promise<ApiResponse<{ outlineSchema: OutlineSection[]; styleConfig: Record<string, unknown> }>>;

export function updateReportTemplateStructure(
  reportTemplateId: string,
  payload: { outlineSchema: OutlineSection[]; styleConfig: Record<string, unknown> }
): Promise<ApiResponse<{ id: string }>>;

export function listReportMaterials(params?: {
  page?: number;
  pageSize?: number;
  category?: string;
  enabled?: boolean;
}): Promise<PageResponse<ReportMaterial>>;

export function uploadReportMaterial(formData: FormData): Promise<ApiResponse<ReportMaterial>>;

export function deleteReportMaterial(materialId: string): Promise<ApiResponse<{ id: string }>>;
```

### 5.2 报告草稿与记录

```ts
export type CreateReportDraftPayload = {
  name: string;
  reportType: ReportTypeCode;
  templateId: string;
  topic: string;
  specialty?: string;
  businessObject?: string;
  year: number;
  extraContext?: Record<string, unknown>;
};

export function createReportDraft(
  payload: CreateReportDraftPayload
): Promise<ApiResponse<{ id: string; status: ReportStatus }>>;

export function listReports(params?: {
  page?: number;
  pageSize?: number;
  reportType?: ReportTypeCode;
  status?: ReportStatus | string;
  year?: number;
  name?: string;
}): Promise<PageResponse<Report>>;

export function getReport(reportId: string): Promise<ApiResponse<Report>>;

export function updateReport(
  reportId: string,
  payload: Partial<CreateReportDraftPayload>
): Promise<ApiResponse<Report>>;

export function deleteReport(reportId: string): Promise<ApiResponse<{ id: string }>>;
```

### 5.3 大纲、章节与正文

```ts
export function listReportOutlines(reportId: string): Promise<ApiResponse<ReportOutline[]>>;

export function createReportOutline(
  reportId: string,
  payload: { source: "manual"; sections: OutlineSection[] }
): Promise<ApiResponse<ReportOutline>>;

export function getReportOutline(
  reportId: string,
  outlineId: string
): Promise<ApiResponse<ReportOutline>>;

export function updateReportOutline(
  reportId: string,
  outlineId: string,
  payload: { sections: OutlineSection[] }
): Promise<ApiResponse<ReportOutline>>;

export function deleteOutlineSection(
  reportId: string,
  outlineId: string,
  sectionId: string
): Promise<ApiResponse<{ id: string }>>;

export function listReportSections(reportId: string): Promise<ApiResponse<ReportSection[]>>;

export function getReportSection(
  reportId: string,
  sectionId: string
): Promise<ApiResponse<ReportSection>>;

export function updateReportSection(
  reportId: string,
  sectionId: string,
  payload: { content: string; tables: unknown[] }
): Promise<ApiResponse<ReportSection>>;

export function createSectionVersion(
  reportId: string,
  sectionId: string,
  payload: {
    source: "ai" | "manual";
    requirements?: string;
    materialIds?: string[];
    preserveManualEdits?: boolean;
  }
): Promise<ApiResponse<ReportSection>>;
```

### 5.4 生成任务与重试

```ts
export type CreateReportJobPayload = {
  jobType: ReportJobType;
  target: {
    scope: "report" | "section";
    sectionId?: string | null;
  };
  requirements?: string;
  materialIds?: string[];
  options?: {
    preserveManualEdits?: boolean;
    saveResult?: boolean;
  };
};

export function createReportJob(
  reportId: string,
  payload: CreateReportJobPayload
): Promise<ApiResponse<ReportJob>>;

export function getReportJob(jobId: string): Promise<ApiResponse<ReportJob>>;

export function listReportJobs(reportId: string): Promise<ApiResponse<ReportJob[]>>;

export function listReportJobAttempts(jobId: string): Promise<ApiResponse<unknown[]>>;

export function createReportJobAttempt(jobId: string): Promise<ApiResponse<ReportJob>>;

export function listReportEvents(reportId: string): Promise<ApiResponse<unknown[]>>;
```

### 5.5 报告文件与下载

```ts
export function createReportFile(payload: {
  reportId: string;
  format: "docx";
  templateId: string;
  styleOptions?: {
    numberingMode?: "global" | string;
  };
}): Promise<ApiResponse<ReportFile>>;

export function listReportFiles(params?: {
  page?: number;
  pageSize?: number;
  reportId?: string;
}): Promise<PageResponse<ReportFile>>;

export function getReportFile(reportFileId: string): Promise<ApiResponse<ReportFile>>;

export function downloadReportFileContent(reportFileId: string): Promise<Blob>;
```

`GET /report-files/{reportFileId}/content` 可能返回二进制文件流，不按普通 JSON envelope 解析；失败时仍按统一错误结构处理。

### 5.6 统计与日志

```ts
export function getReportStatisticsOverview(): Promise<ApiResponse<{
  reportCount: number;
  templateCount: number;
  materialCount?: number;
  jobs?: {
    pending?: number;
    running?: number;
    succeeded?: number;
    partialSucceeded?: number;
    failed?: number;
    canceled?: number;
  };
}>>;

export function getReportStatisticsDaily(params?: {
  days?: number;
}): Promise<ApiResponse<Array<{ date: string; count: number }>>>;

export function listReportOperationLogs(params?: {
  page?: number;
  pageSize?: number;
  targetType?: "report" | "template" | "material" | "reportFile" | string;
  targetId?: string;
}): Promise<PageResponse<unknown>>;
```

## 6. 关键流程设计

### 6.1 创建报告并生成大纲

1. `GET /report-types`
2. `GET /report-templates?reportType=...&enabled=true`
3. `GET /report-materials`
4. `POST /reports` 创建草稿
5. `POST /reports/{reportId}/jobs`，`jobType=outline_generation`
6. 轮询 `GET /report-jobs/{jobId}`
7. 成功后 `GET /reports/{reportId}/outlines` 获取最新大纲版本

### 6.2 编辑大纲章节

1. 用户新增、删除、上移、下移、改标题、改层级。
2. 保存整棵大纲树：`PATCH /reports/{reportId}/outlines/{outlineId}`。
3. 删除指定章节也可调用：`DELETE /reports/{reportId}/outlines/{outlineId}/sections/{sectionId}`。
4. 前端保存成功后重新读取 `GET /reports/{reportId}/outlines/{outlineId}`，避免本地排序与后端编号不一致。

### 6.3 生成完整报告

1. 确认当前大纲已保存。
2. `POST /reports/{reportId}/jobs`，`jobType=content_generation`。
3. 轮询 `GET /report-jobs/{jobId}` 或 `GET /reports/{reportId}/events`。
4. 展示 `completedSections / totalSections / percent`。
5. 成功后 `GET /reports/{reportId}/sections` 获取章节正文和表格。
6. 若 `partial_succeeded`，保留已生成章节，失败章节显示重试入口。

### 6.4 编辑正文和表格

1. 查询章节：`GET /reports/{reportId}/sections/{sectionId}`。
2. 保存章节：`PATCH /reports/{reportId}/sections/{sectionId}`。
3. 单章节 AI 重新生成：优先使用 `POST /reports/{reportId}/sections/{sectionId}/versions`；如果后端统一走任务，也可使用 `POST /reports/{reportId}/jobs`，`jobType=section_regeneration`。

### 6.5 DOCX 导出和下载

1. 确认章节正文已保存。
2. `POST /report-files` 创建文件。
3. 若返回 `status=pending` 或 `jobId`，轮询文件元数据或任务状态。
4. 成功后 `GET /report-files/{reportFileId}/content` 下载文件。
5. 重新导出只调用 `POST /report-files`，不重新调用 AI 生成任务。

## 7. 权限与角色边界

普通用户：

- 可创建报告、生成大纲、编辑大纲、生成正文、编辑正文、导出 DOCX、查看/删除自己的报告记录。
- 可选择已启用模板。
- 可引用已发布专业素材。

管理员/超级管理员：

- 可上传、查看、编辑、删除或停用模板。
- 可使用模板可视化编辑器维护 `outlineSchema` 和 `styleConfig`。
- 可上传、查看、删除专业素材。
- 可查看统计、任务、操作日志和 requestId 诊断信息。

权限不足时，后端返回 `403 forbidden`。前端应展示无权限提示，并隐藏高风险操作入口。

## 8. 状态处理

### 8.1 任务状态到 UI 的映射

| 后端状态 | 前端展示 | 操作 |
|---|---|---|
| `pending` | 等待中 | 禁用重复提交 |
| `running` | 生成中 | 展示进度 |
| `succeeded` | 已完成 | 允许进入下一步 |
| `partial_succeeded` | 部分成功 | 展示失败章节和重试入口 |
| `failed` | 失败 | 展示错误原因和重试入口 |
| `canceled` | 已取消 | 允许重新创建任务 |

### 8.2 错误处理

- 401：跳转或提示重新登录，由大项目统一处理。
- 403：显示“当前角色无权限访问该功能”。
- 409：提示当前状态不允许操作，例如未保存大纲时生成正文。
- 502：展示 AI、MinIO 或下游依赖失败，并允许重试任务。
- 同步创建任务失败：读取 `error.message` 和 `error.fields`。
- 长任务失败：读取 `ReportJob.error`。

## 9. 旧路径替换表

| 旧前端/旧文档写法 | 最新接口写法 |
|---|---|
| `POST /reports/{id}/outline/generate` | `POST /reports/{reportId}/jobs` + `jobType=outline_generation` |
| `POST /reports/{id}/outline/regenerate` | `POST /reports/{reportId}/jobs` + `jobType=outline_regeneration` |
| `POST /reports/{id}/content/generate` | `POST /reports/{reportId}/jobs` + `jobType=content_generation` |
| `POST /reports/{id}/content/regenerate` | `POST /reports/{reportId}/jobs` + `jobType=content_regeneration` |
| `POST /reports/{id}/sections/{sectionId}/regenerate` | `POST /reports/{reportId}/sections/{sectionId}/versions` 或 `POST /reports/{reportId}/jobs` + `jobType=section_regeneration` |
| `GET /generation-tasks/{taskId}` | `GET /report-jobs/{jobId}` |
| `POST /generation-tasks/{taskId}/retry` | `POST /report-jobs/{jobId}/attempts` |
| `POST /reports/{id}/exports` | `POST /report-files` |
| `GET /exports/{fileId}` | `GET /report-files/{reportFileId}` |
| `GET /exports/{fileId}/file` | `GET /report-files/{reportFileId}/content` |
| `GET /templates` | `GET /report-templates` |
| `GET /materials` | `GET /report-materials` |
| `GET /statistics/overview` | `GET /report-statistics/overview` |
| `GET /statistics/report-generation-trend` | `GET /report-statistics/daily?days=30` |

## 10. 验收清单

- [ ] 前端 API 路径不出现 `generate`、`regenerate`、`export`、`retry`、`download` 等动作词。
- [ ] 所有 JSON 响应按 `data/requestId` 或分页 envelope 解析。
- [ ] 所有公开字段使用 camelCase，例如 `requestId`、`reportType`、`templateId`、`businessObject`。
- [ ] 生成大纲、生成全文、导出 DOCX、重试失败任务均创建资源或任务。
- [ ] 普通用户不显示模板/素材上传和删除入口。
- [ ] 管理员入口支持模板、素材、统计、日志和接口诊断。
- [ ] 文件下载不暴露 MinIO object key 或内部 URL。
- [ ] 任务失败和部分成功能保留已生成内容，并允许重试。
