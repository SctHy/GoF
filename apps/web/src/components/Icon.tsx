import { ArrowLeft, Bell, Bookmark, CheckCheck, ChevronDown, Clapperboard, Film, Flame, Heart, House, Image, LoaderCircle, LockKeyhole, LogIn, LogOut, Megaphone, MessageCircle, PanelsTopLeft, Play, RefreshCw, Save, Search, Send, Settings, Share2, SlidersHorizontal, Sparkles, Upload, UserPlus, UserRound, X, CircleHelp, type LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";
const icons: Record<string, LucideIcon> = {
  home: House,
  auto_awesome: Sparkles,
  subscriptions: PanelsTopLeft,
  local_fire_department: Flame,
  notifications: Bell,
  search: Search,
  upload: Upload,
  person: UserRound,
  logout: LogOut,
  hourglass_top: LoaderCircle,
  sync_problem: RefreshCw,
  chat_bubble: MessageCircle,
  tune: SlidersHorizontal,
  close: X,
  send: Send,
  favorite: Heart,
  bookmark: Bookmark,
  share: Share2,
  play_arrow: Play,
  lock: LockKeyhole,
  video_library: Clapperboard,
  movie: Film,
  image: Image,
  login: LogIn,
  publish: Upload,
  refresh: RefreshCw,
  done_all: CheckCheck,
  expand_more: ChevronDown,
  manage_accounts: Settings,
  save: Save,
  person_add: UserPlus,
  campaign: Megaphone,
  arrow_back: ArrowLeft,
};
export function Icon({ name, className }: { name: string; className?: string }) {
  const Component = icons[name] || CircleHelp;
  return <Component aria-hidden="true" className={cn("size-5 shrink-0", className)} size={22} />;
}
