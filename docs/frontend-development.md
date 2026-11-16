# 前端开发

React + TypeScript + Vite；TanStack Router/Query；Tailwind v4 与 shadcn Base UI `base-nova`。组件来源及许可见 [说明](../apps/web/vendor/shadcn-ui/README.md)。

## 环境与检查

Node 26（最低 26.12）、npm 11；依赖精确版本写入 package.json 和 lockfile，使用 npm ci 安装。

```bash
cd apps/web
npm ci
npm run dev
npm run check
```

`typecheck` 检查源码、TS 测试和构建配置；`test` 运行 Vitest Node 逻辑测试，`test:ui` 运行 jsdom + Testing Library 交互测试；`build` 先检查类型再生产打包。根目录 `scripts/check.sh` 和 CI 也覆盖 UI 测试。

## 模块边界

- `src/router.tsx`：声明路由、登录页面保护、公开用户 ID 校验及 `/login`、`/me` 跳转。`App.tsx` 负责身份、QueryClient 和布局，页面通过 Outlet 渲染。
- `src/api/`：fetch、JSON/上传协议、HTTP 错误、AbortSignal 和接口类型；`apiRequest<T>` 允许 204/null，`apiData<T>` 要求响应非空。泛型不替代运行时字段校验。
- `src/lib/queryClient.ts`：缓存策略与查询键；登录、退出或切换身份时重建并清理客户端，不持久化私有缓存或把 token 放入查询键。异步旧会话结果不能修改新登录态。
- `src/hooks/useFeedQuery.ts`：Query 管理 Feed 快照与游标分页。播放器、拖拽、评论输入等临时状态保留在 React Hooks。
- `src/pages/`：页面查询、表单与业务动作；普通列表使用 useQuery/useInfiniteQuery，写操作完成后更新或失效对应缓存。
- `src/components/ui/`：shadcn Base UI 基础组件；侧栏使用 Sidebar/SidebarInset，评论与手机导航使用 Sheet，资料/作品/表单使用 Card、Avatar、Badge、Label 等；图标使用本地 Lucide SVG。组件来源和许可保留在 vendor/shadcn-ui。
- `src/styles.css`：shadcn zinc 浅色主题及独立深色视频区，启用完整 Tailwind（含 Preflight）。页面布局使用工具类，只为播放器尺寸、遮罩、拖动动画保留专用 CSS；不再维护旧版页面选择器。

## 必须保持的行为

1. Feed 禁止焦点、重连和重新挂载触发自动刷新。追加页沿用同一 request_id；显式刷新才开启新快照并重置观看位置。
2. 追加失败保留当前视频并停止自动追加；游标过期显示“分页已过期，请刷新”。点赞、收藏等缓存更新不能解除失败状态或偷偷重试旧游标。
3. 查询函数将 Query 提供的 signal 传给 fetch；卸载/身份切换取消读请求，迟到响应只属于旧缓存。写操作不自动重试。
4. 点赞、收藏、关注和评论更新对应缓存；上传后使个人作品缓存失效。用户资料占位数据仅在 Query 缓存中保存。
5. SidebarInset 提供唯一 main 地标，路由页面不重复嵌套 main。手机导航选择后关闭 Sheet。Base UI 弹窗管理键盘 Escape、焦点限制与恢复；登录/关系标签页支持键盘选择。表单提交按钮显式声明 submit。
6. 播放器手势、首帧/卡顿统计、曝光去重与预加载资源清理保留在业务层；输入框、弹窗和输入法组合输入不触发 Feed 快捷键。播放进度来自媒体时间，暂停和静音按钮操作实际 video 元素。
7. 头像缺失时使用 AvatarFallback 首字，封面缺失时使用 public/placeholder-cover.svg；不请求外部演示头像。没有后端能力的搜索、分享、筛选控件不展示。

## 验证边界

逻辑与组件测试覆盖 API、分页快照、取消、迟到响应、身份隔离、导航、表单和弹窗。浏览器验收使用本地模拟 API 与合成视频，验证界面和浏览器播放链路；不代表真实后端联调、移动设备帧率或生产性能。构建大小以当前 Vite 输出为准。
