# 管理台构建块压回告警线内：页面按域懒加载 + 上游图标选择器移出主入口

Category: enhancement
Status: in-progress——2026-09-24 通道 1 按用户令开票；01 ready-for-agent，02 ready-for-human（上游半边归用户）
出处：2026-09-24 用户问 `vite build` 的 500 kB 分块告警是不是 legal-entity-profile/04 的改动造成的——查明不是（干净 `HEAD` 同样告警，`vendor` 块逐字节不变）；再按用户「看看如何彻底解决」做原型实测，用户同意按建议开票。

## 现状（钉 `4244b1bb`，干净检出实测）

`vite build` 报「Some chunks are larger than 500 kB」，超线的是两块：

| 块 | 压缩后（gzip） | 构成（压缩前） |
|---|---|---|
| `vendor` | 1,187.08 kB（244.04 kB） | `lucide-react` 1,550 个图标模块 773 kB、`country-flag-icons` 全部国旗 336 kB；其余是 radix、floating-ui、tailwind-merge 等 |
| `index`（入口，应用代码） | 600.44 kB（163.76 kB） | 全部 `src/`，页面经 `page-registry.tsx` 全量静态导入；`pages/party` 最大，324 kB |

两个病根互不相干：

- **`vendor`**：`@idpxyz/ui-primitives@0.1.25` 的主入口 `dist/index.js` 里有 `IconPicker`，写着 `import * as LucideIcons from 'lucide-react'` 与 `import * as Flags from 'country-flag-icons/react/3x2'`，并在模块顶层执行 `Object.entries(LucideIcons)`、对全部国家跑两遍 `Intl.DisplayNames`、遍历全部国旗。顶层代码打包器摇不掉，于是整套图标与国旗进包，每次启动还算一遍。而 admin-web 与它直接依赖的其余五个 `@idpxyz` 包对 `IconPicker`、`renderIconValue`、`isFlag`、`getFlagCode`、`FLAG_PREFIX` 零引用。
- **`index`**：应用代码本身。拆块那笔 `84a41916`（2026-08-24）提交信记的应用块是 74 KB，一个月后 600 kB，每张票还在涨。

两处现行文字因此失真：`vite.config.ts` 拆块注释的「也让单块体积回到告警线内」对 `vendor` 从未成立（`84a41916` 当时即 1.17 MB），「重量在依赖不在页面」对 `index` 已不成立；`apps/admin-web/README.md`「已知跟进」把 `vendor` 归因于 framer-motion / radix / d3 / highlight，其中 framer-motion、d3、highlight.js 今天根本没装。

## 原型实测（钉 `4244b1bb`，仓外副本，已丢弃）

| 方案 | 入口块 | `vendor` | 超线块 |
|---|---|---|---|
| 现状 | 600.44 kB | 1,187.08 kB | 2 |
| 只做 A：`ui-primitives` 主入口去掉 `IconPicker` 那一块 | 600.44 kB | 211.05 kB | 1 |
| 只做 B：页面按域懒加载（13 个域 barrel） | 69.40 kB | 1,187.08 kB | 1 |
| A + B | 69.40 kB | 211.05 kB | 0（最大 211.05 kB；懒加载块最大为 party，205.63 kB） |

A + B 后首屏 JS（入口 + `react-vendor` + `ui-kit` + `vendor`）合计由 2,066 kB 降到 560 kB，gzip 由 491 kB 降到 168 kB。

- A 的模拟：构建时把 `ui-primitives` 的 `dist/index.js` 换成去掉 `IconPicker` 块、三条命名空间导入与五个导出名的版本，即子路径入口落地后 admin-web 看到的主入口。
- B 另测一格：`main.tsx` 仍从 barrel 导入 `configureShipmentRequestApi`、`configureVisibilityApi` 时，这两个域连同模板约 100 kB（压缩前）被钉回入口块。

## 子票

| 票 | 标题 | 状态 |
|---|---|---|
| [01](./issues/01-lazy-load-pages-by-context.md) | 页面按域懒加载，首屏块压回告警线内（B） | ready-for-agent · 开工前等共享树上在途的前端票提交 |
| [02](./issues/02-icon-picker-off-main-entry-and-chunk-gate.md) | 上游把 `IconPicker` 移出主入口，本仓换装新包并把超线改为构建失败（A + 门禁） | ready-for-human · 上游 `idp-ui` 半边归用户或另开 `idp-ui` 会话；Blocked by 01 |

走法：两张票本仓那半的 diff 都只在 `apps/admin-web/**` 与票面，走 [workflow.md「前端切片」](../../docs/agents/workflow.md#前端切片一人在-main-上直接做)。

## 不做

- 不调高 `build.chunkSizeWarningLimit`：那只是关掉告警，下一次涨到 1 MB 也没有信号。
- 不在本仓用 `pnpm patch` 给 0.1.25 打补丁过渡：告警不挡任何门、产品尚无租户在用，不值得在本仓维护一份上游代码的补丁；且 `pnpm patch` 与 `file:` tarball 覆盖、CI 的 `--frozen-lockfile` 能否同用未验证。
