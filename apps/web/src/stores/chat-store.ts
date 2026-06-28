import { create } from 'zustand'

type ChatDraftState = {
  draft: string
  setDraft: (draft: string) => void
}

export const useChatStore = create<ChatDraftState>((set) => ({
  draft: '',
  setDraft: (draft) => set({ draft }),
}))
