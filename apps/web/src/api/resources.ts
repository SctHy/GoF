import type { ActionMap, FeedScene, FeedItemResponse, Page, Message, PlaybackConfig, PreloadVideo, RelationUser, RelationTab } from "../types.ts";
import { DEFAULT_PLAYBACK_CONFIG } from "../lib/constants.ts";
import { detectNetworkType } from "../lib/playback.ts";
import { apiData, apiRequest } from "./client.ts";

export async function fetchFeedPage(scene: FeedScene, token: string, cursor = "", requestID = "", signal?: AbortSignal) {
  const limit = 10;
  if (scene === "recommend") {
    return apiData<Page<FeedItemResponse>>("/api/feed-queries", {
      method: "POST",
      token,
      signal,
      body: {
        scene,
        cursor,
        limit,
        context: {
          request_id: requestID
        }
      }
    });
  }

  const params = new URLSearchParams({ scene, limit: String(limit) });
  if (cursor) {
    params.set("cursor", cursor);
  }
  return apiData<Page<FeedItemResponse>>(`/api/feed-items?${params.toString()}`, { token, signal });
}

export async function fetchMessages(token: string, cursor = "", signal?: AbortSignal) {
  const params = new URLSearchParams({ limit: "20" });
  if (cursor) {
    params.set("cursor", cursor);
  }
  return apiData<Page<Message>>(`/api/messages?${params.toString()}`, { token, signal });
}

export async function fetchPlaybackConfig(token: string, signal?: AbortSignal) {
  const params = new URLSearchParams({
    platform: "Web",
    network_type: detectNetworkType()
  });
  return apiData<PlaybackConfig>(`/api/playback-config?${params.toString()}`, { token, signal });
}

export async function fetchPreloadVideos(token: string, currentVideoID: number, limit: number, signal?: AbortSignal) {
  const params = new URLSearchParams({
    current_video_id: String(currentVideoID || 0),
    limit: String(limit || DEFAULT_PLAYBACK_CONFIG.preload_count)
  });
  return apiData<{ items: PreloadVideo[] }>(`/api/preload-videos?${params.toString()}`, { token, signal });
}

export async function markMessagesRead(token: string, messageIDs: number[] = []) {
  return apiRequest("/api/messages", {
    method: "PATCH",
    token,
    body: {
      message_ids: messageIDs
    }
  });
}

export async function loadFollowingMap(token: string, signal?: AbortSignal) {
  const next: ActionMap = {};
  let cursor = "";
  for (let page = 0; page < 20; page++) {
    const data = await apiData<Page<RelationUser>>(relationListPath("following", cursor, 100), { token, signal });
    for (const item of data.items || []) {
      next[item.user_id] = true;
    }
    if (!data.has_more || !data.next_cursor) {
      break;
    }
    cursor = data.next_cursor;
  }
  return next;
}

export function relationListPath(tab: RelationTab, cursor = "", limit = 20) {
  const params = new URLSearchParams({ limit: String(limit) });
  if (cursor) {
    params.set("cursor", cursor);
  }
  const resource = tab === "followers" ? "followers" : "following";
  return `/api/users/me/${resource}?${params.toString()}`;
}
