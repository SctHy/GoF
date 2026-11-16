import { Heart, MessageCircle, Play, X } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogHeader,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { UserAvatar } from "@/components/UserAvatar";
import { PageMessage } from "@/components/PageMessage";
import { isVideoSource } from "@/lib/media";
import { DEFAULT_COVER_URL } from "@/lib/constants";
import { formatMetric, formatRelativeTime } from "@/lib/format";
import type {
  Video,
  RelationTab,
  RelationUser,
  LoadState,
  ActionMap,
  Action,
} from "@/types";

type RelationListProps = {
  tab: RelationTab;
  items: RelationUser[];
  state: LoadState;
  error: string;
  hasMore: boolean;
  following: ActionMap;
  busyID: number;
  currentUserID: number;
  onRetry: Action;
  onLoadMore: Action;
  onToggleFollow: (userID: number) => void;
};
type RelationModalProps = RelationListProps & {
  onTabChange: (tab: RelationTab) => void;
  onClose: Action;
};

export function VideoGrid({
  videos,
  state,
  onSelect,
}: {
  videos: Video[];
  state: string;
  onSelect: (video: Video) => void;
}) {
  if (state === "loading")
    return (
      <div
        className="grid grid-cols-2 gap-4 md:grid-cols-3"
        aria-label="正在加载作品"
      >
        {[1, 2, 3].map((id) => (
          <Skeleton key={id} className="aspect-4/3 rounded-xl" />
        ))}
      </div>
    );
  if (state !== "ready")
    return <PageMessage icon="sync_problem" title={state} />;
  if (!videos.length)
    return (
      <PageMessage icon="video_library" title="暂无作品，发布第一条视频吧。" />
    );
  return (
    <div className="grid grid-cols-1 gap-5 min-[480px]:grid-cols-2 xl:grid-cols-3">
      {videos.map((video) => (
        <Card
          key={video.id}
          className="group relative gap-0 overflow-hidden py-0 shadow-none transition-colors duration-150 hover:bg-muted/40"
        >
          <div className="relative aspect-video overflow-hidden bg-muted">
            <img
              className="size-full object-cover"
              src={video.cover_url || DEFAULT_COVER_URL}
              alt=""
              loading="lazy"
            />
            <span className="absolute inset-0 flex items-center justify-center bg-black/5">
              <span className="flex size-10 items-center justify-center rounded-full bg-black/60 text-white">
                <Play className="size-4 fill-current" />
              </span>
            </span>
            <Badge variant="secondary" className="absolute top-3 right-3">
              {video.status === 0 ? "审核中" : "已发布"}
            </Badge>
          </div>
          <CardContent className="space-y-3 p-4">
            <h3 className="truncate text-sm font-medium">{video.title}</h3>
            <div className="flex items-center gap-4 text-xs text-muted-foreground">
              <span className="flex items-center gap-1.5">
                <Heart className="size-3.5" />
                {formatMetric(video.like_count || 0)}
              </span>
              <span className="flex items-center gap-1.5">
                <MessageCircle className="size-3.5" />
                {formatMetric(video.comment_count || 0)}
              </span>
            </div>
          </CardContent>
          <Button
            variant="ghost"
            className="absolute inset-0 z-10 h-full w-full rounded-xl hover:bg-transparent"
            aria-label={`播放 ${video.title}`}
            onClick={() => onSelect(video)}
          />
        </Card>
      ))}
    </div>
  );
}
export function RelationList({
  tab,
  items,
  state,
  error,
  hasMore,
  following,
  busyID,
  currentUserID,
  onRetry,
  onLoadMore,
  onToggleFollow,
}: RelationListProps) {
  if (state === "loading")
    return (
      <div className="space-y-4" aria-label="正在加载关系">
        {[1, 2, 3].map((id) => (
          <Skeleton className="h-16 w-full" key={id} />
        ))}
      </div>
    );
  if (state === "error" && !items.length)
    return (
      <PageMessage
        icon="sync_problem"
        title={error || "关系列表加载失败"}
        action="重试"
        onAction={onRetry}
      />
    );
  return (
    <div className="space-y-4">
      {!items.length && (
        <PageMessage
          icon="person"
          title={tab === "following" ? "暂无关注" : "暂无粉丝"}
        />
      )}
      <div className="divide-y">
        {items.map((item) => (
          <article
            className="flex items-center gap-3 py-4 first:pt-0"
            key={item.user_id}
          >
            <UserAvatar
              src={item.avatar_url}
              name={item.nickname}
              className="size-11"
            />
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">
                {item.nickname || `用户_${item.user_id}`}
              </p>
              <p className="mt-1 truncate text-xs text-muted-foreground">
                {item.bio || "这个用户还没有填写简介。"}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                {formatRelativeTime(item.followed_at)}
              </p>
            </div>
            <Button
              size="sm"
              variant={following[item.user_id] ? "outline" : "default"}
              onClick={() => onToggleFollow(item.user_id)}
              disabled={
                busyID === item.user_id || item.user_id === currentUserID
              }
            >
              {item.user_id === currentUserID
                ? "本人"
                : busyID === item.user_id
                  ? "处理中"
                  : following[item.user_id]
                    ? "已关注"
                    : "关注"}
            </Button>
          </article>
        ))}
      </div>
      {state === "error" && items.length > 0 && (
        <p className="text-sm text-destructive" role="alert">
          {error || "加载失败"}
        </p>
      )}
      {hasMore && (
        <Button
          variant="outline"
          className="w-full"
          onClick={onLoadMore}
          disabled={state === "loadingMore"}
        >
          {state === "loadingMore" ? "加载中" : "加载更多"}
        </Button>
      )}
    </div>
  );
}
export function RelationModal({
  onClose,
  onTabChange,
  ...props
}: RelationModalProps) {
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        className="max-h-[85dvh] overflow-y-auto p-6 sm:max-w-lg"
        showCloseButton={false}
      >
        <DialogHeader className="flex-row items-start justify-between">
          <div className="space-y-1.5">
            <DialogTitle>
              {props.tab === "following" ? "关注的人" : "粉丝"}
            </DialogTitle>
            <DialogDescription>查看关注和粉丝，管理关注关系。</DialogDescription>
          </div>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onClose}
            aria-label="关闭关系弹窗"
          >
            <X />
          </Button>
        </DialogHeader>
        <Tabs
          value={props.tab}
          onValueChange={(value) => {
            if (value === "following" || value === "followers")
              onTabChange(value);
          }}
        >
          <TabsList aria-label="关系类型" className="w-full">
            <TabsTrigger value="following" className="flex-1">
              关注
            </TabsTrigger>
            <TabsTrigger value="followers" className="flex-1">
              粉丝
            </TabsTrigger>
          </TabsList>
        </Tabs>
        <RelationList {...props} />
      </DialogContent>
    </Dialog>
  );
}
export function WorkViewer({
  video,
  onClose,
}: {
  video: Video;
  onClose: Action;
}) {
  const media = video.media_url || video.cover_url || DEFAULT_COVER_URL;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        className="overflow-hidden p-0 sm:max-w-4xl"
        showCloseButton={false}
      >
        <DialogHeader className="flex-row items-center justify-between px-5 pt-5">
          <div className="space-y-1.5">
            <DialogTitle>{video.title || "作品"}</DialogTitle>
            <DialogDescription>
              {formatMetric(video.like_count || 0)} 点赞 ·{" "}
              {formatMetric(video.comment_count || 0)} 评论
            </DialogDescription>
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={onClose}
            aria-label="关闭"
          >
            <X />
          </Button>
        </DialogHeader>
        <div className="flex max-h-[75dvh] items-center justify-center bg-black">
          {isVideoSource(media) ? (
            <video
              className="max-h-[75dvh] w-full"
              src={media}
              poster={video.cover_url || DEFAULT_COVER_URL}
              controls
              autoPlay
              playsInline
            />
          ) : (
            <img className="max-h-[75dvh] object-contain" src={media} alt="" />
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
