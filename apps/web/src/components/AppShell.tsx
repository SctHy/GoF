import type { CSSProperties, ReactNode } from "react";
import { Bell, LogIn, LogOut, Plus, UserRound, Video } from "lucide-react";
import { Brand } from "@/components/Brand";
import { Icon } from "@/components/Icon";
import { UserAvatar } from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarRail,
  SidebarTrigger,
  useSidebar,
} from "@/components/ui/sidebar";
import { FEED_SCENES } from "@/lib/constants";
import { formatBadgeCount } from "@/lib/format";
import type { User, Navigate, Action } from "@/types";

type AppShellProps = {
  children: ReactNode;
  user: User | null;
  route: string;
  authenticated: boolean;
  unreadCount: number;
  onNavigate: Navigate;
  onLogout: Action;
};

export function AppShell(props: AppShellProps) {
  return (
    <SidebarProvider style={{ "--sidebar-width": "15rem" } as CSSProperties}>
      <ShellContent {...props} />
    </SidebarProvider>
  );
}
function ShellContent({
  children,
  user,
  route,
  authenticated,
  unreadCount,
  onNavigate,
  onLogout,
}: AppShellProps) {
  const { setOpenMobile } = useSidebar();
  const navigate = (path: string) => {
    setOpenMobile(false);
    onNavigate(path);
  };
  const pageName =
    FEED_SCENES.find((scene) => scene.route === route)?.label ||
    { "/profile": "个人主页", "/messages": "消息中心", "/upload": "发布视频" }[
      route
    ] ||
    "用户主页";
  return (
    <>
      <Sidebar variant="inset" collapsible="icon">
        <SidebarHeader className="px-3 py-5">
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                className="h-10 gap-2.5 hover:bg-transparent"
                onClick={() => navigate("/timeline")}
                aria-label="GCFeed 首页"
              >
                <Brand />
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupLabel>发现</SidebarGroupLabel>
            <SidebarMenu aria-label="主导航">
              {FEED_SCENES.map((scene) => (
                <SidebarMenuItem key={scene.key}>
                  <SidebarMenuButton
                    size="lg"
                    isActive={route === scene.route}
                    tooltip={scene.label}
                    onClick={() => navigate(scene.route)}
                  >
                    <Icon name={scene.icon} />
                    <span>{scene.label}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroup>
          <SidebarGroup>
            <SidebarGroupLabel>我的空间</SidebarGroupLabel>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton
                  size="lg"
                  isActive={route === "/messages"}
                  tooltip="消息"
                  onClick={() =>
                    navigate(authenticated ? "/messages" : "/auth")
                  }
                >
                  <Bell />
                  <span>消息</span>
                  {authenticated && unreadCount > 0 && (
                    <Badge className="ml-auto" variant="secondary">
                      {formatBadgeCount(unreadCount)}
                    </Badge>
                  )}
                </SidebarMenuButton>
              </SidebarMenuItem>
              <SidebarMenuItem>
                <SidebarMenuButton
                  size="lg"
                  isActive={route === "/profile"}
                  tooltip="个人主页"
                  onClick={() => navigate(authenticated ? "/profile" : "/auth")}
                >
                  <UserRound />
                  <span>个人主页</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
              <SidebarMenuItem>
                <SidebarMenuButton
                  size="lg"
                  isActive={route === "/upload"}
                  tooltip="发布视频"
                  onClick={() => navigate(authenticated ? "/upload" : "/auth")}
                >
                  <Video />
                  <span>发布视频</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroup>
        </SidebarContent>
        <SidebarFooter className="gap-3 p-3">
          <p className="px-2 text-xs leading-relaxed text-muted-foreground group-data-[collapsible=icon]:hidden">
            每一次停留，都有新的发现。
          </p>
          <Separator />
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                size="lg"
                onClick={() => navigate(authenticated ? "/profile" : "/auth")}
              >
                <UserAvatar
                  src={user?.avatar_url}
                  name={user?.nickname}
                  className="size-8"
                />
                <span className="grid min-w-0 gap-0.5 text-left">
                  <span className="truncate font-medium">
                    {authenticated
                      ? user?.nickname || user?.account
                      : "欢迎来到 GCFeed"}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {authenticated ? "查看个人主页" : "登录，收藏你喜欢的内容"}
                  </span>
                </span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
        <SidebarRail />
      </Sidebar>
      <SidebarInset className="min-w-0 md:peer-data-[variant=inset]:shadow-none md:peer-data-[variant=inset]:ring-1 md:peer-data-[variant=inset]:ring-border">
        <header className="sticky top-0 z-30 flex h-16 shrink-0 items-center gap-3 border-b bg-background/95 px-4 backdrop-blur-sm lg:px-7">
          <SidebarTrigger aria-label="切换导航" />
          <Separator orientation="vertical" className="mx-1 h-4!" />
          <span className="hidden text-sm text-muted-foreground sm:inline">
            GCFeed
          </span>
          <span className="hidden text-border sm:inline">/</span>
          <span className="text-sm font-medium">{pageName}</span>
          <div className="ml-auto flex items-center gap-2">
            <Button
              size="sm"
              onClick={() => navigate(authenticated ? "/upload" : "/auth")}
            >
              <Plus />
              <span className="hidden sm:inline">发布</span>
              <span className="sr-only sm:hidden">发布</span>
            </Button>
            {authenticated ? (
              <>
                <Button
                  variant="ghost"
                  size="icon"
                  className="relative"
                  aria-label="通知"
                  onClick={() => navigate("/messages")}
                >
                  <Bell />
                  {unreadCount > 0 && (
                    <span className="absolute top-1.5 right-1.5 size-1.5 rounded-full bg-primary" />
                  )}
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="个人资料"
                  onClick={() => navigate("/profile")}
                >
                  <UserAvatar
                    src={user?.avatar_url}
                    name={user?.nickname}
                    className="size-7"
                  />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="退出登录"
                  onClick={onLogout}
                >
                  <LogOut />
                </Button>
              </>
            ) : (
              <Button
                variant="outline"
                size="sm"
                onClick={() => navigate("/auth")}
              >
                <LogIn />
                登录
              </Button>
            )}
          </div>
        </header>
        {children}
      </SidebarInset>
    </>
  );
}
