import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { replayEvents, streamChat } from '@/api/chat'
import { ChatInput, ChatMessages, ChatSidebar } from '@/components/chat'
import {
  createSafeToolStep,
  formatQAError,
  formatQAStreamError,
  getCitationDelta,
  getSafeReasoningStep,
  useCreateSession,
  useDeleteSession,
  useRenameSession,
  useSessionMessages,
  useSessions,
} from '@/features/qa'
import type {
  QACitation,
  QAMessage,
  QASession,
  QASessionListItem,
  QAThinkingStep,
} from '@/lib/types'
import { useChatStore } from '@/stores/chat-store'

function nextId(): string {
  return Date.now().toString(36) + Math.random().toString(36).slice(2)
}

function getSessionTitle(prompt: string): string {
  return prompt.slice(0, 30) + (prompt.length > 30 ? '...' : '')
}

function toSessionListItem(session: QASession, messages: QAMessage[]): QASessionListItem {
  const last = messages[messages.length - 1]
  return {
    ...session,
    lastMessagePreview: last ? last.content.slice(0, 50) : session.lastMessagePreview,
    messageCount: messages.length || session.messageCount,
  }
}

const SUGGESTED_PROMPTS = [
  '变压器巡检有哪些要点？',
  '如何判断变压器油是否需要更换？',
  '电力安全工作规程中关于停电操作的规定是什么？',
]

