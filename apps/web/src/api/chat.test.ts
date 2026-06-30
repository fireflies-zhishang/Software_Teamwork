import { describe, expect, it, vi } from 'vitest'

import { streamChat } from './chat'

function streamResponse(body: string) {
  return new Response(new TextEncoder().encode(body), {
    headers: { 'Content-Type': 'text/event-stream' },
    status: 200,
  })
}

describe('chat stream API', () => {
  it('normalizes answer.delta text payloads to content', async () => {
    const onAnswerDelta = vi.fn()
    const onAnswerCompleted = vi.fn()
    const onError = vi.fn()
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValue(
        streamResponse(
          [
            'event: answer.delta',
            'data: {"text":"root","index":0}',
            '',
            'event: answer.delta',
            'data: {"content":" cause","index":1}',
            '',
            'event: answer.completed',
            'data: {"responseRunId":"run-1"}',
            '',
            '',
          ].join('\n'),
        ),
      )
    vi.stubGlobal('fetch', fetchMock)

    streamChat('session-1', 'question', {
      onAnswerCompleted,
      onAnswerDelta,
      onError,
    })

    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))
    expect(onError).not.toHaveBeenCalled()
    await vi.waitFor(() => expect(onAnswerCompleted).toHaveBeenCalledTimes(1))

    expect(onAnswerDelta).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({ content: 'root', index: 0, seq: 1, text: 'root' }),
    )
    expect(onAnswerDelta).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({ content: ' cause', index: 1, seq: 2 }),
    )
  })
})
