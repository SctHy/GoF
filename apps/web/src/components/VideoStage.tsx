import { Pause, Play, Volume2, VolumeX } from "lucide-react";
import { UserAvatar } from "@/components/UserAvatar";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { Icon } from "@/components/Icon";
import { Button } from "@/components/ui/button";
import type {
  FeedItem,
  PlaybackMetrics,
  PublicProfile,
  Action,
} from "../types.ts";
interface VideoStageProps {
  item: FeedItem;
  active: boolean;
  liked: boolean;
  favorited: boolean;
  following: boolean;
  followBusy: boolean;
  ownVideo: boolean;
  followError: string;
  onLike: Action;
  onComment: Action;
  onFavorite: Action;
  onFollow: Action;
  onPlaybackQoS?: (item: FeedItem, metrics: PlaybackMetrics) => void;
  onOpenAuthor: (profile: PublicProfile) => void;
}
import { isVideoSource } from "../lib/media.ts";
import { memo, useCallback, useEffect, useRef, useState } from "react";
import { DEFAULT_COVER_URL } from "../lib/constants.ts";
import { formatMetric } from "../lib/format.ts";
import { createVideoQoSState } from "../lib/playback.ts";
import { profileFromFeedItem } from "../lib/profiles.ts";

function VideoStageComponent({
  item,
  active,
  liked,
  favorited,
  following,
  followBusy,
  ownVideo,
  followError,
  onLike,
  onComment,
  onFavorite,
  onFollow,
  onPlaybackQoS,
  onOpenAuthor,
}: VideoStageProps) {
  const cover = item.cover_url || DEFAULT_COVER_URL;
  const media = item.media_url || cover;
  const showVideo = isVideoSource(media);
  const [paused, setPaused] = useState(false);
  const [muted, setMuted] = useState(true);
  const [progress, setProgress] = useState(0);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const itemRef = useRef(item);
  const qosRef = useRef(createVideoQoSState(item.video_id));

  useEffect(() => {
    itemRef.current = item;
  }, [item]);

  const flushQoS = useCallback(() => {
    const state = qosRef.current;
    if (!state || !state.playingStartedAt) return;
    const watchMs = Math.max(
      0,
      Math.round(performance.now() - state.playingStartedAt),
    );
    if (
      watchMs <= 0 &&
      state.stutterCount <= 0 &&
      state.firstFrameMs === undefined
    )
      return;
    onPlaybackQoS?.(itemRef.current, {
      firstFrameMs: state.firstFrameMs,
      stutterCount: state.stutterCount,
      watchMs,
      reportID: state.reportID,
    });
    qosRef.current = {
      ...createVideoQoSState(itemRef.current.video_id),
      loadStartedAt: performance.now(),
    };
  }, [onPlaybackQoS]);

  useEffect(() => {
    qosRef.current = createVideoQoSState(item.video_id);
    if (active && showVideo) {
      qosRef.current.loadStartedAt = performance.now();
    }
    return () => {
      flushQoS();
    };
  }, [active, flushQoS, item.video_id, showVideo]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video || !showVideo) return;
    if (active) {
      const playback = video.play();
      if (playback?.catch) {
        playback.catch(() => {});
      }
      return;
    }
    video.pause();
  }, [active, media, showVideo]);

  function handleLoadedData() {
    const state = qosRef.current;
    if (!active || !state || state.firstFrameMs !== undefined) return;
    const startedAt = state.loadStartedAt || performance.now();
    state.firstFrameMs = Math.max(0, Math.round(performance.now() - startedAt));
  }

  function handlePlaying() {
    const state = qosRef.current;
    if (!active || !state) return;
    if (!state.loadStartedAt) {
      state.loadStartedAt = performance.now();
    }
    if (!state.playingStartedAt) {
      state.playingStartedAt = performance.now();
    }
  }

  function handleWaiting() {
    const state = qosRef.current;
    if (!active || !state) return;
    state.stutterCount += 1;
  }

  return (
    <article className="video-stage dark" aria-label={item.title}>
      <img className="stage-backdrop" src={cover} alt="" />
      {showVideo ? (
        <video
          ref={videoRef}
          className="stage-media"
          src={media}
          poster={cover}
          autoPlay={active}
          muted={muted}
          loop
          playsInline
          preload={active ? "metadata" : "none"}
          onLoadedData={handleLoadedData}
          onPlaying={handlePlaying}
          onWaiting={handleWaiting}
          onPlay={() => setPaused(false)}
          onTimeUpdate={(event) => {
            const v = event.currentTarget;
            setProgress(
              v.duration > 0 ? (v.currentTime / v.duration) * 100 : 0,
            );
          }}
          onPause={() => {
            setPaused(true);
            flushQoS();
          }}
          onEnded={flushQoS}
        />
      ) : (
        <img className="stage-media" src={media} alt="" />
      )}
      <div className="stage-vignette" />
      {showVideo && (
        <div className="absolute top-4 right-4 z-10 flex gap-2">
          <Button
            variant="secondary"
            size="icon"
            className="rounded-full bg-black/35 text-white hover:bg-black/60"
            aria-label={paused ? "播放视频" : "暂停视频"}
            onClick={() => {
              const video = videoRef.current;
              if (video) {
                if (video.paused) void video.play().catch(() => {});
                else video.pause();
              }
            }}
          >
            {paused ? <Play /> : <Pause />}
          </Button>
          <Button
            variant="secondary"
            size="icon"
            className="rounded-full bg-black/35 text-white hover:bg-black/60"
            aria-label={muted ? "开启声音" : "静音"}
            onClick={() => setMuted(!muted)}
          >
            {muted ? <VolumeX /> : <Volume2 />}
          </Button>
        </div>
      )}
      <div className="stage-copy">
        <div className="mb-4 flex items-center gap-3">
          <Button
            variant="ghost"
            className="h-auto max-w-full gap-2.5 p-0 text-white hover:bg-transparent hover:text-white/80"
            onClick={() => onOpenAuthor(profileFromFeedItem(item))}
          >
            <UserAvatar
              src={item.avatar_url}
              name={item.author}
              className="size-9 ring-1 ring-white/20"
            />
            <span className="truncate">{item.author}</span>
          </Button>
          {!ownVideo && (
            <Button
              size="sm"
              variant={following ? "secondary" : "default"}
              className="rounded-full px-4"
              onClick={onFollow}
              disabled={followBusy}
            >
              {followBusy ? "处理中" : following ? "已关注" : "关注"}
            </Button>
          )}
        </div>
        {followError && (
          <p role="status" className="mb-2 text-sm text-rose-200">
            {followError}
          </p>
        )}
        <h1 className="text-xl font-semibold tracking-tight md:text-2xl">
          {item.title}
        </h1>
        {item.description && (
          <p className="mt-2 line-clamp-2 text-sm leading-relaxed text-white/70">
            {item.description}
          </p>
        )}
      </div>
      <div className="action-rail">
        <ActionButton
          icon="favorite"
          label={formatMetric(item.like_count)}
          active={liked}
          onClick={onLike}
        />
        <ActionButton
          icon="chat_bubble"
          label={formatMetric(item.comment_count)}
          onClick={onComment}
        />
        <ActionButton
          icon="bookmark"
          label={formatMetric(item.favorite_count)}
          active={favorited}
          onClick={onFavorite}
        />
      </div>
      {showVideo && (
        <div
          className="absolute inset-x-0 bottom-0 z-10 h-0.5 bg-white/15"
          role="progressbar"
          aria-label="播放进度"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.round(progress)}
        >
          <div
            className="h-full bg-white/80"
            style={{ width: `${progress}%` }}
          />
        </div>
      )}
    </article>
  );
}

export function ActionButton({
  icon,
  label,
  active,
  onClick,
}: {
  icon: string;
  label: string;
  active?: boolean;
  onClick?: Action;
}) {
  const actionLabel =
    (
      { favorite: "点赞", chat_bubble: "评论", bookmark: "收藏" } as Record<
        string,
        string
      >
    )[icon] || icon;
  return (
    <div className="grid justify-items-center gap-1.5 text-white">
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant="secondary"
              size="icon-lg"
              className={`size-11 rounded-full border border-white/10 bg-white/10 backdrop-blur-sm hover:bg-white/20 ${active ? "text-rose-400" : "text-white"}`}
              onClick={onClick}
              aria-label={actionLabel}
              aria-pressed={active}
            />
          }
        >
          <Icon name={icon} className={active ? "fill-current" : ""} />
        </TooltipTrigger>
        <TooltipContent>{actionLabel}</TooltipContent>
      </Tooltip>
      <span className="text-xs font-medium tabular-nums">{label}</span>
    </div>
  );
}

export const VideoStage = memo(VideoStageComponent);
