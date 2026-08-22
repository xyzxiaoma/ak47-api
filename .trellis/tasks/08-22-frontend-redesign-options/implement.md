# A＋B 前端重设计执行计划

## 第一批：视觉基础与公开站点

1. 盘点现有主题变量、字体、动画和图标引用，建立 A/B Token 命名与迁移映射。
2. 增加噪点/纸张材质背景、非默认色板、短圆角和状态色；移除首页蓝紫渐变。
3. 重做公共 Header：编辑式目录、状态标记、登录动作和移动端抽屉，保留所有链接与认证行为。
4. 重做首页 Hero、Stats、Features、HowItWorks、CTA：使用不对称布局、请求路径和证据侧栏。
5. 重做公共 Footer 的栏目层次，同时保留 New API 归属和原项目链接。

## 第二批：认证壳层与工作台

6. 重做 AppHeader、AppSidebar、NavGroup 和 SectionPageLayout 的视觉语言；保留动态导航、权限过滤、折叠和设置 drill-in。
7. 重排 Dashboard 统计、图表和空状态，使其呈现请求流、渠道健康、费用账本和待处理事项。
8. 为 Models、Channels、Usage Logs、Wallet、Profile、Users、System Settings 提供共享页面容器和表格/详情模式，按页面逐步替换旧样式。

## 第三批：资产、i18n 与质量

9. 将新增图标替换为 Iconify 本地映射；图片按 Picsum/Pexels/unDraw 规则登记来源和 fallback。
10. 补齐七语言新增文案，检查长文本、RTL 预留、字体回退和法律文案。
11. 为壳层、首页布局、移动端抽屉、空状态、键盘操作和 reduced-motion 增加行为回归测试。
12. 执行 `bun run format:check`、`bun run lint`、`bun run typecheck`、`bun run build`，再做浏览器桌面/移动视觉验收。

## 回滚点

- Token 层可单独回滚，不影响业务逻辑。
- 首页组件按 section 回滚；保留旧组件文件直到视觉验收完成。
- 认证壳层只替换 class 和布局组合，任何路由/权限回归立即停止后续迁移。
