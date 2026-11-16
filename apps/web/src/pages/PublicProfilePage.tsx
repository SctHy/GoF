import { UserAvatar } from "@/components/UserAvatar";
import { Card, CardContent } from "@/components/ui/card";
import { Icon } from "@/components/Icon";
import { Button } from "@/components/ui/button";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { apiData, errorMessage } from "@/api/client";
import { VideoGrid, WorkViewer } from "@/components/ProfileContent";
import { formatOptionalMetric } from "@/lib/format";
import { queryKeys } from "@/lib/queryClient";
import type { Video, VideoList, PublicProfile, Navigate } from "@/types";

export function PublicProfilePage({
  userID,
  onNavigate,
}: {
  userID: number;
  onNavigate: Navigate;
}) {
  const [selectedWork, setSelectedWork] = useState<Video | null>(null);
  const profile = useQuery({
    queryKey: queryKeys.publicProfile(userID),
    queryFn: ({ signal }) =>
      apiData<PublicProfile>(`/api/users/${userID}`, { signal }),
  });
  const works = useQuery({
    queryKey: queryKeys.publicVideos(userID),
    queryFn: ({ signal }) =>
      apiData<VideoList>(`/api/users/${userID}/videos?limit=24`, { signal }),
  });
  const videos = works.data?.items || [];
  const videosState = works.isPending
    ? "loading"
    : works.isError
      ? errorMessage(works.error) || "作品加载失败"
      : "ready";
  const profileState = profile.isPending
    ? "loading"
    : profile.isError
      ? errorMessage(profile.error) || "资料加载失败"
      : "ready";
  const displayProfile: PublicProfile = profile.data || {
    id: userID,
    nickname: `用户_${userID}`,
    avatar_url: "",
    bio: "",
  };

  return (
    <div className="page-container">
      <Button
        variant="ghost"
        className="-ml-3"
        onClick={() => onNavigate("/timeline")}
      >
        <Icon name="home" />
        返回发现
      </Button>
      <Card className="shadow-none">
        <CardContent className="flex flex-col items-start gap-5 pt-2 sm:flex-row sm:items-center">
          <UserAvatar
            src={displayProfile.avatar_url}
            name={displayProfile.nickname}
            className="size-20 text-2xl"
          />
          <div className="flex-1">
            <h1 className="text-2xl font-semibold tracking-tight">
              {displayProfile.nickname || `用户_${userID}`}
            </h1>
            <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
              {displayProfile.bio || "这个用户还没有填写简介。"}
            </p>
            <div className="mt-4 flex gap-5 text-sm" aria-label="资料统计">
              {[
                ["关注", displayProfile.following_count],
                ["粉丝", displayProfile.follower_count],
                ["作品", displayProfile.work_count],
              ].map(([label, count]) => (
                <span key={label} className="flex gap-2">
                  <strong>
                    {formatOptionalMetric(
                      typeof count === "number" ? count : undefined,
                    )}
                  </strong>
                  <span className="text-muted-foreground">{label}</span>
                </span>
              ))}
            </div>
            {profileState !== "loading" && profileState !== "ready" && (
              <p className="mt-3 text-sm text-destructive" role="alert">
                {profileState}
              </p>
            )}
          </div>
        </CardContent>
      </Card>
      <section className="space-y-5">
        <h2 className="border-b pb-4 text-base font-semibold">他的作品</h2>
        <VideoGrid
          videos={videos}
          state={videosState}
          onSelect={setSelectedWork}
        />
      </section>
      {selectedWork && (
        <WorkViewer
          video={selectedWork}
          onClose={() => setSelectedWork(null)}
        />
      )}
    </div>
  );
}
export default PublicProfilePage;
