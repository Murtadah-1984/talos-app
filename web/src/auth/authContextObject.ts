import { createContext } from 'react'

export interface AuthState {
  token: string | null
  userId: string | null
  login: (email: string, name: string) => Promise<void>
  logout: () => void
}

export const AuthContext = createContext<AuthState | undefined>(undefined)
