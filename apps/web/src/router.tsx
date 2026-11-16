import { lazy } from 'react'
import { createRootRoute, createRoute, createRouter, Navigate, type RouterHistory } from '@tanstack/react-router'
import App from '@/App'
import { useAppContext } from '@/lib/appContext'
import FeedPage from '@/pages/FeedPage'
import type { FeedScene } from '@/types'

const LoginPage = lazy(() => import('@/pages/LoginPage'))
const ProfilePage = lazy(() => import('@/pages/ProfilePage'))
const PublicProfilePage = lazy(() => import('@/pages/PublicProfilePage'))
const UploadPage = lazy(() => import('@/pages/UploadPage'))
const MessagesPage = lazy(() => import('@/pages/MessagesPage'))
const root = createRootRoute({ component: App, notFoundComponent: () => <Navigate to="/timeline" replace /> })
function Landing() {
  const { session } = useAppContext()
  return <Navigate to={session.token ? '/recommend' : '/timeline'} replace />
}
function FeedRoute({ scene }: { scene: FeedScene }) {
  const { session, navigate } = useAppContext()
  return <FeedPage key={scene} feedScene={scene} session={session} onNavigate={navigate} />
}
const index = createRoute({ getParentRoute: () => root, path: '/', component: Landing })
const auth = createRoute({ getParentRoute: () => root, path: '/auth', component: () => {
  const { session, navigate } = useAppContext()
  return <LoginPage session={session} onNavigate={navigate} />
} })
const profile = createRoute({ getParentRoute: () => root, path: '/profile', component: () => {
  const { session, navigate } = useAppContext()
  return session.token && session.user ? <ProfilePage session={session} onNavigate={navigate} /> : <Navigate to="/timeline" replace />
} })
const publicProfile = createRoute({ getParentRoute: () => root, path: '/users/$userID', component: () => {
  const { userID } = publicProfile.useParams()
  const { navigate } = useAppContext()
  const id = Number(userID)
  return /^\d+$/.test(userID) && id > 0 && Number.isSafeInteger(id) ? <PublicProfilePage key={id} userID={id} onNavigate={navigate} /> : <Navigate to="/timeline" replace />
} })
const upload = createRoute({ getParentRoute: () => root, path: '/upload', component: () => {
  const { session, navigate } = useAppContext()
  return <UploadPage session={session} onNavigate={navigate} />
} })
const messages = createRoute({ getParentRoute: () => root, path: '/messages', component: () => {
  const { session, navigate, refreshUnreadCount } = useAppContext()
  return session.token && session.user ? <MessagesPage session={session} onNavigate={navigate} onUnreadChange={refreshUnreadCount} /> : <Navigate to="/auth" replace />
} })
const timeline = createRoute({ getParentRoute: () => root, path: '/timeline', component: () => <FeedRoute scene="timeline" /> })
const recommend = createRoute({ getParentRoute: () => root, path: '/recommend', component: () => <FeedRoute scene="recommend" /> })
const following = createRoute({ getParentRoute: () => root, path: '/following', component: () => <FeedRoute scene="following" /> })
const hot = createRoute({ getParentRoute: () => root, path: '/hotfeed', component: () => <FeedRoute scene="hot" /> })
const loginAlias = createRoute({ getParentRoute: () => root, path: '/login', component: () => <Navigate to="/auth" replace /> })
const meAlias = createRoute({ getParentRoute: () => root, path: '/me', component: () => <Navigate to="/profile" replace /> })
const routeTree = root.addChildren([index, auth, profile, publicProfile, upload, messages, timeline, recommend, following, hot, loginAlias, meAlias])
export function createAppRouter(history?: RouterHistory) {
  return createRouter({ routeTree, history, defaultPreload: 'intent' })
}
export const router = createAppRouter()
declare module '@tanstack/react-router' { interface Register { router: typeof router } }
