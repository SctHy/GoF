import { createContext, useContext } from 'react'
import type { Navigate, Session } from '@/types'

export interface AppContextValue {
  session: Session
  navigate: Navigate
  refreshUnreadCount: () => Promise<void>
}
export const AppContext = createContext<AppContextValue | null>(null)
export function useAppContext() {
  const value = useContext(AppContext)
  if (!value) throw new Error('AppContext is required')
  return value
}
