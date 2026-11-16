import { QueryClient } from '@tanstack/react-query'

// A new client is mounted for each login identity; no private cache is persisted.
export function createServerQueryClient() {
  return new QueryClient({ defaultOptions: {
    queries: { staleTime: 15_000, gcTime: 5 * 60_000, retry: false, refetchOnWindowFocus: false },
    mutations: { retry: false },
  } })
}

export const queryKeys = {
  unread: ['messages', 'unread'] as const,
  messages: ['messages', 'list'] as const,
  following: ['relations', 'following-map'] as const,
  relations: (tab: string) => ['relations', 'list', tab] as const,
  ownVideos: ['videos', 'mine'] as const,
  publicProfile: (id: number) => ['users', id] as const,
  publicVideos: (id: number) => ['users', id, 'videos'] as const,
  comments: (id: number) => ['videos', id, 'comments'] as const,
  playback: ['playback', 'config'] as const,
}
