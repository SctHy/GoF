import { TooltipProvider } from "@/components/ui/tooltip";
import { Suspense, useCallback, useEffect, useMemo, useState } from 'react'
import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { apiRequest } from '@/api/client'
import { AppShell } from '@/components/AppShell'
import { PageMessage } from '@/components/PageMessage'
import { useSession } from '@/hooks/useSession'
import { useUnreadCount } from '@/hooks/useUnreadCount'
import { createServerQueryClient } from '@/lib/queryClient'
import { AppContext } from '@/lib/appContext'
import type { Session } from '@/types'

export default function App() {
  const session = useSession()
  return <SessionWorkspace key={`${session.token}:${session.user?.id || 0}`} session={session} />
}
function SessionWorkspace({ session }: { session: Session }) {
  const [client] = useState(createServerQueryClient)
  useEffect(() => () => { client.clear() }, [client])
  return <QueryClientProvider client={client}><AppLayout session={session} /></QueryClientProvider>
}
function AppLayout({ session }: { session: Session }) {
  const route = useRouterState({ select: state => state.location.pathname })
  const routeNavigate = useNavigate()
  const navigate = useCallback((path: string) => { void routeNavigate({ to: path }) }, [routeNavigate])
  const { count, refresh } = useUnreadCount(session.token, route)
  const logout = useCallback(() => {
    if (session.token) void apiRequest('/api/sessions/current', { method: 'DELETE', token: session.token }).catch(() => {})
    session.clearAuth()
    navigate('/timeline')
  }, [session.token, session.clearAuth, navigate])
  const context = useMemo(() => ({ session, navigate, refreshUnreadCount: refresh }), [session, navigate, refresh])
  const content = <Suspense fallback={<PageMessage icon="hourglass_top" title="正在加载页面" />}><Outlet /></Suspense>
  return <AppContext.Provider value={context}><TooltipProvider>
    {route === '/auth' ? content : <AppShell user={session.user} route={route} authenticated={!!(session.token && session.user)} unreadCount={count} onNavigate={navigate} onLogout={logout}>{content}</AppShell>}
  </TooltipProvider></AppContext.Provider>
}
