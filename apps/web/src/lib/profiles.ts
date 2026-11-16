import type { User, PublicProfile, FeedItem, Comment, Session } from "../types.ts";
import { USER_KEY } from "./constants.ts";

export function readStoredUser(): User | null {
  try {
    const raw = localStorage.getItem(USER_KEY);
    return raw ? JSON.parse(raw) as User : null;
  } catch {
    return null;
  }
}

export function profileFromFeedItem(item: FeedItem): PublicProfile {
  return {
    id: item.author_id,
    nickname: item.author,
    avatar_url: item.avatar_url,
    bio: ""
  };
}

export function profileFromComment(comment: Comment): PublicProfile {
  return {
    id: comment.user_id,
    nickname: comment.user_nickname || `用户_${comment.user_id}`,
    avatar_url: comment.user_avatar_url || "",
    bio: ""
  };
}

export function updateSessionRelationCount(session: Session, followingCount: number) {
  if (!session.user) return;
  session.setAuth(session.token, {
    ...session.user,
    following_count: followingCount
  });
}
