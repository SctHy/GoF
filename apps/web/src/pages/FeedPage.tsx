import { ArrowDown, ArrowUp, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Kbd } from "@/components/ui/kbd";
import { errorMessage, errorStatus } from "../api/client.ts";
import type {
  PointerEvent as ReactPointerEvent,
  WheelEvent as ReactWheelEvent,
} from "react";
import type {
  PageProps,
  FeedScene,
  FeedItem,
  ActionMap,
  Comment,
  LoadState,
  Swipe,
  Drag,
  PublicProfile,
  PlaybackMetrics,
  InteractionResponse,
  FollowResponse,
  Page,
} from "../types.ts";
import { useFeedQuery } from "../hooks/useFeedQuery.ts";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/lib/queryClient";
import { memo, useCallback, useEffect, useRef, useState } from "react";
import { apiData, apiRequest } from "../api/client.ts";
import {
  fetchPlaybackConfig,
  fetchPreloadVideos,
  loadFollowingMap,
} from "../api/resources.ts";
import { CommentPanel } from "../components/CommentPanel.tsx";
import {
  FeedMessage,
  FeedPaginationNotice,
} from "../components/FeedMessage.tsx";
import { VideoStage } from "../components/VideoStage.tsx";
import {
  DEFAULT_PLAYBACK_CONFIG,
  FEED_SCENES,
  FEED_TRANSITION_MS,
  emptyProfile,
} from "../lib/constants.ts";
import {
  clampSwipeOffset,
  getFeedTrackStyle,
  isInteractiveTarget,
  requiresAuthFeed,
} from "../lib/feed.ts";
import {
  buildPlaybackQoSPayload,
  createPlaybackQoSKey,
  normalizePlaybackConfig,
  prewarmVideoAssets,
} from "../lib/playback.ts";
import {
  updateSessionRelationCount,
} from "../lib/profiles.ts";