export function ChatPage() {
  const {
    data: sessionsData,
    error: sessionsQueryError,
    isLoading: sessionsLoading,
    refetch: refetchSessions,
  } = useSessions()

  const sessions = useChatStore((state) => state.sessions)
  const setSessions = useChatStore((state) => state.setSessions)
  const activeId = useChatStore((state) => state.activeId)
  const setActiveId = useChatStore((state) => state.setActiveId)
  const streaming = useChatStore((state) => state.streaming)
  const setStreaming = useChatStore((state) => state.setStreaming)
  const error = useChatStore((state) => state.error)
  const setError = useChatStore((state) => state.setError)
  const lastFailedMsg = useChatStore((state) => state.lastFailedMsg)
  const setLastFailedMsg = useChatStore((state) => state.setLastFailedMsg)
  const clearError = useChatStore((state) => state.clearError)
  const addSession = useChatStore((state) => state.addSession)
  const removeSession = useChatStore((state) => state.removeSession)
  const updateSessionMessages = useChatStore((state) => state.updateSessionMessages)
  const appendSessionMessages = useChatStore((state) => state.appendSessionMessages)
  const messagesBySession = useChatStore((state) => state.messagesBySession)

  const {
    data: serverMessages,
    error: messagesQueryError,
    isError: hasMessagesError,
  } = useSessionMessages(activeId ?? '')

  const [inputText, setInputText] = useState('')
  const createSessionMut = useCreateSession()
  const deleteSessionMut = useDeleteSession()
  const renameSessionMut = useRenameSession()
  const abortRef = useRef<(() => void) | null>(null)
  const responseRunIdRef = useRef<string | null>(null)

  useEffect(() => {
    if (!sessionsData?.items) return

    const currentSessions = useChatStore.getState().sessions
    const merged: QASession[] = sessionsData.items.map((item) => {
      const existing = currentSessions.find((session) => session.id === item.id)
      return existing
        ? { ...existing, status: item.status, title: item.title, updatedAt: item.updatedAt }
        : item
    })
    setSessions(merged)
  }, [sessionsData, setSessions])

  useEffect(() => {
    if (!serverMessages?.items || !activeId) return

    const current = useChatStore.getState().messagesBySession[activeId]
    if ((!current || current.length === 0) && serverMessages.items.length > 0) {
      updateSessionMessages(activeId, serverMessages.items)
    }
  }, [activeId, serverMessages, updateSessionMessages])

  useEffect(() => {
    if (!hasMessagesError || !activeId) return

    const local = useChatStore.getState().messagesBySession[activeId]
    if (!local || local.length === 0) {
      setError(formatQAError(messagesQueryError, '加载会话消息'))
    }
  }, [activeId, hasMessagesError, messagesQueryError, setError])

  useEffect(() => {
    return () => {
      abortRef.current?.()
    }
  }, [])

  const sidebarItems: QASessionListItem[] = useMemo(
    () =>
      sessions.map((session) => toSessionListItem(session, messagesBySession[session.id] ?? [])),
    [messagesBySession, sessions],
  )

  const handleCreate = useCallback(async () => {
    try {
      const newSession = await createSessionMut.mutateAsync('新对话')
      addSession(newSession)
      setActiveId(newSession.id)
    } catch (caughtError) {
      setError(formatQAError(caughtError, '创建会话'))
    }
  }, [addSession, createSessionMut, setActiveId, setError])

  const handleDelete = useCallback(
    async (sessionId: string) => {
      try {
        await deleteSessionMut.mutateAsync(sessionId)
        removeSession(sessionId)
      } catch (caughtError) {
        setError(formatQAError(caughtError, '删除会话'))
      }
    },
    [deleteSessionMut, removeSession, setError],
  )

  const handleRename = useCallback(
    async (sessionId: string, title: string) => {
      try {
        await renameSessionMut.mutateAsync({ sessionId, title })
        setSessions(
          useChatStore
            .getState()
            .sessions.map((session) =>
              session.id === sessionId ? { ...session, title } : session,
            ),
        )
      } catch (caughtError) {
        setError(formatQAError(caughtError, '重命名会话'))
      }
    },
    [renameSessionMut, setError, setSessions],
  )

  const sendMessage = useCallback(
    async (text: string) => {
      const trimmed = text.trim()
      if (!trimmed || useChatStore.getState().streaming) return

      clearError()
      let targetId = useChatStore.getState().activeId

      if (!targetId) {
        try {
          const newSession = await createSessionMut.mutateAsync(getSessionTitle(trimmed))
          addSession(newSession)
          targetId = newSession.id
          setActiveId(targetId)
        } catch (caughtError) {
          setError(formatQAError(caughtError, '创建会话'))
          return
        }
      }

      const sessionId = targetId
      const now = new Date().toISOString()
      const userMessage: QAMessage = {
        content: trimmed,
        createdAt: now,
        id: nextId(),
        role: 'user',
        sessionId,
        status: 'completed',
      }
      const assistantMessage: QAMessage = {
        citations: [],
        content: '',
        createdAt: now,
        id: nextId(),
        role: 'assistant',
        sessionId,
        status: 'streaming',
        thinking: [],
      }

      appendSessionMessages(sessionId, [userMessage, assistantMessage])
      useChatStore.setState((state) => ({
        sessions: state.sessions.map((session) => {
          if (session.id !== sessionId) return session
          const messages = state.messagesBySession[sessionId] ?? []
          const isFirst = messages.length <= 2
          return {
            ...session,
            title: isFirst ? getSessionTitle(trimmed) : session.title,
            updatedAt: now,
          }
        }),
      }))

      setStreaming(true)

      let content = ''
      const steps: QAThinkingStep[] = []
      const citations: QACitation[] = []
      const toolStepIndexes = new Map<string, number>()

      const patchAssistant = (patch: Partial<QAMessage>) => {
        useChatStore.setState((state) => {
          const messages = [...(state.messagesBySession[sessionId] ?? [])]
          const lastIndex = messages.length - 1
          const last = messages[lastIndex]
          if (!last || last.role !== 'assistant') return state

          messages[lastIndex] = { ...last, ...patch }
          return {
            messagesBySession: {
              ...state.messagesBySession,
              [sessionId]: messages,
            },
          }
        })
      }

      let lastSeq = -1
      const verifySeq = (seq: number): boolean => {
        if (seq <= lastSeq) return false
        lastSeq = seq
        return true
      }

      const upsertToolStep = (kind: 'started' | 'completed' | 'failed', data: unknown) => {
        const { step, toolCallId } = createSafeToolStep(kind, data)
        const knownIndex = toolCallId ? toolStepIndexes.get(toolCallId) : undefined
        const fallbackIndex =
          knownIndex ??
          steps.findIndex((item) => item.type === 'tool_call' && item.status === 'running')

        if (fallbackIndex >= 0) {
          steps[fallbackIndex] = step
          if (toolCallId) toolStepIndexes.set(toolCallId, fallbackIndex)
        } else {
          steps.push(step)
          if (toolCallId) toolStepIndexes.set(toolCallId, steps.length - 1)
        }
        patchAssistant({ thinking: [...steps] })
      }

      const { abort } = streamChat(sessionId, trimmed, {
        onAgentIterationStarted(data) {
          if (!verifySeq(data.seq)) return
          const iterationNo = typeof data.iterationNo === 'number' ? data.iterationNo : undefined
          const alreadyRunning = steps.some(
            (step) => step.type === 'agent_iteration' && step.status === 'running',
          )
          if (!alreadyRunning) {
            steps.push({
              label: iterationNo ? `Agent 迭代 ${iterationNo}` : 'Agent 正在分析',
              status: 'running',
              type: 'agent_iteration',
            })
          }
          patchAssistant({ thinking: [...steps] })
        },
        onAnswerCompleted(data) {
          setStreaming(false)
          abortRef.current = null
          const runId = typeof data.responseRunId === 'string' ? data.responseRunId : undefined
          if (runId) responseRunIdRef.current = runId
          patchAssistant({
            citations: [...citations],
            content,
            status: 'completed',
            thinking: [...steps],
          })
        },
        onAnswerDelta(data) {
          if (!verifySeq(data.seq)) return
          content += data.content
          patchAssistant({ content, status: 'streaming' })
        },
        onAbort() {
          setStreaming(false)
          abortRef.current = null
          patchAssistant({
            citations: [...citations],
            content,
            status: 'stopped',
            thinking: [...steps],
          })
        },
        onCitationDelta(data) {
          if (!verifySeq(data.seq)) return
          const citation = getCitationDelta(data)
          if (!citation) return
          citations.push(citation)
          patchAssistant({ citations: [...citations] })
        },
        onError(streamError) {
          if (!verifySeq(streamError.seq)) return
          const message = formatQAStreamError(streamError)
          setError(message)

          if (streamError.fatal) {
            setStreaming(false)
            abortRef.current = null
            setLastFailedMsg(trimmed)
            patchAssistant({
              citations: [...citations],
              content,
              status: 'failed',
              thinking: [...steps],
            })
            return
          }

          const runId = responseRunIdRef.current
          if (runId) {
            void replayEvents(sessionId, runId).catch((caughtError) => {
              setError(formatQAError(caughtError, '恢复 QA 流式事件'))
            })
          }
        },
        onMessageCreated(data) {
          if (!verifySeq(data.seq)) return
          const messageId = typeof data.messageId === 'string' ? data.messageId : undefined
          const runId = typeof data.responseRunId === 'string' ? data.responseRunId : undefined
          if (messageId) patchAssistant({ id: messageId })
          if (runId) responseRunIdRef.current = runId
        },
        onReasoningStep(data) {
          if (!verifySeq(data.seq)) return
          const step = getSafeReasoningStep(data)
          if (!step) return
          const index = steps.findIndex(
            (item) => item.type === step.type && item.label === step.label,
          )
          if (index >= 0) {
            steps[index] = step
          } else {
            steps.push(step)
          }
          patchAssistant({ thinking: [...steps] })
        },
        onToolCompleted(data) {
          if (!verifySeq(data.seq)) return
          upsertToolStep('completed', data)
        },
        onToolFailed(data) {
          if (!verifySeq(data.seq)) return
          upsertToolStep('failed', data)
        },
        onToolStarted(data) {
          if (!verifySeq(data.seq)) return
          upsertToolStep('started', data)
        },
      })

      abortRef.current = abort
    },
    [
      addSession,
      appendSessionMessages,
      clearError,
      createSessionMut,
      setActiveId,
      setError,
      setLastFailedMsg,
      setStreaming,
    ],
  )

  const handleRetry = useCallback(() => {
    if (!lastFailedMsg) return
    const failedMessage = lastFailedMsg
    clearError()
    void sendMessage(failedMessage)
  }, [clearError, lastFailedMsg, sendMessage])

  const handleSuggested = useCallback(
    (prompt: string) => {
      setInputText(prompt)
      window.setTimeout(() => {
        void sendMessage(prompt)
        setInputText('')
      }, 0)
    },
    [sendMessage],
  )

  const activeSession = sessions.find((session) => session.id === activeId)
  const activeMessages = activeId ? (messagesBySession[activeId] ?? []) : []

  return (
    <div className="flex h-full">
      <ChatSidebar
        activeId={activeId ?? ''}
        fetchError={sessionsQueryError ? formatQAError(sessionsQueryError, '加载会话列表') : null}
        isLoading={sessionsLoading}
        onCreate={handleCreate}
        onDelete={handleDelete}
        onRename={handleRename}
        onRetryFetch={() => void refetchSessions()}
        onSelect={setActiveId}
        sessions={sidebarItems}
      />

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex shrink-0 items-center justify-between border-b border-border bg-muted/30 px-6 py-3">
          <h1 className="text-lg font-semibold text-foreground">
            {activeSession?.title ?? '智能问答'}
          </h1>
        </header>

        <ChatMessages
          error={error}
          messages={activeMessages}
          onRetry={handleRetry}
          onSuggestedClick={handleSuggested}
          streaming={streaming}
          suggestedPrompts={SUGGESTED_PROMPTS}
        />

        <ChatInput
          disabled={streaming}
          onChange={setInputText}
          onSend={sendMessage}
          value={inputText}
        />
      </div>
    </div>
  )
}
