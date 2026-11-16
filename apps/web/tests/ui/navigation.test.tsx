import { beforeEach, test, expect, vi } from 'vitest'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { createAppRouter } from '@/router'
import { TOKEN_KEY, USER_KEY } from '@/lib/constants'
import { feedResponse, session, video } from '../fixtures'

const json = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status })
let fetchMock: ReturnType<typeof vi.fn<typeof fetch>>
beforeEach(() => {
  fetchMock = vi.fn<typeof fetch>(async (input) => {
    const path = new URL(String(input), 'http://localhost').pathname
    if (path === '/api/feed-items' || path === '/api/feed-queries') return json({ items: [feedResponse], has_more: false, next_cursor: '' })
    if (path === '/api/sessions') return json({ access_token: 'bob-token' })
    if (path === '/api/users/me') return json({ ...session.user, id: 2, nickname: 'Bob' })
    if (path === '/api/users/2') return json({ ...session.user, id: 2, nickname: 'Bob' })
    if (path.endsWith('/videos')) return json({ items: [video] })
    if (path === '/api/message-stats/unread') return json({ unread_count: 0 })
    return json({ items: [], has_more: false, next_cursor: '' })
  })
  vi.stubGlobal('fetch', fetchMock)
})
function open(path: string, authenticated = false) {
  if (authenticated) { localStorage.setItem(TOKEN_KEY, session.token); localStorage.setItem(USER_KEY, JSON.stringify(session.user)) }
  const router = createAppRouter(createMemoryHistory({ initialEntries: [path] }))
  const view = render(<RouterProvider router={router} />)
  return { router, view }
}

test.each([['/login', '/auth'], ['/me', '/timeline'], ['/messages', '/auth'], ['/unknown', '/timeline'], ['/users/nope', '/timeline'], ['/', '/timeline']])(
  'route %s resolves to %s', async (path, expected) => {
    const { router } = open(path)
    await waitFor(() => expect(router.state.location.pathname).toBe(expected))
    await waitFor(() => expect(screen.getAllByRole('main')).toHaveLength(1))
  })

test('public profiles route by user ID and browser back restores the previous page', async () => {
  const { router } = open('/timeline')
  await screen.findByRole('heading', { name: feedResponse.title })
  await act(async () => { await router.navigate({ to: '/users/$userID', params: { userID: '2' } }) })
  await screen.findByRole('heading', { name: 'Bob' })
  await act(async () => router.history.back())
  await screen.findByRole('heading', { name: feedResponse.title })
  expect(router.state.location.pathname).toBe('/timeline')
})

test('logging out cancels private reads and a late response cannot populate the next identity', async () => {
  let finishOld!: (response: Response) => void
  let oldSignal: AbortSignal | null | undefined
  const normal = fetchMock.getMockImplementation()!
  fetchMock.mockImplementation((input, init) => {
    const url = new URL(String(input), 'http://localhost')
    if (url.pathname === '/api/messages') {
      if (new Headers(init?.headers).get('Authorization') === `Bearer ${session.token}`) {
        oldSignal = init?.signal
        return new Promise(resolve => { finishOld = resolve })
      }
      return Promise.resolve(json({ items: [{ id: 2, title: 'Bob 的消息', content: '新身份', type: 'SYSTEM', is_read: false, created_at: '2026-11-17' }], has_more: false }))
    }
    return normal(input, init)
  })
  const { router } = open('/messages', true)
  await waitFor(() => expect(finishOld).toBeTypeOf('function'))
  await userEvent.click(screen.getByRole('button', { name: '退出登录' }))
  await waitFor(() => expect(oldSignal?.aborted).toBe(true))
  await act(async () => { await router.navigate({ to: '/auth' }) })
  await userEvent.type(await screen.findByPlaceholderText('请输入账号'), 'bob')
  await userEvent.type(screen.getByPlaceholderText('输入密码'), 'password')
  await userEvent.click(screen.getByRole('button', { name: '登录' }))
  await waitFor(() => expect(localStorage.getItem(TOKEN_KEY)).toBe('bob-token'))
  await act(async () => { await router.navigate({ to: '/messages' }) })
  await screen.findByText('Bob 的消息')
  await act(async () => finishOld(json({ items: [{ id: 1, title: '旧身份消息', content: 'private', type: 'SYSTEM', is_read: false, created_at: '2026-11-17' }], has_more: false })))
  expect(screen.queryByText('旧身份消息')).toBeNull()
  expect(screen.getByText('Bob 的消息')).toBeTruthy()
})

test('profile edit dialog preserves form submit and upload page remains accessible', async () => {
  const { router } = open('/profile', true)
  await userEvent.click(await screen.findByRole('button', { name: '编辑资料' }))
  const dialog = await screen.findByRole('dialog', { name: '资料编辑' })
  expect(dialog.querySelector('form')).toBeTruthy()
  await userEvent.click(screen.getByRole('button', { name: '保存' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(fetchMock.mock.calls.some(([url, init]) => url === '/api/users/me' && init?.method === 'PATCH')).toBe(true)
  await act(async () => { await router.navigate({ to: '/upload' }) })
  expect(await screen.findByRole('heading', { name: '发布视频' })).toBeTruthy()
})


test('publishing invalidates cached own works before returning to the profile', async () => {
  let published = false
  const normal = fetchMock.getMockImplementation()!
  fetchMock.mockImplementation(async (input, init) => {
    const path = new URL(String(input), 'http://localhost').pathname
    if (path === '/api/uploads') return json({ url: '/uploads/new.mp4' })
    if (path === '/api/videos' && init?.method === 'POST') { published = true; return json({ id: 9 }) }
    if (path === '/api/users/me/videos') return json({ items: published ? [video, { ...video, id: 9, title: 'New published video' }] : [video] })
    return normal(input, init)
  })
  vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:preview')
  vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
  const { router } = open('/profile', true)
  await screen.findByRole('heading', { name: video.title })
  await act(async () => { await router.navigate({ to: '/upload' }) })
  await userEvent.type(await screen.findByPlaceholderText('输入视频标题'), 'New published video')
  await userEvent.upload(document.querySelector<HTMLInputElement>('input[accept="video/*"]')!, new File(['video'], 'video.mp4', { type: 'video/mp4' }))
  await userEvent.upload(document.querySelector<HTMLInputElement>('input[accept="image/*"]')!, new File(['image'], 'cover.png', { type: 'image/png' }))
  await userEvent.click(document.querySelector<HTMLButtonElement>('form button[type="submit"]')!)
  await screen.findByRole('heading', { name: 'New published video' })
  expect(router.state.location.pathname).toBe('/profile')
  expect(fetchMock.mock.calls.filter(([url]) => String(url).startsWith('/api/users/me/videos'))).toHaveLength(2)
})


test('mobile navigation opens as a sheet and closes after choosing a page', async () => {
  const normal = vi.mocked(window.matchMedia).getMockImplementation()!
  vi.spyOn(window, 'matchMedia').mockImplementation(query => ({ ...normal(query), matches: query === '(max-width: 767px)' }))
  const { router } = open('/timeline')
  await screen.findByRole('heading', { name: feedResponse.title })
  await userEvent.click(screen.getByRole('button', { name: '切换导航' }))
  await screen.findByRole('dialog', { name: 'Sidebar' })
  await userEvent.click(screen.getByRole('button', { name: '推荐流' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/recommend'))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(screen.getAllByRole('main')).toHaveLength(1)
})
