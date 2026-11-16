import type { ReactNode } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { test, expect, vi } from 'vitest'
import { QueryClientProvider, focusManager, onlineManager } from '@tanstack/react-query'
import { createServerQueryClient } from '@/lib/queryClient'
import { useFeedQuery } from '@/hooks/useFeedQuery'
import { feedResponse } from '../fixtures'

const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status })
function harness() {
  const client = createServerQueryClient()
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
  return { client, wrapper }
}

test('feed snapshots deduplicate pages, stop on expired cursors and only restart on explicit refresh', async () => {
  const fetch = vi.fn(async (url: string, init?: RequestInit) => {
    expect(url).toBe('/api/feed-queries')
    const body = JSON.parse(String(init?.body))
    if (body.cursor === 'expired') return json({ error: 'cursor expired' }, 400)
    if (body.cursor === 'next') return json({ items: [feedResponse, { ...feedResponse, video_id: 2 }], has_more: true, next_cursor: 'expired' })
    return json({ items: [feedResponse], has_more: true, next_cursor: 'next' })
  })
  vi.stubGlobal('fetch', fetch)
  const { wrapper, client } = harness()
  const onReset = vi.fn()
  const hook = renderHook(() => useFeedQuery({ scene: 'recommend', token: 'token', onReset, onUnauthorized() {} }), { wrapper })
  await waitFor(() => expect(hook.result.current.items).toHaveLength(1))
  const firstID = hook.result.current.feedRequestID
  act(() => hook.result.current.loadMoreFeed())
  await waitFor(() => expect(hook.result.current.items.map(item => item.video_id)).toEqual([1, 2]))
  act(() => hook.result.current.loadMoreFeed())
  await waitFor(() => expect(hook.result.current.paginationError).toBe('分页已过期，请刷新'))
  expect(hook.result.current.feedState).toBe('ready')
  expect(hook.result.current.hasMore).toBe(false)
  expect(hook.result.current.items).toHaveLength(2)
  act(() => { hook.result.current.loadMoreFeed(); focusManager.setFocused(false); focusManager.setFocused(true); onlineManager.setOnline(false); onlineManager.setOnline(true) })
  expect(fetch).toHaveBeenCalledTimes(3)
  act(() => hook.result.current.updateItem(1, { liked: true, like_count: 9 }))
  await waitFor(() => expect(hook.result.current.items[0]?.liked).toBe(true))
  expect(hook.result.current.hasMore).toBe(false)
  act(() => hook.result.current.loadMoreFeed())
  expect(fetch).toHaveBeenCalledTimes(3)
  act(() => hook.result.current.loadFeed())
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(4))
  await waitFor(() => expect(hook.result.current.items).toHaveLength(1))
  expect(onReset).toHaveBeenCalledOnce()
  expect(hook.result.current.feedRequestID).not.toBe(firstID)
  const requests = fetch.mock.calls.map(([, init]) => JSON.parse(String(init?.body)))
  expect(requests.slice(0, 3).every(body => body.context.request_id === firstID)).toBe(true)
  hook.unmount(); client.clear(); focusManager.setFocused(undefined)
})

test('refresh cancels an old snapshot and ignores its late response even when transport ignores abort', async () => {
  let finishOld!: (response: Response) => void
  let oldSignal: AbortSignal | null | undefined
  const fetch = vi.fn<typeof globalThis.fetch>()
    .mockImplementationOnce((_url, init) => { oldSignal = init?.signal; return new Promise(resolve => { finishOld = resolve }) })
    .mockResolvedValue(json({ items: [{ ...feedResponse, video_id: 9 }], has_more: false, next_cursor: '' }))
  vi.stubGlobal('fetch', fetch)
  const { wrapper, client } = harness()
  const hook = renderHook(() => useFeedQuery({ scene: 'timeline', token: '', onReset() {}, onUnauthorized() {} }), { wrapper })
  await waitFor(() => expect(finishOld).toBeTypeOf('function'))
  act(() => hook.result.current.loadFeed())
  await waitFor(() => expect(hook.result.current.items[0]?.video_id).toBe(9))
  expect(oldSignal?.aborted).toBe(true)
  await act(async () => finishOld(json({ items: [feedResponse], has_more: false, next_cursor: '' })))
  expect(hook.result.current.items[0]?.video_id).toBe(9)
  hook.unmount(); client.clear()
})

test('unmount aborts in-flight feed requests', async () => {
  let signal: AbortSignal | null | undefined
  vi.stubGlobal('fetch', vi.fn<typeof fetch>((_url, init) => { signal = init?.signal; return new Promise(() => {}) }))
  const { wrapper, client } = harness()
  const hook = renderHook(() => useFeedQuery({ scene: 'timeline', token: '', onReset() {}, onUnauthorized() {} }), { wrapper })
  await waitFor(() => expect(signal).toBeTruthy())
  hook.unmount()
  expect(signal?.aborted).toBe(true)
  client.clear()
})
