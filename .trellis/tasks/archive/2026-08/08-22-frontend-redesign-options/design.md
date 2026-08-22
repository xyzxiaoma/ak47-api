# A＋B 前端重设计技术方案

## 目标与边界

采用“断层控制室”作为认证后工作台语言，采用“编辑部账本”作为公开首页与 Docs/定价的叙事语言。首轮实现覆盖全局视觉 Token、公开首页、公共 Header/Footer、认证壳层和 Dashboard 关键区域；业务页面沿用现有路由和数据契约，通过共享 Token 与页面容器逐步迁移。

不修改 API、路由 URL、权限判断、账务逻辑、数据库字段、国际化语言范围和 New API/QuantumNous 法律归属。

## 视觉 Token

- 背景：煤黑、暖纸白、石墨灰，不使用纯平单色；统一叠加 CSS 噪点与局部径向材质。
- 强调色：锈橙、酸橄榄、砖红、琥珀；不使用紫色、靛蓝或蓝紫渐变。
- 字体：正文保留 Public Sans；公开内容标题允许 Lora；数据和代码使用等宽字族。
- 形状：减少默认大圆角，按页面类型使用锐角、短圆角和标签化边界。
- 图标：引入 Iconify 的本地可审计集合；关键图标使用语义标签，装饰图标隐藏。
- 动效：只使用 spring 或非线性 cubic-bezier，支持 prefers-reduced-motion。

## 组件与页面边界

1. `web/src/styles/theme.css` / `theme-presets.css`：建立 A/B Token 和噪点背景工具类。
2. `web/src/components/layout/components/public-header.tsx`、`public-layout.tsx`：重做公开站点的编辑式导航。
3. `web/src/features/home/components/`：改为“请求路径叙事 + 证据侧栏 + 不对称章节”。保留自定义 HTML/URL 首页能力。
4. `web/src/components/layout/components/app-header.tsx`、`app-sidebar.tsx`、`nav-group.tsx`：保留权限、动态导航和设置 drill-in，仅改变信息层级与外观。
5. `web/src/features/dashboard/`：优先将首页统计重排为请求流、健康、费用和待处理事项，不改变查询 API。
6. Models、Channels、Usage Logs、Wallet、Profile、Users、System Settings：通过共享 `ControlRoomPage`、`LedgerTable`、`SectionRail` 等模式分批迁移。

## 数据流与兼容性

- 页面继续使用现有 React Query hooks 和 API；视觉组件只消费已存在的数据类型。
- 空数据必须显示诚实的空状态，不补造模型、费用、可用率或交易数据。
- 所有新增文案走 i18next；同步登记 `static-keys.ts` 与七个 locale。
- 自定义首页 iframe/HTML/Markdown 逻辑保持原行为。

## 资产策略

- Iconify：优先打包本地图标，避免运行时 CDN/API 依赖。
- Picsum：仅用于开发和空状态占位，不进入核心业务语义。
- Pexels：真实图片必须记录来源、替代文本和许可信息；建议下载后纳入可审计资产目录。
- unDraw：用于协议、路由、账本解释插画，并统一自定义为项目色。

## 风险与回滚

- 先替换 Token 和壳层，任何页面可以通过旧组件路径回退。
- 每一批迁移都保持 routeTree、queryKey、权限和数据字段不变。
- 对比度、键盘导航、移动端抽屉和 reduced-motion 是发布阻断项。
- 生产环境不依赖外部图片 CDN 才能渲染关键内容。
