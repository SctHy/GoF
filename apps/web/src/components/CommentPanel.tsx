import { Send, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import { UserAvatar } from "@/components/UserAvatar";
import { PageMessage } from "@/components/PageMessage";
import { formatMetric, formatRelativeTime } from "@/lib/format";
import { profileFromComment } from "@/lib/profiles";
import type { User, Comment, LoadState, PublicProfile, Action } from "@/types";

type CommentPanelProps = {
  open: boolean;
  value: string;
  onChange: (value: string) => void;
  onSubmit: Action;
  onClose: Action;
  user: User;
  count: number;
  comments: Comment[];
  state: LoadState;
  error: string;
  onRetry: Action;
  authenticated: boolean;
  onOpenUser: (profile: PublicProfile) => void;
};
export function CommentPanel({
  open,
  value,
  onChange,
  onSubmit,
  onClose,
  user,
  count,
  comments,
  state,
  error,
  onRetry,
  authenticated,
  onOpenUser,
}: CommentPanelProps) {
  return (
    <Sheet
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <SheetContent
        showCloseButton={false}
        className="gap-0 data-[side=right]:w-full sm:max-w-md!"
      >
        <SheetHeader className="flex-row items-center justify-between border-b p-5">
          <div className="space-y-1">
            <SheetTitle className="flex items-center gap-2">
              评论 <Badge variant="secondary">{formatMetric(count)}</Badge>
            </SheetTitle>
            <SheetDescription>聊聊你看到的精彩瞬间。</SheetDescription>
          </div>
          <Button
            variant="ghost"
            size="icon"
            aria-label="关闭评论"
            onClick={onClose}
          >
            <X />
          </Button>
        </SheetHeader>
        <div className="min-h-0 flex-1 space-y-6 overflow-y-auto p-5">
          {state === "loading" && (
            <PageMessage icon="hourglass_top" title="正在加载评论" />
          )}
          {state === "error" && (
            <PageMessage
              icon="sync_problem"
              title={error || "评论加载失败"}
              action="重试"
              onAction={onRetry}
            />
          )}
          {state === "ready" && comments.length === 0 && (
            <PageMessage
              icon="chat_bubble"
              title="暂无评论，来说说你的看法吧。"
            />
          )}
          {comments.map((comment) => (
            <article className="flex gap-3" key={comment.id}>
              <Button
                variant="ghost"
                size="icon"
                aria-label={`查看 ${comment.user_nickname || "用户"} 的主页`}
                onClick={() => {
                  onClose();
                  onOpenUser(profileFromComment(comment));
                }}
              >
                <UserAvatar
                  src={comment.user_avatar_url}
                  name={comment.user_nickname}
                />
              </Button>
              <div className="min-w-0 flex-1">
                <div className="flex items-center justify-between gap-2">
                  <Button
                    variant="link"
                    className="h-auto p-0 text-foreground"
                    onClick={() => {
                      onClose();
                      onOpenUser(profileFromComment(comment));
                    }}
                  >
                    {comment.user_nickname || `用户_${comment.user_id}`}
                  </Button>
                  <time className="shrink-0 text-xs text-muted-foreground">
                    {formatRelativeTime(comment.created_at)}
                  </time>
                </div>
                <p className="mt-1.5 text-sm leading-relaxed wrap-anywhere">
                  {comment.content}
                </p>
              </div>
            </article>
          ))}
        </div>
        <form
          className="flex items-center gap-3 border-t bg-muted/25 p-4"
          onSubmit={(event) => {
            event.preventDefault();
            onSubmit();
          }}
        >
          <UserAvatar src={user.avatar_url} name={user.nickname} />
          <Input
            aria-label="评论内容"
            value={value}
            onChange={(event) => onChange(event.target.value)}
            placeholder={authenticated ? "添加评论..." : "登录后评论"}
            disabled={!authenticated}
          />
          <Button
            size="icon"
            type="submit"
            aria-label="发送评论"
            disabled={!authenticated || !value.trim()}
          >
            <Send />
          </Button>
        </form>
      </SheetContent>
    </Sheet>
  );
}
