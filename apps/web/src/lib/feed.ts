import type { CSSProperties } from "react";
import type { FeedScene, FeedItem, FeedItemResponse, Swipe, SwipeDirection } from "../types.ts";
import { errorMessage, errorStatus } from "../api/client.ts";
import { FEED_TRANSITION_MS } from "./constants.ts";

export function requiresAuthFeed(scene: FeedScene) {
  return scene === "following" || scene === "recommend";
}

export function createFeedRequestID(scene: FeedScene) {
  const random = Math.random().toString(36).slice(2, 8);
  return `web-${scene}-${Date.now()}-${random}`;
}

export function appendFeedItems<T extends { video_id: number }>(currentItems: T[], nextItems: T[]) {
  if (!nextItems.length) return currentItems;
  const seen = new Set(currentItems.map((item) => item.video_id));
  const merged = [...currentItems];
  for (const item of nextItems) {
    if (seen.has(item.video_id)) continue;
    seen.add(item.video_id);
    merged.push(item);
  }
  return merged;
}

export function mapFeedItem(item: FeedItemResponse, feedScene: FeedScene = "timeline", requestID = ""): FeedItem {
  return {
    video_id: item.video_id,
    author_id: item.author_id,
    title: item.title,
    media_url: item.media_url,
    cover_url: item.cover_url,
    like_count: item.like_count,
    comment_count: item.comment_count,
    favorite_count: item.favorite_count,
    liked: Boolean(item.liked),
    favorited: Boolean(item.favorited),
    author: item.author_nickname || `创作者_${item.author_id}`,
    avatar_url: item.author_avatar_url || "",
    description: item.description || "",
    feed_scene: feedScene,
    request_id: requestID
  };
}

export function getFeedTrackStyle(swipe: Swipe | null): CSSProperties {
  if (!swipe) {
    return {
      transform: "translate3d(0, 0, 0)"
    };
  }
  const base = swipe.direction === "prev" ? -swipe.height : 0;
  return {
    transform: `translate3d(0, ${base + swipe.offset}px, 0)`,
    transition: swipe.settling ? `transform ${FEED_TRANSITION_MS}ms cubic-bezier(0.16, 1, 0.3, 1)` : "none"
  };
}

export function clampSwipeOffset(direction: SwipeDirection, delta: number, height: number) {
  if (direction === "next") {
    return Math.max(-height, Math.min(0, delta));
  }
  return Math.min(height, Math.max(0, delta));
}

export function isInteractiveTarget(target: EventTarget | null) {
  return target instanceof Element && Boolean(target.closest("button, a, input, textarea, select, [role=dialog]"));
}

export function paginationErrorMessage(error: unknown) {
  if (errorStatus(error) === 400 && /cursor/i.test(String(errorMessage(error) || ""))) {
    return "分页已过期，请刷新";
  }
  return "加载更多失败，请刷新重试";
}
