# UI component sources

`src/components/ui/` contains shadcn Base UI components from the `base-nova` registry. The upstream MIT license is in [LICENSE](LICENSE).

- Registry: https://ui.shadcn.com/r/styles/base-nova/
- Sidebar API: https://ui.shadcn.com/docs/components/base/sidebar

Application-specific composition lives outside `components/ui/`. Tailwind theme and video layout rules are in `src/styles.css`.

本地视觉调整：按钮、标签页和侧栏仅过渡颜色；状态徽标采用小圆角；弹窗和抽屉使用半透明遮罩，不使用背景模糊。组件交互继续由 Base UI 管理。
