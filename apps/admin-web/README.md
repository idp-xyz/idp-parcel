# parcel-admin-web — 租户管理台

产品自带的租户管理界面（[ADR-0021](../../docs/adr/0021-frontline-operations-client-is-part-of-the-product.md) 定序先行的那一端），与后端同仓、同版本发布；落位依据 [ADR-0018](../../docs/adr/0018-product-clients-share-release-boundary-under-apps.md) 经 ADR-0020/0021 保留的结论：产品内客户端落顶层 `apps/`，每端一个子目录。`packages/` 等出现第二个端且确有共享物时才建。

**这里不建两类界面**：租户的锚点货主客户走 `PAR-INT-01` 登记的租户既有渠道；开发方跨租户运维后台是显式未决，禁止顺手建立（均见 ADR-0020/0021）。

## 现状

外壳（主题、导航、路由）可用，其上已有四层多会话并行交付的产出：

- `src/domain/status.tsx` — 领域状态词表（47 词，词取权威文档原词）与 `StatusBadgeFor`；
- `src/components/states/` — 页面四态组件（加载/空/错误/**未配置**），「未配置」专门呈现 403 `ACCESS_CHANNEL_NOT_CONFIGURED`（[ADR-0022](../../docs/adr/0022-http-status-carries-answer-formed-not-business-verdict.md) 语义：HTTP 状态只表示答案是否形成）；
- `src/templates/` — 列表页/详情页/复核工作流三个页面模板（演示数据隔离在 `demo.ts`，标注合成 `S`，不进桶导出）；
- `src/pages/shipment-request/` — UC-PS-001 提交、UC-PS-005 决定前撤回、委托查阅（列表/详情，对 `GET /shipment-request-views` 真实查询契约取数，统一不可见结果按 CONTEXT 语义呈现）与 UC-PS-006 取消与收寄后处置（对 `POST /shipment-requests/parcel-cancellations` 逐件受理、逐件呈现，三种已提交走向同为有效业务答案），对 `parcel-api` 真实端点形状；接入渠道未配置时如实呈现「未配置」态；
- `src/pages/governance/` — 治理与复核类页面（接受前人工复核、异常分诊、对账单、收付款核销、阶段决定），模板骨架已按各自 CONTEXT 语义搭好（栏目、列、动作命名），数据区以「未配置」态如实呈现，不含合成数据；
- 导航按 CONTEXT-MAP 业务价值链分区覆盖全部十个限界上下文：35 个模块条目现已全部登记页面（已接线/骨架/演示三档、零规划占位），带主责与出处的 `UnwiredModule` 占位机制保留但当前无消费者——「导航先于页面」的外壳设计行为已走完它的占位期；
- `src/pages/template-preview/` — 模板预览页（导航「演示」区）：用隔离合成 `S` 数据实例化三套模板并带四态切换器，是 `templates/demo.ts` 的唯一合法消费者。

治理页的业务编排接线按 [ADR-0017](../../docs/adr/0017-admission-gates-judged-by-blocking-cause.md) 的闸门推进，页面不含未确认参数的默认值。

## 技术栈

Vite + React 19 + TypeScript + Tailwind CSS 3，UI 组件来自 [`idpxyz/idp-ui`](https://github.com/idpxyz/idp-ui) 的 `@idpxyz/*` 包（GitHub Packages，TypeScript 源码形态发布，消费方自行编译——因此 `tailwind.config.js` 要扫描 `node_modules/@idpxyz/*/src`）。外壳形态参考 loms-web 的传统控制台（品牌头 + 左侧导航 + 单页区）。

## 前置与安装

- Node 20+、pnpm 10+
- 从注册表安装 `@idpxyz/*` 需要带 `read:packages` 的 GitHub PAT（本目录 `.npmrc` 已指定 scope 注册表，凭据放用户级 `~/.npmrc`，不要提交）：

```
//npm.pkg.github.com/:_authToken=<PAT>
```

```bash
cd apps/admin-web
pnpm install
pnpm dev        # 开发服务器
pnpm build      # tsc 类型检查 + vite 构建
```

本机当前的 `node_modules` 是用 idp-ui `master` 构建的本地 tarball 装的（当时无 PAT），
与注册表工件同源同形（dist + d.ts）；配好 PAT 后重跑 `pnpm install` 即切回注册表来源并生成锁文件。

## 与后端联调

前端一律带 `/api` 前缀发起，`vite.config.ts` 的开发代理剥前缀转发到 `cmd/parcel-api`（默认 `:8080`，后端用 `IDP_PARCEL_HTTP_ADDR` 改监听、前端用 `PARCEL_API_TARGET` 改目标）。代理链已烟测：穿过代理后端收到 `/shipment-requests`（前缀已剥），403 未配置包封原样穿回。

本机两处暗礁（实测 2026-08-24）：
- **8080 被 Windows 服务（svchost）占用**——本机起 parcel-api 需 `IDP_PARCEL_HTTP_ADDR` 换端口，并给前端配同值 `PARCEL_API_TARGET`；
- **vite dev 只监听 IPv6 `::1`**——浏览器/curl 用 `localhost:5173`，用 `127.0.0.1` 会被拒。

## 版本与验证口径

`@idpxyz/*` 版本对齐 idp-ui `master` 当前包版本（0.1.23 / 0.1.25）。本骨架的构建验证是在 idp-ui workspace 内以同一份源码完成的（本机无 `read:packages` 凭据，装不了注册表包）；首次从注册表真实安装时如遇版本缺失，按注册表实际版本调整。

## 已知跟进

- CI：按 ADR-0018 的后果，第一个端落地后 CI 需新增非 Go 的独立 job；该 job 需要能读 GitHub Packages 的凭据（org secret 或包访问授权），尚未接。
- 产品色：`ui-tokens` 的 `productAccent` 尚未登记 parcel，`App.tsx` 暂用默认色；登记属上游 idp-ui 仓的改动。
- 产物体积：vendor 块约 1.17MB（gzip 约 239KB），构成是 framer-motion/radix/d3/highlight 等 `@idpxyz` 传递依赖——本应用只用到部分组件，但上游桶导出与缺 `sideEffects` 声明让未用的重依赖摇不掉。已按变更频率拆 `react-vendor`/`ui-kit`/`vendor` 三块保缓存（业务改动只失效 app 块）；根治（`sideEffects: false` 与子路径导出）属上游 idp-ui 仓。