export function FeedPage({
  feedScene,
  session,
  onNavigate,
}: PageProps & { feedScene: FeedScene }) {
  const [index, setIndex] = useState(0);
  const [followBusyID, setFollowBusyID] = useState(0);
  const [followError, setFollowError] = useState("");
  const [commentsOpen, setCommentsOpen] = useState(false);
  const [commentsError, setCommentsError] = useState("");
  const [commentText, setCommentText] = useState("");
  const [swipe, setSwipe] = useState<Swipe | null>(null);
  const isSwiping = Boolean(swipe);
  const wheelLocked = useRef(false);
  const transitionTimer = useRef<number | null>(null);
  const feedMainRef = useRef<HTMLElement | null>(null);
  const dragRef = useRef<Drag | null>(null);
  const swipeRef = useRef<Swipe | null>(null);
  const exposedRef = useRef(new Set<string>());
  const preloadedVideoRef = useRef(new Set<string>());
  const currentFeedScene =
    FEED_SCENES.find((scene) => scene.key === feedScene) || FEED_SCENES[0];

  const client = useQueryClient();
  const onReset = useCallback(() => {
    setSwipe(null);
    setIndex(0);
    setCommentsOpen(false);
    exposedRef.current = new Set<string>();
    if (transitionTimer.current) window.clearTimeout(transitionTimer.current);
  }, []);
  const onUnauthorized = useCallback(() => {
    session.clearAuth();
    onNavigate("/auth");
  }, [session.clearAuth, onNavigate]);
  const {
    items,
    updateItem: updateCurrentItem,
    feedState,
    feedError,
    paginationError,
    hasMore,
    loadingMore,
    feedRequestID,
    loadFeed,
    loadMoreFeed,
  } = useFeedQuery({
    scene: feedScene,
    token: session.token,
    onReset,
    onUnauthorized,
  });
  const openComments = useCallback(() => setCommentsOpen(true), []);
  const openAuthor = useCallback(
    (profile: PublicProfile) => {
      client.setQueryData(
        queryKeys.publicProfile(profile.id),
        (previous) => previous || profile,
        { updatedAt: 0 },
      );
      onNavigate(`/users/${profile.id}`);
    },
    [client, onNavigate],
  );
  const followingQuery = useQuery({
    queryKey: queryKeys.following,
    enabled: !!session.token,
    queryFn: ({ signal }) => loadFollowingMap(session.token, signal),
  });
  const following = followingQuery.data || {};
  const playbackQuery = useQuery({
    queryKey: queryKeys.playback,
    enabled: !!session.token,
    queryFn: ({ signal }) => fetchPlaybackConfig(session.token, signal),
  });
  const playbackConfig = normalizePlaybackConfig(
    playbackQuery.data || DEFAULT_PLAYBACK_CONFIG,
  );
  useEffect(() => {
    if (
      [followingQuery.error, playbackQuery.error].some(
        (error) => errorStatus(error) === 401,
      )
    )
      onUnauthorized();
  }, [followingQuery.error, playbackQuery.error, onUnauthorized]);

  useEffect(() => {
    if (
      feedState === "ready" &&
      items.length > 0 &&
      index >= items.length - 3
    ) {
      loadMoreFeed();
    }
  }, [feedState, index, items.length, loadMoreFeed]);

  useEffect(() => {
    return () => {
      if (transitionTimer.current) {
        window.clearTimeout(transitionTimer.current);
      }
    };
  }, []);

  useEffect(() => {
    swipeRef.current = swipe;
  }, [swipe]);

  useEffect(() => {
    if (!swipe && index >= items.length && items.length > 0) {
      setIndex(items.length - 1);
    }
  }, [index, items.length, swipe]);

  const current = items[index];
  const visibleCurrent = swipe ? items[swipe.fromIndex] : current;
  const visibleNext = swipe ? items[swipe.toIndex] : null;
  const trackStyle = getFeedTrackStyle(swipe);

  useEffect(() => {
    if (!current || feedState !== "ready" || !session.token) return;
    const scene = current.feed_scene || feedScene;
    const requestID = current.request_id || feedRequestID;
    const exposureKey = `${scene}:${requestID}:${current.video_id}`;
    if (!requestID || exposedRef.current.has(exposureKey)) return;
    exposedRef.current.add(exposureKey);
    apiRequest("/api/video-view-events", {
      method: "POST",
      token: session.token,
      body: {
        video_id: current.video_id,
        scene,
        request_id: requestID,
        event_type: "exposed",
        watch_ms: 0,
        completed: false,
      },
    }).catch((error: unknown) => {
      if (errorStatus(error) === 401 && requiresAuthFeed(feedScene)) {
        session.clearAuth();
        onNavigate("/auth");
      }
    });
  }, [current, feedRequestID, feedScene, feedState, onNavigate, session]);

  const preloadQuery = useQuery({
    queryKey: [
      "playback",
      "preload",
      current?.video_id,
      playbackConfig.preload_count,
    ],
    enabled: !!current && feedState === "ready" && !!session.token,
    queryFn: ({ signal }) =>
      fetchPreloadVideos(
        session.token,
        current!.video_id,
        playbackConfig.preload_count,
        signal,
      ),
  });
  useEffect(() => {
    if (!preloadQuery.data) return;
    return prewarmVideoAssets(
      preloadQuery.data.items,
      preloadedVideoRef.current,
    );
  }, [preloadQuery.data]);
  useEffect(() => {
    if (errorStatus(preloadQuery.error) === 401 && requiresAuthFeed(feedScene))
      onUnauthorized();
  }, [preloadQuery.error, feedScene, onUnauthorized]);

  const reportPlaybackQoS = useCallback(
    (item: FeedItem, metrics: PlaybackMetrics) => {
      if (!session.token || !item?.video_id) return;
      const payload = buildPlaybackQoSPayload(metrics);
      if (!payload) return;
      apiRequest("/api/playback-qos-reports", {
        method: "POST",
        token: session.token,
        headers: {
          "Idempotency-Key": createPlaybackQoSKey(item, metrics),
        },
        body: {
          video_id: item.video_id,
          ...payload,
        },
      }).catch((error: unknown) => {
        if (errorStatus(error) === 401) {
          session.clearAuth();
          onNavigate("/auth");
        }
      });
    },
    [onNavigate, session.clearAuth, session.token],
  );

  const requireLogin = useCallback(() => {
    if (session.token) return true;
    onNavigate("/auth");
    return false;
  }, [onNavigate, session.token]);

  const setLike = useCallback(async () => {
    if (!current || isSwiping || !requireLogin()) return;
    try {
      const nextLiked = !current.liked;
      const data = await apiData<InteractionResponse>(
        `/api/videos/${current.video_id}/like`,
        {
          method: nextLiked ? "PUT" : "DELETE",
          token: session.token,
          headers: {
            "Idempotency-Key": `web-like-${current.video_id}-${Date.now()}`,
          },
        },
      );
      updateCurrentItem(current.video_id, {
        liked: data.active,
        like_count: data.like_count ?? current.like_count,
      });
    } catch (error) {
      if (errorStatus(error) === 401) {
        session.clearAuth();
        onNavigate("/auth");
      }
    }
  }, [
    current,
    onNavigate,
    requireLogin,
    session,
    isSwiping,
    updateCurrentItem,
  ]);

  const setFavorite = useCallback(async () => {
    if (!current || isSwiping || !requireLogin()) return;
    try {
      const nextFavorited = !current.favorited;
      const data = await apiData<InteractionResponse>(
        `/api/videos/${current.video_id}/favorite`,
        {
          method: nextFavorited ? "PUT" : "DELETE",
          token: session.token,
          headers: {
            "Idempotency-Key": `web-favorite-${current.video_id}-${Date.now()}`,
          },
        },
      );
      updateCurrentItem(current.video_id, {
        favorited: data.active,
        favorite_count: data.favorite_count ?? current.favorite_count,
      });
    } catch (error) {
      if (errorStatus(error) === 401) {
        session.clearAuth();
        onNavigate("/auth");
      }
    }
  }, [
    current,
    onNavigate,
    requireLogin,
    session,
    isSwiping,
    updateCurrentItem,
  ]);

  const setFollow = useCallback(async () => {
    if (!current || isSwiping || !requireLogin()) return;
    if (current.author_id === session.user?.id) return;

    const authorID = current.author_id;
    const nextFollowing = !Boolean(following[authorID]);
    setFollowBusyID(authorID);
    setFollowError("");
    try {
      const data = await apiData<FollowResponse>(
        `/api/users/me/following/${authorID}`,
        {
          method: nextFollowing ? "PUT" : "DELETE",
          token: session.token,
          headers: {
            "Idempotency-Key": `web-follow-${authorID}-${Date.now()}`,
          },
        },
      );
      client.setQueryData<ActionMap>(queryKeys.following, (state) => ({
        ...state,
        [authorID]: data.following,
      }));
      void client.invalidateQueries({ queryKey: ["relations", "list"] });
      updateSessionRelationCount(session, data.following_count);
    } catch (error) {
      if (errorStatus(error) === 401) {
        session.clearAuth();
        onNavigate("/auth");
        return;
      }
      setFollowError(errorMessage(error) || "关注操作失败");
    } finally {
      setFollowBusyID(0);
    }
  }, [
    client,
    current,
    following,
    onNavigate,
    requireLogin,
    session,
    isSwiping,
  ]);

  const commentsQuery = useQuery({
    queryKey: queryKeys.comments(current?.video_id || 0),
    enabled: !!current && commentsOpen,
    queryFn: ({ signal }) =>
      apiData<Page<Comment>>(
        `/api/videos/${current!.video_id}/comments?limit=50`,
        { signal },
      ),
  });
  const comments = commentsQuery.data?.items || [];
  const commentsState: LoadState = commentsQuery.isPending
    ? "loading"
    : commentsQuery.isError || commentsError
      ? "error"
      : "ready";
  const loadComments = () => {
    setCommentsError("");
    void commentsQuery.refetch();
  };
  useEffect(() => {
    setCommentsError("");
  }, [current?.video_id]);
  useEffect(() => {
    const page = commentsQuery.data;
    if (current && page && !page.has_more) {
      updateCurrentItem(current.video_id, (item) => ({
        comment_count: Math.max(page.items.length, item.comment_count),
      }));
    }
  }, [commentsQuery.data, current?.video_id, updateCurrentItem]);

  async function submitComment() {
    if (!current || !requireLogin()) return;
    const content = commentText.trim();
    if (!content) return;
    try {
      const data = await apiData<Comment>(
        `/api/videos/${current.video_id}/comments`,
        {
          method: "POST",
          token: session.token,
          headers: {
            "Idempotency-Key": `web-comment-${current.video_id}-${Date.now()}`,
          },
          body: {
            content,
          },
        },
      );
      setCommentText("");
      client.setQueryData<Page<Comment>>(
        queryKeys.comments(current.video_id),
        (page) => ({
          items: [
            data,
            ...(page?.items || []).filter((item) => item.id !== data.id),
          ],
          next_cursor: page?.next_cursor || "",
          has_more: page?.has_more || false,
        }),
      );
      updateCurrentItem(current.video_id, {
        comment_count: data.comment_count ?? current.comment_count + 1,
      });
      setCommentsError("");
    } catch (error) {
      if (errorStatus(error) === 401) {
        session.clearAuth();
        onNavigate("/auth");
        return;
      }
      setCommentsError(errorMessage(error) || "评论发布失败");
    }
  }

  function getStageHeight() {
    return feedMainRef.current?.clientHeight || window.innerHeight || 1;
  }

  function moveTo(nextIndex: number) {
    if (
      swipe ||
      nextIndex === index ||
      nextIndex < 0 ||
      nextIndex >= items.length
    )
      return;
    const direction = nextIndex > index ? "next" : "prev";
    const height = getStageHeight();
    if (transitionTimer.current) {
      window.clearTimeout(transitionTimer.current);
    }
    setSwipe({
      fromIndex: index,
      toIndex: nextIndex,
      direction,
      height,
      offset: 0,
      settling: "",
    });
    window.requestAnimationFrame(() => {
      setSwipe((state) =>
        state && state.fromIndex === index && state.toIndex === nextIndex
          ? {
              ...state,
              offset: direction === "next" ? -height : height,
              settling: "commit",
            }
          : state,
      );
    });
    transitionTimer.current = window.setTimeout(() => {
      setIndex(nextIndex);
      setSwipe(null);
      transitionTimer.current = null;
    }, FEED_TRANSITION_MS);
  }

  function settleSwipe(commit: boolean) {
    const active = swipeRef.current;
    if (!active) return;
    if (transitionTimer.current) {
      window.clearTimeout(transitionTimer.current);
    }
    setSwipe({
      ...active,
      offset: commit
        ? active.direction === "next"
          ? -active.height
          : active.height
        : 0,
      settling: commit ? "commit" : "cancel",
    });
    transitionTimer.current = window.setTimeout(() => {
      if (commit) {
        setIndex(active.toIndex);
      }
      setSwipe(null);
      transitionTimer.current = null;
    }, FEED_TRANSITION_MS);
  }

  function handlePointerDown(event: ReactPointerEvent<HTMLElement>) {
    if (
      event.button > 0 ||
      swipe ||
      items.length < 2 ||
      isInteractiveTarget(event.target)
    )
      return;
    dragRef.current = {
      pointerId: event.pointerId,
      startY: event.clientY,
      fromIndex: index,
      active: false,
      direction: "",
      height: 0,
    };
    event.currentTarget.setPointerCapture(event.pointerId);
  }

  function handlePointerMove(event: ReactPointerEvent<HTMLElement>) {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) return;
    const delta = event.clientY - drag.startY;
    if (!drag.active) {
      if (Math.abs(delta) < 8) return;
      const direction = delta < 0 ? "next" : "prev";
      const toIndex =
        direction === "next" ? drag.fromIndex + 1 : drag.fromIndex - 1;
      if (toIndex < 0 || toIndex >= items.length) {
        return;
      }
      const height = getStageHeight();
      dragRef.current = {
        ...drag,
        active: true,
        direction,
        toIndex,
        height,
      };
      setSwipe({
        fromIndex: drag.fromIndex,
        toIndex,
        direction,
        height,
        offset: clampSwipeOffset(direction, delta, height),
        settling: "",
      });
      event.preventDefault();
      return;
    }

    setSwipe((state) =>
      state
        ? {
            ...state,
            offset: clampSwipeOffset(state.direction, delta, state.height),
            settling: "",
          }
        : state,
    );
    event.preventDefault();
  }

  function handlePointerEnd(event: ReactPointerEvent<HTMLElement>) {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) return;
    dragRef.current = null;
    const active = swipeRef.current;
    if (!drag.active || !active) return;
    const threshold = Math.min(active.height * 0.24, 220);
    settleSwipe(Math.abs(active.offset) >= threshold);
  }

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (commentsOpen && event.key !== "Escape") return;
      if (
        event.defaultPrevented ||
        event.isComposing ||
        (event.key !== "Escape" && isInteractiveTarget(event.target))
      )
        return;
      if (["ArrowDown", "j", "J"].includes(event.key)) {
        event.preventDefault();
        moveTo(Math.min(items.length - 1, index + 1));
      }
      if (["ArrowUp", "k", "K"].includes(event.key)) {
        event.preventDefault();
        moveTo(Math.max(0, index - 1));
      }
      if (event.key === "l" || event.key === "L") {
        setLike();
      }
      if (event.key === "f" || event.key === "F") {
        setFavorite();
      }
      if (event.key === "r" || event.key === "R") {
        setFollow();
      }
      if (event.key === "c" || event.key === "C") {
        setCommentsOpen(true);
      }
      if (event.key === "Escape") {
        setCommentsOpen(false);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [
    index,
    items.length,
    setFavorite,
    setFollow,
    setLike,
    isSwiping,
    commentsOpen,
  ]);

  function goNext() {
    moveTo(Math.min(items.length - 1, index + 1));
  }

  function goPrev() {
    moveTo(Math.max(0, index - 1));
  }

  function handleWheel(event: ReactWheelEvent<HTMLElement>) {
    if (
      Math.abs(event.deltaY) < 32 ||
      wheelLocked.current ||
      swipe ||
      items.length < 2
    )
      return;
    wheelLocked.current = true;
    if (event.deltaY > 0) {
      goNext();
    } else {
      goPrev();
    }
    window.setTimeout(() => {
      wheelLocked.current = false;
    }, 420);
  }

  return (
    <div className="page-container max-w-7xl! space-y-5!">
      <div className="page-heading">
        <div>
          <p className="mb-1 text-xs font-medium tracking-widest text-muted-foreground">
            DISCOVER
          </p>
          <h2 className="text-2xl font-semibold tracking-tight">
            {currentFeedScene.label}
          </h2>
        </div>
        <Button
          variant="outline"
          onClick={loadFeed}
          disabled={feedState === "loading"}
        >
          <RefreshCw />
          刷新内容
        </Button>
      </div>
      <section
        className="feed-main"
        ref={feedMainRef}
        onWheel={handleWheel}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerEnd}
        onPointerCancel={handlePointerEnd}
      >
        {feedState === "loading" && (
          <FeedMessage
            icon="hourglass_top"
            title={`正在加载${currentFeedScene.label}`}
          />
        )}
        {feedState === "auth" && (
          <FeedMessage
            icon="lock"
            title={`登录后查看${currentFeedScene.label}`}
            action="登录"
            onAction={() => onNavigate("/auth")}
          />
        )}
        {feedState === "error" && (
          <FeedMessage
            icon="sync_problem"
            title={feedError}
            action="重新加载"
            onAction={loadFeed}
          />
        )}
        {feedState === "ready" && items.length === 0 && (
          <FeedMessage
            icon="video_library"
            title={`${currentFeedScene.label}暂无视频`}
            action="刷新"
            onAction={loadFeed}
          />
        )}
        {visibleCurrent && (
          <div
            className={`feed-stage-wrap ${swipe ? `swiping ${swipe.direction} ${swipe.settling ? "settling" : "dragging"}` : ""}`}
          >
            <div className="feed-stage-track" style={trackStyle}>
              {swipe?.direction === "prev" && visibleNext && (
                <div className="feed-stage-layer">
                  <VideoStage
                    item={visibleNext}
                    active={false}
                    liked={visibleNext.liked}
                    favorited={visibleNext.favorited}
                    following={Boolean(following[visibleNext.author_id])}
                    followBusy={followBusyID === visibleNext.author_id}
                    ownVideo={visibleNext.author_id === session.user?.id}
                    onLike={setLike}
                    onComment={openComments}
                    onFavorite={setFavorite}
                    onFollow={setFollow}
                    onPlaybackQoS={reportPlaybackQoS}
                    onOpenAuthor={openAuthor}
                    followError={followError}
                  />
                </div>
              )}
              <div className="feed-stage-layer">
                <VideoStage
                  item={visibleCurrent}
                  active={!swipe}
                  liked={visibleCurrent.liked}
                  favorited={visibleCurrent.favorited}
                  following={Boolean(following[visibleCurrent.author_id])}
                  followBusy={followBusyID === visibleCurrent.author_id}
                  ownVideo={visibleCurrent.author_id === session.user?.id}
                  onLike={setLike}
                  onComment={openComments}
                  onFavorite={setFavorite}
                  onFollow={setFollow}
                  onPlaybackQoS={reportPlaybackQoS}
                  onOpenAuthor={openAuthor}
                  followError={followError}
                />
              </div>
              {swipe?.direction === "next" && visibleNext && (
                <div className="feed-stage-layer">
                  <VideoStage
                    item={visibleNext}
                    active={false}
                    liked={visibleNext.liked}
                    favorited={visibleNext.favorited}
                    following={Boolean(following[visibleNext.author_id])}
                    followBusy={followBusyID === visibleNext.author_id}
                    ownVideo={visibleNext.author_id === session.user?.id}
                    onLike={setLike}
                    onComment={openComments}
                    onFavorite={setFavorite}
                    onFollow={setFollow}
                    onPlaybackQoS={reportPlaybackQoS}
                    onOpenAuthor={openAuthor}
                    followError={followError}
                  />
                </div>
              )}
            </div>
          </div>
        )}
        <FeedPaginationNotice error={paginationError} onRefresh={loadFeed} />
        {feedState === "ready" && loadingMore && (
          <div className="feed-loading-pill">加载中</div>
        )}
        {feedState === "ready" &&
          items.length > 0 &&
          !hasMore &&
          !paginationError &&
          index === items.length - 1 && (
            <div className="feed-loading-pill">已到末尾</div>
          )}
      </section>
      <div className="flex items-center justify-between gap-3">
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <span className="hidden sm:inline">滚动或使用</span>
          <Kbd>↑</Kbd>
          <Kbd>↓</Kbd>
          <span>切换视频</span>
        </p>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            aria-label="上一个视频"
            disabled={index === 0 || !!swipe}
            onClick={goPrev}
          >
            <ArrowUp />
            <span className="hidden sm:inline">上一条</span>
          </Button>
          <Button
            variant="outline"
            size="sm"
            aria-label="下一个视频"
            disabled={!!swipe || (!hasMore && index >= items.length - 1)}
            onClick={goNext}
          >
            <ArrowDown />
            <span className="hidden sm:inline">下一条</span>
          </Button>
        </div>
      </div>
      <CommentPanel
        open={commentsOpen}
        value={commentText}
        onChange={setCommentText}
        onClose={() => setCommentsOpen(false)}
        onSubmit={submitComment}
        user={session.user || emptyProfile}
        count={current?.comment_count || 0}
        comments={comments}
        state={commentsState}
        error={commentsError || errorMessage(commentsQuery.error)}
        onRetry={loadComments}
        authenticated={Boolean(session.token && session.user)}
        onOpenUser={openAuthor}
      />
    </div>
  );
}

export default memo(FeedPage);
