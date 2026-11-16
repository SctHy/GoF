import { Pencil, Plus, X } from "lucide-react";
import { UserAvatar } from "@/components/UserAvatar";
import { Card, CardContent } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { errorMessage, errorStatus } from "../api/client.ts";
import type {
  Video,
  VideoList,
  RelationUser,
  RelationTab,
  LoadState,
  ActionMap,
  Page,
  FollowResponse,
  User,
} from "../types.ts";
import type { FormEvent } from "react";
import type { PageProps } from "../types.ts";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query";
import { queryKeys } from "@/lib/queryClient";
import { useEffect, useState } from "react";
import { apiData, uploadFile } from "../api/client.ts";
import { loadFollowingMap, relationListPath } from "../api/resources.ts";
import {
  RelationModal,
  VideoGrid,
  WorkViewer,
} from "../components/ProfileContent.tsx";
import { emptyProfile } from "../lib/constants.ts";
import { formatMetric } from "../lib/format.ts";
import { updateSessionRelationCount } from "../lib/profiles.ts";

export function ProfilePage({ session, onNavigate }: PageProps) {
  const client = useQueryClient();
  const baseUser = session.user || emptyProfile;
  const [form, setForm] = useState({
    nickname: baseUser.nickname || "",
    avatar_url: baseUser.avatar_url || "",
    bio: baseUser.bio || "",
  });
  const [avatarFile, setAvatarFile] = useState<File | null>(null);
  const [avatarPreview, setAvatarPreview] = useState("");
  const [editing, setEditing] = useState(false);
  const [selectedWork, setSelectedWork] = useState<Video | null>(null);
  const [status, setStatus] = useState("");
  const [relationTab, setRelationTab] = useState<RelationTab>("following");
  const [relationModalOpen, setRelationModalOpen] = useState(false);
  const [relationError, setRelationError] = useState("");
  const [relationBusyID, setRelationBusyID] = useState(0);
  const followingCount =
    baseUser.following_count ?? 0;
  const followerCount = baseUser.follower_count ?? 0;

  useEffect(() => {
    setForm({
      nickname: baseUser.nickname || "",
      avatar_url: baseUser.avatar_url || "",
      bio: baseUser.bio || "",
    });
    setAvatarFile(null);
    setAvatarPreview("");
  }, [baseUser.avatar_url, baseUser.bio, baseUser.nickname]);

  useEffect(() => {
    if (!avatarFile) {
      setAvatarPreview("");
      return;
    }
    const objectURL = URL.createObjectURL(avatarFile);
    setAvatarPreview(objectURL);
    return () => URL.revokeObjectURL(objectURL);
  }, [avatarFile]);

  const works = useQuery({
    queryKey: queryKeys.ownVideos,
    enabled: !!session.token,
    queryFn: ({ signal }) =>
      apiData<VideoList>("/api/users/me/videos?limit=12", {
        token: session.token,
        signal,
      }),
  });
  const followingQuery = useQuery({
    queryKey: queryKeys.following,
    enabled: !!session.token,
    queryFn: ({ signal }) => loadFollowingMap(session.token, signal),
  });
  const relations = useInfiniteQuery({
    queryKey: queryKeys.relations(relationTab),
    enabled: !!session.token && relationModalOpen,
    initialPageParam: "",
    queryFn: ({ pageParam, signal }) =>
      apiData<Page<RelationUser>>(relationListPath(relationTab, pageParam), {
        token: session.token,
        signal,
      }),
    getNextPageParam: (last, _pages, _cursor, cursors) =>
      last.has_more && last.next_cursor && !cursors.includes(last.next_cursor)
        ? last.next_cursor
        : undefined,
  });
  const videos = works.data?.items || [];
  const videosState = works.isPending
    ? "loading"
    : works.isError
      ? errorMessage(works.error) || "作品加载失败"
      : "ready";
  const relationItems =
    relations.data?.pages.flatMap((page) => page.items) || [];
  const relationFollowing = followingQuery.data || {};
  const relationHasMore = relations.hasNextPage;
  const relationState: LoadState = relations.isPending
    ? "loading"
    : relations.isFetchingNextPage
      ? "loadingMore"
      : relations.isError || relationError
        ? "error"
        : "ready";
  useEffect(() => {
    if (
      [works.error, followingQuery.error, relations.error].some(
        (error) => errorStatus(error) === 401,
      )
    ) {
      session.clearAuth();
      onNavigate("/auth");
    }
  }, [
    works.error,
    followingQuery.error,
    relations.error,
    session.clearAuth,
    onNavigate,
  ]);

  function openRelationModal(tab: RelationTab) {
    setRelationError("");
    setRelationTab(tab);
    setRelationModalOpen(true);
  }

  async function toggleRelationFollow(targetUserID: number) {
    if (!session.token) {
      onNavigate("/auth");
      return;
    }
    if (!targetUserID || targetUserID === baseUser.id) return;

    const currentFollowing = Boolean(relationFollowing[targetUserID]);
    setRelationBusyID(targetUserID);
    setRelationError("");
    try {
      const data = await apiData<FollowResponse>(
        `/api/users/me/following/${targetUserID}`,
        {
          method: currentFollowing ? "DELETE" : "PUT",
          token: session.token,
          headers: {
            "Idempotency-Key": `web-profile-follow-${targetUserID}-${Date.now()}`,
          },
        },
      );
      client.setQueryData<ActionMap>(queryKeys.following, (state) => ({
        ...state,
        [targetUserID]: data.following,
      }));
      if (relationTab === "following" && !data.following) {
        client.setQueryData<InfiniteData<Page<RelationUser>, string>>(
          queryKeys.relations("following"),
          (state) =>
            state && {
              ...state,
              pages: state.pages.map((page) => ({
                ...page,
                items: page.items.filter(
                  (item) => item.user_id !== targetUserID,
                ),
              })),
            },
        );
      }
      updateSessionRelationCount(session, data.following_count);
    } catch (error) {
      if (errorStatus(error) === 401) {
        session.clearAuth();
        onNavigate("/auth");
        return;
      }
      setRelationError(errorMessage(error) || "关注操作失败");
    } finally {
      setRelationBusyID(0);
    }
  }

  async function handleSave(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setStatus("保存中");
    try {
      let avatarURL = form.avatar_url;
      if (avatarFile && session.token) {
        const uploaded = await uploadFile(avatarFile, "avatar", session.token);
        avatarURL = uploaded.url;
      }
      let profile = { ...baseUser, ...form };
      if (session.token) {
        profile = await apiData<User>("/api/users/me", {
          method: "PATCH",
          token: session.token,
          body: {
            nickname: form.nickname,
            avatar_url: avatarURL,
            bio: form.bio,
          },
        });
      }
      session.setAuth(session.token, profile);
      client.setQueryData(queryKeys.publicProfile(profile.id), profile);
      setAvatarFile(null);
      setStatus("已保存");
      setEditing(false);
    } catch (error) {
      setStatus(errorMessage(error) || "保存失败");
    }
  }

  return (
    <div className="page-container">
      <div className="page-heading">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">个人主页</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            你的作品，以及一路相遇的人。
          </p>
        </div>
        <Button onClick={() => onNavigate("/upload")}>
          <Plus />
          发布视频
        </Button>
      </div>
      <Card className="overflow-hidden py-0 shadow-none">
        <div className="h-20 border-b bg-muted/60 sm:h-28" />
        <CardContent className="relative px-5 pb-6 sm:px-7">
          <div className="-mt-10 flex items-end justify-between gap-4">
            <UserAvatar
              src={avatarPreview || form.avatar_url}
              name={form.nickname || baseUser.account}
              className="size-20 text-2xl ring-4 ring-background sm:size-24"
            />
            <Button
              variant="outline"
              onClick={() => setEditing(true)}
              aria-label="编辑资料"
            >
              <Pencil />
              编辑资料
            </Button>
          </div>
          <div className="mt-5">
            <h2 className="text-xl font-semibold">
              {form.nickname || baseUser.account}
            </h2>
            <p className="mt-1 text-xs text-muted-foreground">
              @{baseUser.account}
            </p>
            <p className="mt-3 max-w-2xl text-sm leading-relaxed text-muted-foreground">
              {form.bio || "还没有填写简介。"}
            </p>
          </div>
          <div className="mt-5 flex gap-6" aria-label="资料统计">
            <Button
              variant="ghost"
              className="h-auto gap-2 p-0 hover:bg-transparent"
              onClick={() => openRelationModal("following")}
            >
              <strong>{formatMetric(followingCount)}</strong>
              <span className="font-normal text-muted-foreground">关注</span>
            </Button>
            <Button
              variant="ghost"
              className="h-auto gap-2 p-0 hover:bg-transparent"
              onClick={() => openRelationModal("followers")}
            >
              <strong>{formatMetric(followerCount)}</strong>
              <span className="font-normal text-muted-foreground">粉丝</span>
            </Button>
            <span className="flex gap-2 text-sm">
              <strong>{formatMetric(videos.length)}</strong>
              <span className="text-muted-foreground">作品</span>
            </span>
          </div>
        </CardContent>
      </Card>
      <section className="space-y-5">
        <div className="flex items-center gap-2 border-b pb-4">
          <h2 className="text-base font-semibold">我的作品</h2>
          <span className="text-sm text-muted-foreground">{videos.length}</span>
        </div>
        <VideoGrid
          videos={videos}
          state={videosState}
          onSelect={setSelectedWork}
        />
      </section>
      {relationModalOpen && (
        <RelationModal
          tab={relationTab}
          items={relationItems}
          state={relationState}
          error={relationError || errorMessage(relations.error)}
          hasMore={relationHasMore}
          following={relationFollowing}
          busyID={relationBusyID}
          currentUserID={baseUser.id}
          onTabChange={openRelationModal}
          onClose={() => setRelationModalOpen(false)}
          onRetry={() => {
            setRelationError("");
            void relations.refetch();
          }}
          onLoadMore={() => {
            if (!relations.isFetching)
              void relations.fetchNextPage({ cancelRefetch: false });
          }}
          onToggleFollow={toggleRelationFollow}
        />
      )}
      {selectedWork && (
        <WorkViewer
          video={selectedWork}
          onClose={() => setSelectedWork(null)}
        />
      )}
      <Dialog open={editing} onOpenChange={setEditing}>
        <DialogContent
          className="max-h-[90dvh] overflow-y-auto p-6 sm:max-w-lg"
          showCloseButton={false}
        >
          <div className="flex items-start justify-between">
            <div className="space-y-1.5">
              <DialogTitle>资料编辑</DialogTitle>
              <DialogDescription>让大家更了解你。</DialogDescription>
            </div>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => setEditing(false)}
              aria-label="关闭"
            >
              <X />
            </Button>
          </div>
          <form className="space-y-5" onSubmit={handleSave}>
            <div className="form-field">
              <Label htmlFor="profile-name">昵称</Label>
              <Input
                id="profile-name"
                value={form.nickname}
                onChange={(event) =>
                  setForm({ ...form, nickname: event.target.value })
                }
              />
            </div>
            <div className="form-field">
              <Label htmlFor="profile-avatar">头像</Label>
              <div className="flex items-center gap-4 rounded-lg border p-3">
                <UserAvatar
                  src={avatarPreview || form.avatar_url}
                  name={form.nickname}
                  className="size-12"
                />
                <Input
                  id="profile-avatar"
                  className="min-w-0"
                  type="file"
                  accept="image/*"
                  onChange={(event) =>
                    setAvatarFile(event.target.files?.[0] || null)
                  }
                />
              </div>
            </div>
            <div className="form-field">
              <Label htmlFor="profile-bio">简介</Label>
              <Textarea
                id="profile-bio"
                value={form.bio}
                onChange={(event) =>
                  setForm({ ...form, bio: event.target.value })
                }
                rows={4}
              />
            </div>
            {status && (
              <p role="status" className="text-sm text-destructive">
                {status}
              </p>
            )}
            <div className="flex justify-end gap-2 border-t pt-4">
              <Button variant="outline" onClick={() => setEditing(false)}>
                取消
              </Button>
              <Button type="submit">保存</Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}

export default ProfilePage;
