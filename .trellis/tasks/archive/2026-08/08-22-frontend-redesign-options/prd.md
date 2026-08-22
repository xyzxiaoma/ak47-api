# 全项目前端视觉重设计方案

## Goal

在不改变现有 API、权限、路由和账务事实的前提下，为 New API 衍生项目建立一套有辨识度的前端视觉与信息架构方向，先输出多个可比较方案，再由用户选定主方向后进入设计与实现阶段。

## Confirmed Facts

- 前端位于 `web/`，技术栈为 React 19、TypeScript、TanStack Router、Tailwind CSS 4、Base UI、Hugeicons、React Query、i18next。
- 公开首页当前由 `Hero → Stats → Features → HowItWorks → CTA → Footer` 组成；Hero 使用蓝/紫渐变、网格背景、左右近等宽分栏和圆角胶囊导航。
- 登录后壳层已有 AppHeader、可折叠 AppSidebar、Dashboard、Models、Channels、Usage Logs、Wallet、Profile、Users、System Settings、Playground/Chat 等模块，并通过路由驱动二级设置视图。
- 当前支持 en、zh、zh-TW、fr、ru、ja、vi；需要保留 New API / QuantumNous 相关法律归属和原项目链接。
- 用户给出的规则要求避免紫色/靛蓝/蓝紫渐变、纯平背景、Tailwind 默认色板、Hero+三卡片、完美居中、等宽多栏、无意义文案、默认 shadcn/Material 组件、Emoji 图标和线性动画；图标使用 Iconify，图片按 Picsum/Pexels/unDraw 分类。

## Requirements

- 提供至少 3 套不同的整体方向，分别说明：品牌气质、色彩/材质、公开首页结构、登录后工作台结构、关键页面样式、竞品启发、实施成本与风险。
- 竞品参考以 AI API Gateway / Model Router / LLM Observability 类产品为主，至少覆盖 OpenRouter、Portkey、Helicone、LiteLLM 的公开信息架构或视觉取向；竞品只作为参考，不复制其品牌资产。
- 方案必须覆盖“公开营销面”和“登录后操作面”，不能只调整颜色或首页 Hero。
- 方案要保持当前业务事实：不虚构模型、用量、价格、供应商状态或交易数据；数据为空时使用诚实的空状态。
- 图片系统要考虑 CSP、加载失败、版权、性能和多语言替代文本；生产实现优先把确定素材本地化或可审计化。

## Acceptance Criteria

- [ ] 交付 3–4 套可比较的设计方案，并明确推荐方案及推荐理由。
- [ ] 每套方案都包含公开首页、认证工作台、导航/信息架构、色彩/材质、图标/图片策略和动效原则。
- [ ] 明确现有代码可复用区域与需要重构的区域，至少引用 `web/src/features/home/`、`web/src/components/layout/`、`web/src/styles/` 和 `web/src/lib/theme-customization.ts`。
- [ ] 明确不改变的兼容性边界：API、路由、权限、账务与国际化覆盖范围。
- [ ] 以一个用户问题收敛主方向；本轮不修改产品代码、不提交、不部署。

## Notes

- 本文档只记录方案阶段的需求与验收，不代表已批准实现。
- 用户确认方向后，再生成 `design.md` 与 `implement.md`，并单独获得“开始实现”批准。
