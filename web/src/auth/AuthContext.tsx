import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { api, get, setCSRF } from '../api/client'
import type { User } from '../types'

type AuthValue = { user: User | null; loading: boolean; login: (username: string, password: string) => Promise<User>; logout: () => Promise<void>; refresh: () => Promise<void>; has: (permission: string) => boolean }
const AuthContext = createContext<AuthValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  const refresh = useCallback(async () => {
    try { const next = await get<User>('/api/v1/auth/me'); setCSRF(next.csrfToken); setUser(next) }
    catch { setCSRF(); setUser(null) }
    finally { setLoading(false) }
  }, [])
  useEffect(() => { void refresh() }, [refresh])
  const login = useCallback(async (username: string, password: string) => {
    const next = await api<User>('/api/v1/auth/login', { method: 'POST', body: JSON.stringify({ username, password }), headers: { 'Content-Type': 'application/json' } })
    setCSRF(next.csrfToken); setUser(next); return next
  }, [])
  const logout = useCallback(async () => { await api('/api/v1/auth/logout', { method: 'POST' }); setCSRF(); setUser(null) }, [])
  const has = useCallback((wanted: string) => !!user?.permissions.some(p => p === '*' || p === wanted || (p.endsWith(':*') && wanted.startsWith(p.slice(0, -1)))), [user])
  const value = useMemo(() => ({ user, loading, login, logout, refresh, has }), [user, loading, login, logout, refresh, has])
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() { const value = useContext(AuthContext); if (!value) throw new Error('AuthProvider missing'); return value }
