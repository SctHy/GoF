import type { PlaybackConfig, PreloadVideo, VideoQoSState, PlaybackMetrics, FeedItem } from "../types.ts";
type NetworkConnection = { effectiveType?: string; type?: string };
import { DEFAULT_PLAYBACK_CONFIG } from "./constants.ts";

export function normalizePlaybackConfig(config: Partial<PlaybackConfig> | null | undefined): PlaybackConfig {
  const preloadCount = Number(config?.preload_count);
  const bufferMs = Number(config?.buffer_ms);
  return {
    platform: config?.platform || DEFAULT_PLAYBACK_CONFIG.platform,
    network_type: config?.network_type || DEFAULT_PLAYBACK_CONFIG.network_type,
    preload_count: Number.isFinite(preloadCount) ? Math.max(1, Math.min(10, preloadCount)) : DEFAULT_PLAYBACK_CONFIG.preload_count,
    buffer_ms: Number.isFinite(bufferMs) && bufferMs > 0 ? bufferMs : DEFAULT_PLAYBACK_CONFIG.buffer_ms
  };
}

export function detectNetworkType() {
  const browser = navigator as Navigator & { connection?: NetworkConnection; mozConnection?: NetworkConnection; webkitConnection?: NetworkConnection };
  const connection = browser.connection || browser.mozConnection || browser.webkitConnection;
  const raw = String(connection?.effectiveType || connection?.type || "").toLowerCase();
  if (raw.includes("wifi")) return "WiFi";
  if (raw.includes("5g")) return "5G";
  if (raw.includes("4g")) return "4G";
  if (raw.includes("3g")) return "3G";
  return DEFAULT_PLAYBACK_CONFIG.network_type;
}

export function prewarmVideoAssets(items: PreloadVideo[], loadedSet: Set<string>) {
  const cleanups: (() => void)[] = [];
  for (const item of items) {
    const coverKey = `cover:${item.cover_url || ""}`;
    if (item.cover_url && !loadedSet.has(coverKey)) {
      const imagePreload = new Image();
      imagePreload.src = item.cover_url;
      loadedSet.add(coverKey);
      cleanups.push(() => { imagePreload.src = ""; });
    }
    const cleanup = prewarmVideoMetadata(item.media_url, loadedSet);
    if (cleanup) cleanups.push(cleanup);
  }
  // Bound dedup metadata for long browsing sessions.
  while (loadedSet.size > 120) loadedSet.delete(loadedSet.values().next().value!);
  return () => cleanups.forEach((cleanup) => cleanup());
}

export function prewarmVideoMetadata(url: string, loadedSet: Set<string>) {
  const mediaURL = String(url || "").trim();
  const mediaKey = `metadata:${mediaURL}`;
  if (!mediaURL || loadedSet.has(mediaKey) || typeof document === "undefined") return;
  loadedSet.add(mediaKey);

  const probe = document.createElement("video");
  probe.preload = "metadata";
  probe.src = mediaURL;
  probe.muted = true;
  probe.playsInline = true;
  probe.setAttribute("aria-hidden", "true");
  probe.style.position = "absolute";
  probe.style.width = "1px";
  probe.style.height = "1px";
  probe.style.opacity = "0";
  probe.style.pointerEvents = "none";

  let timer = 0;
  let disposed = false;
  const cleanup = () => {
    if (disposed) return;
    disposed = true;
    window.clearTimeout(timer);
    probe.removeEventListener("loadedmetadata", cleanup);
    probe.removeEventListener("canplay", cleanup);
    probe.removeEventListener("error", cleanup);
    probe.removeAttribute("src");
    probe.load();
    probe.remove();
  };

  probe.addEventListener("loadedmetadata", cleanup);
  probe.addEventListener("canplay", cleanup);
  probe.addEventListener("error", cleanup);
  timer = window.setTimeout(cleanup, 4000);
  document.body.appendChild(probe);
  probe.load();
  return cleanup;
}

export function createVideoQoSState(videoID: number): VideoQoSState {
  return {
    videoID,
    reportID: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    loadStartedAt: 0,
    playingStartedAt: 0,
    firstFrameMs: undefined,
    stutterCount: 0
  };
}

export function buildPlaybackQoSPayload(metrics: PlaybackMetrics) {
  const watchMs = Math.max(0, Math.round(Number(metrics?.watchMs || 0)));
  const stutterCount = Math.max(0, Math.round(Number(metrics?.stutterCount || 0)));
  const firstFrameNumber = Number(metrics?.firstFrameMs);
  const firstFrameMs = Number.isFinite(firstFrameNumber) ? Math.max(0, Math.round(firstFrameNumber)) : undefined;
  if (watchMs <= 0 && stutterCount <= 0 && firstFrameMs === undefined) return null;
  return {
    ...(firstFrameMs === undefined ? {} : { first_frame_ms: firstFrameMs }),
    stutter_count: stutterCount,
    watch_ms: watchMs
  };
}

export function createPlaybackQoSKey(item: FeedItem, metrics: PlaybackMetrics) {
  return `web-qos-${item.video_id}-${metrics?.reportID || Date.now()}`;
}
