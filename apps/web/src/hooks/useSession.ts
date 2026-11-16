import type { Session, User } from '@/types'
import { useCallback, useMemo, useRef, useState } from 'react'
import { TOKEN_KEY, USER_KEY } from '@/lib/constants'
import { readStoredUser } from '@/lib/profiles'

export function useSession(): Session {
  const [auth, setAuthState] = useState(() => ({ token: localStorage.getItem(TOKEN_KEY) || '', user: readStoredUser() }))
  const currentAuth = useRef(auth)
  const { token, user } = auth
  // An async result captured under an old identity must not change a newer session.
  const setAuth = useCallback((nextToken: string, nextUser: User | null) => {
    if (currentAuth.current.token !== token) return
    if (nextToken) localStorage.setItem(TOKEN_KEY, nextToken)
    else localStorage.removeItem(TOKEN_KEY)
    if (nextUser) localStorage.setItem(USER_KEY, JSON.stringify(nextUser))
    else localStorage.removeItem(USER_KEY)
    currentAuth.current = { token: nextToken, user: nextUser }
    setAuthState(currentAuth.current)
  }, [token])
  const clearAuth = useCallback(() => setAuth('', null), [setAuth])
  return useMemo(() => ({ token, user, setAuth, clearAuth }), [token, user, setAuth, clearAuth])
}
