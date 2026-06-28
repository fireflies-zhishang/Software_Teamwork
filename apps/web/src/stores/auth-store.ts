import { create } from 'zustand'

type AuthState = {
  userName: string | null
  setUserName: (userName: string | null) => void
}

export const useAuthStore = create<AuthState>((set) => ({
  userName: null,
  setUserName: (userName) => set({ userName }),
}))
