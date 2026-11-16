import { CheckCheck, RefreshCw } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { UserAvatar } from "@/components/UserAvatar";
import { Icon } from "@/components/Icon";
import { Button } from "@/components/ui/button";
import { useEffect } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query";
import { errorMessage, errorStatus } from "@/api/client";
import { fetchMessages, markMessagesRead } from "@/api/resources";
import { PageMessage } from "@/components/PageMessage";
import { formatRelativeTime } from "@/lib/format";
import {
  appendMessages,
  messageActor,
  messageBody,
  messageIcon,
} from "@/lib/messages";
import { queryKeys } from "@/lib/queryClient";
import type { PageProps, Message, Page, LoadState } from "@/types";

export function MessagesPage({
  session,
  onNavigate,
  onUnreadChange,
}: PageProps & { onUnreadChange: () => void | Promise<void> }) {
  const client = useQueryClient();
  const query = useInfiniteQuery({
    queryKey: queryKeys.messages,
    enabled: !!session.token,
    initialPageParam: "",
    queryFn: ({ pageParam, signal }) =>
      fetchMessages(session.token, pageParam, signal),
    getNextPageParam: (last, _pages, _cursor, cursors) =>
      last.has_more && last.next_cursor && !cursors.includes(last.next_cursor)
        ? last.next_cursor
        : undefined,
  });
  const mutation = useMutation({
    mutationFn: (ids: number[]) => markMessagesRead(session.token, ids),
    onSuccess: async (_data, ids) => {
      client.setQueryData<InfiniteData<Page<Message>, string>>(
        queryKeys.messages,
        (data) =>
          data && {
            ...data,
            pages: data.pages.map((page) => ({
              ...page,
              items: page.items.map((item) =>
                ids.length === 0 || ids.includes(item.id)
                  ? {
                      ...item,
                      is_read: true,
                      read_at: item.read_at || new Date().toISOString(),
                    }
                  : item,
              ),
            })),
          },
      );
      await onUnreadChange();
    },
  });
  const error = errorMessage(mutation.error || query.error);
  useEffect(() => {
    if (errorStatus(mutation.error || query.error) === 401) {
      session.clearAuth();
      onNavigate("/auth");
    }
  }, [mutation.error, query.error, session.clearAuth, onNavigate]);
  const items = (query.data?.pages || []).reduce<Message[]>(
    (all, page) => appendMessages(all, page.items),
    [],
  );
  const state: LoadState = query.isPending
    ? "loading"
    : query.isFetchingNextPage
      ? "loadingMore"
      : query.isError
        ? "error"
        : "ready";
  const hasMore = query.hasNextPage;
  const markingAll = mutation.isPending && mutation.variables.length === 0;
  const busyID = mutation.isPending ? mutation.variables[0] || 0 : 0;
  const markRead = (ids: number[]) => {
    if (!mutation.isPending) mutation.mutate(ids);
  };

  const unreadCount = items.filter((item) => !item.is_read).length;
  const loadingInitial = state === "loading" && items.length === 0;
  const loadingMore = state === "loadingMore";

  return (
    <div className="page-container max-w-4xl!">
      <div className="page-heading">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">消息中心</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            关注每一次回应，与喜欢你的人保持联系。
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => query.refetch()}
            disabled={loadingInitial || loadingMore}
          >
            <RefreshCw />
            刷新
          </Button>
          <Button
            size="sm"
            onClick={() => markRead([])}
            disabled={markingAll || unreadCount === 0}
          >
            <CheckCheck />
            {markingAll ? "处理中" : "全部已读"}
          </Button>
        </div>
      </div>
      <Card className="gap-0 py-0 shadow-none">
        <div className="flex items-center justify-between border-b px-5 py-4">
          <h2 className="text-sm font-medium">全部消息</h2>
          <Badge variant="secondary">
            {unreadCount > 0 ? `${unreadCount} 未读` : "已读完"}
          </Badge>
        </div>
        <CardContent className="p-0">
          {loadingInitial && (
            <div className="p-5">
              <PageMessage icon="hourglass_top" title="正在加载消息" />
            </div>
          )}
          {state === "error" && !items.length && (
            <div className="p-5">
              <PageMessage
                icon="sync_problem"
                title={error || "消息加载失败"}
                action="重试"
                onAction={() => query.refetch()}
              />
            </div>
          )}
          {state === "ready" && !items.length && (
            <div className="p-5">
              <PageMessage
                icon="notifications"
                title="暂无消息，新的互动会出现在这里。"
              />
            </div>
          )}
          {error && items.length > 0 && (
            <p className="px-5 py-3 text-sm text-destructive" role="alert">
              {error}
            </p>
          )}
          <div className="divide-y">
            {items.map((message) => {
              const actor = messageActor(message);
              return (
                <Button
                  key={message.id}
                  variant="ghost"
                  className={`h-auto w-full items-start justify-start gap-4 rounded-none p-5 text-left whitespace-normal ${message.is_read ? "" : "bg-muted/40"}`}
                  onClick={() => !message.is_read && markRead([message.id])}
                  disabled={busyID === message.id}
                >
                  <span className="flex size-10 shrink-0 items-center justify-center rounded-xl border bg-background text-muted-foreground">
                    <Icon name={messageIcon(message.type)} />
                  </span>
                  <span className="grid min-w-0 flex-1 gap-2">
                    <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
                      <span className="font-medium">{message.title}</span>
                      <time className="text-xs font-normal text-muted-foreground">
                        {formatRelativeTime(message.created_at)}
                      </time>
                    </span>
                    {actor && (
                      <span className="flex items-center gap-2 text-xs text-muted-foreground">
                        <UserAvatar
                          src={actor.avatar_url}
                          name={actor.nickname}
                          className="size-5"
                        />
                        {actor.nickname}
                      </span>
                    )}
                    <span className="text-sm font-normal leading-relaxed text-muted-foreground wrap-anywhere">
                      {messageBody(message)}
                    </span>
                  </span>
                  {!message.is_read && (
                    <span
                      className="mt-1.5 size-2 shrink-0 rounded-full bg-primary"
                      aria-label={busyID === message.id ? "处理中" : "未读"}
                    />
                  )}
                </Button>
              );
            })}
          </div>
        </CardContent>
      </Card>
      {hasMore && (
        <Button
          variant="outline"
          className="w-full"
          onClick={() => query.fetchNextPage({ cancelRefetch: false })}
          disabled={loadingMore}
        >
          {loadingMore ? "加载中" : "加载更多"}
        </Button>
      )}
    </div>
  );
}
export default MessagesPage;
