import { useMemo, useState, type ReactNode } from 'react'
import { api } from '../api/client'
import { AuthContext, type AuthState } from './authContextObject'

// AuthProvider wraps the app shell with the current session (§26). In dev
// mode this calls POST /api/v1/auth/login; in an OIDC deployment this
// component's login() would instead redirect through the identity provider
// (not yet implemented — see docs/roadmap.md).
export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('platform.token'))
  const [userId, setUserId] = useState<string | null>(() => localStorage.getItem('platform.userId'))

  const value = useMemo<AuthState>(
    () => ({
      token,
      userId,
      login: async (email: string, name: string) => {
        const res = await api.post<{ token: string; userId: string }>('/api/v1/auth/login', {
          email,
          name,
        })
        localStorage.setItem('platform.token', res.token)
        localStorage.setItem('platform.userId', res.userId)
        setToken(res.token)
        setUserId(res.userId)
      },
      logout: () => {
        localStorage.removeItem('platform.token')
        localStorage.removeItem('platform.userId')
        setToken(null)
        setUserId(null)
      },
    }),
    [token, userId],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
