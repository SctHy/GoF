import type { FeedItemResponse, Session, Video } from "../src/types.ts";

export const feedResponse: FeedItemResponse = {
  video_id: 1, author_id: 2, author_nickname: "creator", author_avatar_url: "",
  title: "Regression video", description: "", media_url: "/uploads/example.mp4", cover_url: "",
  like_count: 0, comment_count: 0, favorite_count: 0, liked: false, favorited: false,
  published_at: "2026-11-17T00:00:00Z"
};
export const video: Video = {
  id: 1, author_id: 2, title: feedResponse.title, description: "", media_url: feedResponse.media_url,
  cover_url: "", status: 1, like_count: 0, comment_count: 0, favorite_count: 0,
  created_at: "2026-11-17T00:00:00Z", updated_at: "2026-11-17T00:00:00Z"
};
export const session: Session = {
  token: "test",
  user: { id: 1, nickname: "reader", account: "reader", avatar_url: "", bio: "", status: 1, role: "user" },
  setAuth() {}, clearAuth() {}
};
