# parcel-admin-web — 租户管理台

产品自带的租户管理界面（[ADR-0021](../../docs/adr/0021-frontline-operations-client-is-part-of-the-product.md) 定序先行的那一端），与后端同仓、同版本发布；落位依据 [ADR-0018](../../docs/adr/0018-product-clients-share-release-boundary-under-apps.md) 经 ADR-0020/0021 保留的结论：产品内客户端落顶层 `apps/`，每端一个子目录。`packages/` 等出现第二个端且确有共享物时才建。

**这里不建两类界面**：租户的锚点货主客户走 `PAR-INT-01` 登记的租户既有渠道；开发方跨租户运维后台是显式未决，禁止顺手建立（均见 ADR-0020/0021）。

## 现状

外壳（主题、导航、路由）可用，其上已有四层多会话并行交付的产出：

- `src/domain/status.tsx` — 领域状态词表（47 词，词取权威文档原词）与 `StatusBadgeFor`；
- `src/components/states/` — 页面四态组件（加载/空/错误/**未配置**），「未配置」专门呈现 403 `ACCESS_CHANNEL_NOT_CONFIGURED`（[ADR-0022](../../docs/adr/0022-http-status-carries-answer-formed-not-business-verdict.md) 语义：HTTP 状态只表示答案是否形成）；
- `src/templates/` — 列表页/详情页/复核工作流三个页面模板（演示数据隔离在 `demo.ts`，标注合成 `S`，不进桶导出）；
- `src/pages/shipment-request/` — UC-PS-001 提交、UC-PS-005 决定前撤回、委托查阅（列表/详情，对 `GET /shipment-request-views` 真实查询契约取数，统一不可见结果按 CONTEXT 语义呈现）与 UC-PS-006 取消与收寄后处置（对 `POST /shipment-requests/parcel-cancellations` 逐件受理、逐件呈现，三种已提交走向同为有效业务答案），对 `parcel-api` 真实端点形状；接入渠道未配置时如实呈现「未配置」态；
- `src/pages/governance/` — 试点治理页（阶段决定）；曾一并放在这里的接受前人工复核、异常分诊、对账单、收付款核销已按 `src/navigation.ts` 声明的主责上下文归位到 `shipment-request/`、`visibility/`、`settlement/`——页面目录按限界上下文分，不按「治理与复核」这个呈现分组分；模板骨架按各自 CONTEXT 语义搭好，数据区以「未配置」态如实呈现，不含合成数据；
- 导航按 CONTEXT-MAP 业务价值链分区覆盖全部十个限界上下文：全部模块条目均已登记页面（已接线/骨架/演示三档、零规划占位，条目清单与各自档位以 `src/navigation.ts` 与 `src/page-registry.tsx` 为准，不在此复述计数），带主责与出处的 `UnwiredModule` 占位机制保留但当前无消费者——「导航先于页面」的外壳设计行为已走完它的占位期；
- `src/pages/template-preview/` — 模板预览页（导航「演示」区）：用隔离合成 `S` 数据实例化三套模板并带四态切换器，是 `templates/demo.ts` 的唯一合法消费者。

治理页的业务编排接线按 [ADR-0017](../../docs/adr/0017-admission-gates-judged-by-blocking-cause.md) 的闸门推进，页面不含未确认参数的默认值。

## 列表页上列通则

下列约定在主数据七页接线时逐页裁出（`.scratch/master-data-wiring/issues/07-admin-web-pages.md` 的「七项裁决」总则部分），但它们不是那七页专有的——每张对着查阅端点取数的列表页都会撞到，新页照此办、不必重裁；页面专有的裁决留在各自票面。

- **列面向已提交的读面收敛。** 骨架期的占位列不是设计目标，以端点真实答出的字段为准。
- **读面有、骨架无 → 上列。** 列名用 CONTEXT 原词的中文化说法；JSON 字段名不改，收编形状时照抄端点。
- **骨架有、读面无 → 不上列。** 用如实说明交代它为什么不在，不留空列也不填假值——先例是服务区域的地理覆盖列按 `PAR-NET-14` 说明「尚不存在」，并且不为它发请求。
- **展示层合成允许。** 并列、截断、`id@version` 这类合显是呈现手法，不构成第二套口径。
- **过滤只在已取回的数据上做。** 不把过滤条件下推成新查询参数——那要改端点契约。
- **计数摘要必须带 outcome 守卫。** 未配置态与错误态下不得显示「0 个版本／0 条」：那与状态区「这不是目录为空」直接矛盾，四态里只有空态配显示零。这条是接线时五页同时踩中后补上的，新页容易原样再犯。

## 技术栈

Vite + React 19 + TypeScript + Tailwind CSS 3，UI 组件来自 [`idpxyz/idp-ui`](https://github.com/idpxyz/idp-ui) 的 `@idpxyz/*` 包（TypeScript 源码形态发布，消费方自行编译——因此 `tailwind.config.js` 要扫描 `node_modules/@idpxyz/*/src`）。外壳形态参考 myshop-web 的多标签工作区（品牌头 + 左侧导航 + `EditorGroup` 多标签主区 + 右侧检查器栏 + 底部状态栏；hash 仍是位置权威，标签集是它的镜像——取舍见 `src/Layout.tsx` 文件头）。

`@idpxyz/*` 七个包**不从注册表装**：tarball 随仓放在 `vendor/idpxyz-ui/`（idp-ui `master` 构建，0.1.23 / 0.1.25），`package.json` 的 `pnpm.overrides` 把每个包——包括只作传递依赖出现的 `ui-icons`——钉到对应文件。这样任何一次全新检出都能 `pnpm install --frozen-lockfile`，不需要读包凭据；`.npmrc` 里那行 scope 注册表只标明上游发布在哪，overrides 之下不会被访问。换版本的动作是三件一起：替换 tarball、改 overrides 那几行、不带 `--frozen-lockfile` 重装一次让 lockfile 跟上。

## 前置与安装

- Node 20+、pnpm 10（`package.json` 的 `packageManager` 钉 `10.32.1`，pnpm 10 默认会自行切到该版本）

```bash
cd apps/admin-web
pnpm install --frozen-lockfile
pnpm dev        # 开发服务器
pnpm build      # tsc 类型检查 + vite 构建
pnpm test       # scripts/run-tests.mjs
```

## 与后端联调

前端一律带 `/api` 前缀发起，`vite.config.ts` 的开发代理剥前缀转发到 `cmd/parcel-api`（默认 `:8080`，后端用 `IDP_PARCEL_HTTP_ADDR` 改监听、前端用 `PARCEL_API_TARGET` 改目标）。代理链已烟测：穿过代理后端收到 `/shipment-requests`（前缀已剥），403 未配置包封原样穿回。

本机两处暗礁（实测 2026-08-24）：
- **8080 被 Windows 服务（svchost）占用**——本机起 parcel-api 需 `IDP_PARCEL_HTTP_ADDR` 换端口，并给前端配同值 `PARCEL_API_TARGET`；
- **vite dev 只监听 IPv6 `::1`**——浏览器/curl 用 `localhost:5173`，用 `127.0.0.1` 会被拒。

## 版本与验证口径

`@idpxyz/*` 版本以 `vendor/idpxyz-ui/` 里的 tarball 为准（对齐 idp-ui `master` 当时的包版本 0.1.23 / 0.1.25）。门禁三道：`node node_modules/typescript/bin/tsc -b --noEmit`、`node scripts/run-tests.mjs`、`pnpm exec vite build`；CI 的 `admin-web` job（`.github/workflows/ci.yml`）在全新检出上按同一顺序跑，本机验证口径与它一致。走 TypeScript 入口文件而不是 `npx tsc`，是因为后者在缺包时会落到同名占位包上、退 0 却什么都没编。

## 已知跟进
- 产品色：`ui-tokens` 的 `productAccent` 尚未登记 parcel，`App.tsx` 暂用默认色；登记属上游 idp-ui 仓的改动。
- 产物体积：`vendor` 块超过 Vite 的 500 kB 告警线。主因是 `@idpxyz/ui-primitives` 主入口里的 `IconPicker`：它以 `import *` 引整套 `lucide-react` 与 `country-flag-icons`，并在模块顶层遍历，本应用用不到也摇不掉。根治是上游把它挪到子路径入口，本仓换装新包时把超线改成构建失败（`.scratch/admin-web-bundle-size/` 票 02）。页面已按域懒加载（`src/page-registry.tsx`），入口块在告警线内；`react-vendor`/`ui-kit`/`vendor` 三块仍按变更频率拆以保缓存。
