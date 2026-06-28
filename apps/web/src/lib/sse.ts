export type StreamEventType =
  'start' | 'delta' | 'citation' | 'reasoning' | 'progress' | 'done' | 'error'

export type StreamEvent<TPayload = unknown> = {
  type: StreamEventType
  payload?: TPayload
}
