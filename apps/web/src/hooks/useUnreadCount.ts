import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect } from 'react'
import { apiData } from '@/api/client'
import { queryKeys } from '@/lib/queryClient'

export function useUnreadCount(token: string, route: string) {
  const client = useQueryClient()
  const query = useQuery({ queryKey: queryKeys.unread, enabled: !!token,
    queryFn: ({ signal }) => apiData<{ unread_count: number }>('/api/message-stats/unread', { token, signal }) })
  const refresh = useCallback(async () => {
    if (!token) return
    await client.invalidateQueries({ queryKey: queryKeys.unread })
  }, [client, token])
  useEffect(() => { void refresh() }, [refresh, route])
  const value = Number(query.data?.unread_count || 0)
  return { count: Number.isFinite(value) ? value : 0, refresh }
}
