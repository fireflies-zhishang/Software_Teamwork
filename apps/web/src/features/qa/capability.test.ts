import { describe, expect, it } from 'vitest'

import { ApiError } from '@/api/client'
import type { QACitation } from '@/lib/types'

import {
  createSafeToolStep,
  formatQAError,
  formatQAStreamError,
  getCitationAvailabilityText,
  getCitationDelta,
  getSafeReasoningStep,
} from './capability'

describe('QA capability helpers', () => {
  it('formats readiness and dependency errors with request id state', () => {
    expect(
      formatQAError(
        new ApiError({
          code: 'not_implemented',
          message: 'route pending',
          requestId: 'req-501',
          status: 501,
        }),
        'RAG 检索',
      ),
    ).toContain('requestId: req-501')

    expect(
      formatQAStreamError({
        code: 'dependency_error',
        message: 'knowledge unavailable',
        status: 502,
      }),
    ).toContain('响应未包含 requestId')
  })

  it('builds tool steps from sanitized summary fields without dumping raw payloads', () => {
    const view = createSafeToolStep('completed', {
      argumentsSummary: {
        internalPreview: 'http://10.0.0.5/minio/private/object',
        objectKey: 'secret/minio/key',
        prompt: 'full hidden prompt',
        queryCount: 3,
      },
      latencyMs: 120,
      rawResult: 'provider raw response',
      resultSummary: { hitCount: 2 },
      toolCallId: 'tool-1',
      toolName: 'search_knowledge',
    })

    expect(view.toolCallId).toBe('tool-1')
    expect(view.step).toMatchObject({
      label: 'search_knowledge 完成',
      status: 'done',
      type: 'tool_call',
    })
    expect(view.step.detail).toContain('queryCount: 3')
    expect(view.step.detail).not.toContain('secret/minio/key')
    expect(view.step.detail).not.toContain('full hidden prompt')
    expect(view.step.detail).not.toContain('http://10.0.0.5')
    expect(view.step.detail).not.toContain('provider raw response')
  })

  it('does not display free-text tool summaries that may leak sensitive details', () => {
    const view = createSafeToolStep('failed', {
      errorCode: 'dependency_error',
      errorMessage: 'provider raw error body includes http://10.0.0.2/internal',
      latencyMs: 30,
      summary: 'prompt: hidden system prompt http://10.0.0.1/minio/bucket/object',
      toolSummary: 'safe-looking but unstructured text from backend',
      toolName: 'search_knowledge',
    })

    expect(view.step.detail).toContain('dependency_error')
    expect(view.step.detail).toContain('30ms')
    expect(view.step.detail).not.toContain('hidden system prompt')
    expect(view.step.detail).not.toContain('10.0.0.1')
    expect(view.step.detail).not.toContain('10.0.0.2')
    expect(view.step.detail).not.toContain('safe-looking but unstructured')
    expect(view.step.detail).not.toContain('provider raw error body')
  })

  it('accepts only structured reasoning and citation payloads', () => {
    expect(
      getSafeReasoningStep({
        step: {
          detail: '已生成脱敏摘要',
          label: '检索摘要',
          status: 'done',
          type: 'citation',
        },
      }),
    ).toMatchObject({ detail: '已生成脱敏摘要', status: 'done', type: 'citation' })

    expect(
      getSafeReasoningStep({ step: { status: 'done', type: 'private_chain_of_thought' } }),
    ).toBeUndefined()

    const citation = getCitationDelta({
      citation: {
        id: 'cit-1',
        isSourceAvailable: false,
        messageId: 'msg-1',
      },
    })

    expect(citation).toMatchObject({ id: 'cit-1', messageId: 'msg-1' })
    expect(getCitationDelta({ citation: { id: 'cit-2' } })).toBeUndefined()
  })

  it('keeps citation detail readiness explicit', () => {
    const citation: QACitation = {
      id: 'cit-1',
      isSourceAvailable: false,
      messageId: 'msg-1',
    }

    expect(getCitationAvailabilityText(citation)).toContain('仅展示 QA 保存的引用快照')
  })
})
