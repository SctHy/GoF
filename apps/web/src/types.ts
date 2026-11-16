// API fields mirror apps/api/internal/interfaces/http/*/dto.go.
export type FeedScene = "timeline" | "recommend" | "following" | "hot";
export type LoadState = "idle" | "loading" | "loadingMore" | "ready" | "error" | "auth";
export type RelationTab = "following" | "followers";
export type ActionMap = Record<number, boolean>;
export type Navigate = (path: string) => void;
export type Action = () => void;

export interface PublicProfile {
  id: number;
  nickname: string;
  avatar_url: string;
  bio: string;
  following_count?: number;
  follower_count?: number;
  work_count?: number;
}
export interface User extends PublicProfile {
  account: string;
  status: number;
  role: string;
}
export interface Session {
  token: string;
  user: User | null;
  setAuth: (token: string, user: User | null) => void;
  clearAuth: Action;
}
export interface PageProps { session: Session; onNavigate: Navigate }
export interface MessageProps { icon: string; title: string; action?: string; onAction?: Action }
export interface Page<T> { items: T[]; next_cursor: string; has_more: boolean }
export interface Video {
  id: number;
  author_id: number;
  title: string;
  description: string;
  media_url: string;
  cover_url: string;
  status: number;
  like_count: number;
  comment_count: number;
  favorite_count: number;
  published_at?: string;
  created_at: string;
  updated_at: string;
}
export interface VideoList { items: Video[]; limit: number; offset: number }
export interface FeedItemResponse {
  video_id: number;
  author_id: number;
  author_nickname: string;
  author_avatar_url: string;
  title: string;
  description: string;
  media_url: string;
  cover_url: string;
  like_count: number;
  comment_count: number;
  favorite_count: number;
  liked: boolean;
  favorited: boolean;
  published_at: string;
}
export interface FeedItem extends Omit<FeedItemResponse, "author_nickname" | "author_avatar_url" | "published_at"> {
  author: string;
  avatar_url: string;
  feed_scene: FeedScene;
  request_id: string;
}
export interface Comment {
  id: number;
  video_id: number;
  user_id: number;
  user_nickname: string;
  user_avatar_url: string;
  content: string;
  created_at: string;
  comment_count?: number;
}
export interface Message {
  id: number;
  user_id: number;
  type: string;
  title: string;
  content: string;
  actor_id?: number;
  actor_nickname?: string;
  actor_avatar_url?: string;
  is_read: boolean;
  created_at: string;
  read_at?: string;
}
export interface RelationUser {
  user_id: number;
  nickname: string;
  avatar_url: string;
  bio: string;
  followed_at: string;
}
export interface FollowResponse { following: boolean; following_count: number; follower_count: number }
export interface InteractionResponse { active: boolean; like_count: number; favorite_count: number }
export interface PlaybackConfig { platform: string; network_type: string; preload_count: number; buffer_ms: number }
export interface PreloadVideo { video_id: number; media_url: string; cover_url: string }
export interface PlaybackMetrics { watchMs?: number; stutterCount?: number; firstFrameMs?: number; reportID?: string }
export interface VideoQoSState {
  videoID: number;
  reportID: string;
  loadStartedAt: number;
  playingStartedAt: number;
  firstFrameMs?: number;
  stutterCount: number;
}
export type SwipeDirection = "next" | "prev";
export interface Swipe {
  fromIndex: number;
  toIndex: number;
  direction: SwipeDirection;
  height: number;
  offset: number;
  settling: "" | "commit" | "cancel";
}
export interface Drag {
  pointerId: number;
  startY: number;
  fromIndex: number;
  active: boolean;
  direction: SwipeDirection | "";
  height: number;
  toIndex?: number;
}
