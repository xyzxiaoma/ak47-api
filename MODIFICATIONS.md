# Modifications

This project is a modified version of New API distributed under the GNU Affero General Public License v3.0. Upstream copyright, license, warranty, and attribution notices remain in effect.

## 2026-08-22

- Aligned automatic channel connectivity tests with the configured Chat Completions-to-Responses compatibility policy, including the existing global and per-channel pass-through gates.
- Added a public corresponding-source link to the shared footer and default About page. Docker builds accept `VITE_DERIVATIVE_SOURCE_URL` so deployments can point users to the exact published source revision.
- Added an explicit Rsbuild definition for the build-time source URL so production bundles preserve the exact deployment revision.
- Deployment tags: `ak47token-2026-08-22-relaxycode-adapter.1` (superseded), `ak47token-2026-08-22-relaxycode-adapter.2`.

## 2026-08-22（前端视觉重设计）

- 采用“断层控制室 + 编辑式公开首页”方向，统一公开站点、认证壳层、Dashboard 和核心业务页的暖色材质视觉。
- 增加本地 Iconify 图标注册表、噪点/纸张纹理背景与非线性动效，保留 API、路由、权限、账务和国际化契约不变。
- 部署标签：`ak47token-2026-08-22-frontend-control-room.1`。
