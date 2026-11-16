import { useState } from 'react'
import { test, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { CommentPanel } from '@/components/CommentPanel'
import { VideoStage } from '@/components/VideoStage'
import { WorkViewer, RelationModal } from '@/components/ProfileContent'
import { FeedPaginationNotice } from '@/components/FeedMessage'
import { TooltipProvider } from '@/components/ui/tooltip'
import { Button } from '@/components/ui/button'
import Login from '@/pages/LoginPage'
import Feed from '@/pages/FeedPage'
import Profile from '@/pages/ProfilePage'
import PublicProfile from '@/pages/PublicProfilePage'
import Upload from '@/pages/UploadPage'
import Messages from '@/pages/MessagesPage'
import { createServerQueryClient } from '@/lib/queryClient'
import { mapFeedItem } from '@/lib/feed'
import { feedResponse, session, video } from '../fixtures'

const json = (value: unknown) => new Response(JSON.stringify(value))

test('video stages and work dialogs retain playable media', async () => {
  const stage = render(<TooltipProvider><VideoStage item={mapFeedItem(feedResponse)} active={false} liked={false} favorited={false}
    following={false} followBusy={false} ownVideo={false} followError=""
    onLike={() => {}} onComment={() => {}} onFavorite={() => {}} onFollow={() => {}} onOpenAuthor={() => {}} /></TooltipProvider>)
  expect(stage.container.querySelector('video')?.getAttribute('src')).toBe(video.media_url)
  expect(screen.getByRole('heading', { name: 'Regression video' })).toBeTruthy()
  stage.unmount()
  render(<WorkViewer video={video} onClose={() => {}} />)
  expect(await screen.findByRole('dialog', { name: video.title })).toBeTruthy()
  expect(document.querySelector('video')?.getAttribute('src')).toBe(video.media_url)
})

test('all pages mount with the shared query provider', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => json({ ...session.user, items: [], has_more: false, next_cursor: '' })))
  const props = { session, onNavigate() {} }
  const pages = [<Login {...props} />, <Feed {...props} feedScene="timeline" />, <Profile {...props} />,
    <PublicProfile userID={2} onNavigate={() => {}} />, <Upload {...props} />, <Messages {...props} onUnreadChange={() => {}} />]
  for (const page of pages) {
    const client = createServerQueryClient()
    const view = render(<QueryClientProvider client={client}><TooltipProvider>{page}</TooltipProvider></QueryClientProvider>)
    expect(screen.getAllByRole('heading').length).toBeGreaterThan(0)
    await waitFor(() => expect(client.isFetching()).toBe(0))
    view.unmount(); client.clear()
  }
})

test('expired pagination exposes an explicit refresh action', async () => {
  const refresh = vi.fn()
  render(<FeedPaginationNotice error="分页已过期，请刷新" onRefresh={refresh} />)
  await userEvent.click(screen.getByRole('button', { name: '分页已过期，请刷新' }))
  expect(refresh).toHaveBeenCalledOnce()
})

test('relation tabs support keyboard navigation and the dialog closes with Escape', async () => {
  function Demo() {
    const [open, setOpen] = useState(false)
    const [tab, setTab] = useState<'following' | 'followers'>('following')
    return <><Button onClick={() => setOpen(true)}>打开关系</Button>{open && <RelationModal tab={tab} onTabChange={setTab} onClose={() => setOpen(false)}
      items={[]} state="ready" error="" hasMore={false} following={{}} busyID={0} currentUserID={1}
      onRetry={() => {}} onLoadMore={() => {}} onToggleFollow={() => {}} />}</>
  }
  render(<Demo />)
  const trigger = screen.getByRole('button', { name: '打开关系' })
  await userEvent.click(trigger)
  expect(await screen.findByRole('dialog', { name: '关注的人' })).toBeTruthy()
  screen.getByRole('tab', { name: '关注' }).focus()
  await userEvent.keyboard('{ArrowRight}')
  expect(document.activeElement).toBe(screen.getByRole('tab', { name: '粉丝' }))
  await userEvent.keyboard('{Enter}')
  await waitFor(() => expect(screen.getByRole('tab', { name: '粉丝' }).getAttribute('aria-selected')).toBe('true'))
  await userEvent.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  await waitFor(() => expect(document.activeElement).toBe(trigger))
})

test('login/register tabs and submit buttons keep their form behavior', async () => {
  const auth = vi.fn()
  const navigate = vi.fn()
  vi.stubGlobal('fetch', vi.fn(async (url: string) => json(url === '/api/sessions' ? { access_token: 'new-token' } : session.user)))
  render(<Login session={{ ...session, setAuth: auth }} onNavigate={navigate} />)
  await userEvent.click(screen.getByRole('tab', { name: '注册' }))
  await userEvent.type(screen.getByPlaceholderText('请输入账号'), 'reader')
  await userEvent.type(screen.getByPlaceholderText('输入昵称'), 'Reader')
  await userEvent.type(screen.getByPlaceholderText('输入密码'), 'password')
  await userEvent.click(screen.getByRole('button', { name: '注册并登录' }))
  await waitFor(() => expect(auth).toHaveBeenCalledWith('new-token', session.user))
  expect(navigate).toHaveBeenCalledWith('/recommend')
})


test('comment sheet contains focus, submits a draft and restores focus on Escape', async () => {
  const submit = vi.fn()
  function Comments() {
    const [open, setOpen] = useState(false)
    const [value, setValue] = useState('')
    return <><Button onClick={() => setOpen(true)}>打开评论</Button><CommentPanel open={open} onClose={() => setOpen(false)} value={value} onChange={setValue} onSubmit={submit} user={session.user!} count={0} comments={[]} state="ready" error="" onRetry={() => {}} authenticated onOpenUser={() => {}} /></>
  }
  render(<Comments />)
  const trigger = screen.getByRole('button', { name: '打开评论' })
  await userEvent.click(trigger)
  const dialog = await screen.findByRole('dialog', { name: /评论/ })
  await waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true))
  for (let step = 0; step < 5; step++) {
    await userEvent.tab()
    await waitFor(() => expect({ inside: dialog.contains(document.activeElement), focus: document.activeElement?.outerHTML }).toMatchObject({ inside: true }))
  }
  await userEvent.type(screen.getByRole('textbox', { name: '评论内容' }), '喜欢这个视频')
  await userEvent.click(screen.getByRole('button', { name: '发送评论' }))
  expect(submit).toHaveBeenCalledOnce()
  await userEvent.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  await waitFor(() => expect(document.activeElement).toBe(trigger))
})
