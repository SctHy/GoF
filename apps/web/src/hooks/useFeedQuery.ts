import { useInfiniteQuery, useQueryClient, type InfiniteData } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { errorMessage, errorStatus } from '@/api/client'
import { fetchFeedPage } from '@/api/resources'
import { appendFeedItems, createFeedRequestID, mapFeedItem, paginationErrorMessage, requiresAuthFeed } from '@/lib/feed'
import type { FeedScene, FeedItem, Page, LoadState } from '@/types'

export function useFeedQuery({ scene, token, onReset, onUnauthorized }: {
  scene: FeedScene; token: string; onReset: () => void; onUnauthorized: () => void
}) {
  const client = useQueryClient()
  const [paginationFailure, setPaginationFailure] = useState<unknown>(null)
  const [requestID, setRequestID] = useState(() => createFeedRequestID(scene))
  const queryKey = useMemo(() => ['feed', scene, requestID] as const, [scene, requestID])
  const needsAuth = requiresAuthFeed(scene) && !token
  const query = useInfiniteQuery({
    queryKey, enabled: !needsAuth, initialPageParam: '',
    queryFn: async ({ pageParam, signal }): Promise<Page<FeedItem>> => {
      const page = await fetchFeedPage(scene, token, pageParam, requestID, signal)
      return { ...page, items: page.items.map(item => mapFeedItem(item, scene, requestID)) }
    },
    getNextPageParam: (last, _pages, _cursor, cursors) =>
      last.has_more && last.next_cursor && !cursors.includes(last.next_cursor) ? last.next_cursor : undefined,
    // A feed snapshot is stable until the reader explicitly refreshes it.
    staleTime: Infinity, gcTime: 0, retry: false,
    refetchOnMount: false, refetchOnWindowFocus: false, refetchOnReconnect: false,
  })
  const items = useMemo(() => (query.data?.pages || []).reduce<FeedItem[]>((all, page) => appendFeedItems(all, page.items), []), [query.data])
  useEffect(() => { if (query.isFetchNextPageError) setPaginationFailure(query.error) }, [query.isFetchNextPageError, query.error])
  const paginationError = paginationFailure || query.isFetchNextPageError ? paginationErrorMessage(paginationFailure || query.error) : ''
  const hasMore = !!query.hasNextPage && !paginationError
  const loadMoreFeed = useCallback(() => {
    if (hasMore && !query.isFetching && !needsAuth) void query.fetchNextPage({ cancelRefetch: false })
  }, [hasMore, query.isFetching, query.fetchNextPage, needsAuth])
  const loadFeed = useCallback(() => {
    void client.cancelQueries({ queryKey })
    onReset()
    setPaginationFailure(null)
    setRequestID(createFeedRequestID(scene))
  }, [client, queryKey, onReset, scene])
  const updateItem = useCallback((videoID: number, patch: Partial<FeedItem> | ((item: FeedItem) => Partial<FeedItem>)) => {
    client.setQueryData<InfiniteData<Page<FeedItem>, string>>(queryKey, data => data && ({ ...data,
      pages: data.pages.map(page => ({ ...page, items: page.items.map(item => item.video_id === videoID
        ? { ...item, ...(typeof patch === 'function' ? patch(item) : patch) } : item) })) }))
  }, [client, queryKey])
  useEffect(() => {
    if (query.isFetchNextPageError && errorStatus(query.error) === 401 && requiresAuthFeed(scene)) onUnauthorized()
  }, [query.isFetchNextPageError, query.error, scene, onUnauthorized])
  const feedState: LoadState = needsAuth || (errorStatus(query.error) === 401 && requiresAuthFeed(scene)) ? 'auth'
    : query.data ? 'ready' : query.isPending ? 'loading' : 'error'
  return { items, updateItem, feedState, feedError: errorMessage(query.error) || '视频加载失败',
    paginationError, hasMore, loadingMore: query.isFetchingNextPage, feedRequestID: requestID, loadFeed, loadMoreFeed }
}
