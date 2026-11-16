import type { FeedScene, PlaybackConfig, User } from "../types.ts";
export const TOKEN_KEY = "gcfeed.accessToken";
export const USER_KEY = "gcfeed.user";
export const FEED_TRANSITION_MS = 320;
export const DEFAULT_PLAYBACK_CONFIG: PlaybackConfig = {
  platform: "Web",
  network_type: "DEFAULT",
  preload_count: 3,
  buffer_ms: 1200
};
export const FEED_SCENES = [
  { key: "timeline", label: "最新视频", route: "/timeline", icon: "home" },
  { key: "recommend", label: "推荐流", route: "/recommend", icon: "auto_awesome" },
  { key: "following", label: "关注流", route: "/following", icon: "subscriptions" },
  { key: "hot", label: "热门榜单", route: "/hotfeed", icon: "local_fire_department" }
] as const satisfies readonly { key: FeedScene; label: string; route: string; icon: string }[];

export const DEFAULT_COVER_URL = '/placeholder-cover.svg';

export const emptyProfile: User = {
  id: 0,
  account: "",
  nickname: "",
  avatar_url: "",
  bio: "",
  role: "",
  status: 0
};
