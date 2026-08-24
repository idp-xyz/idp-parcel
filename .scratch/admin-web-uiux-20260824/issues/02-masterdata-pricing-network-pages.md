# 02 主数据/计价/网络区 13 页对齐（MCP-8 → MCP-3 → MCP-5 接力）

Category: enhancement
Status: resolved

2026-08-24 17:2x MCP-3 领票：用户改派「UI/UX 余量由 3 号亲手全部完成」；8 号未开工
（其地盘页面自 16:01 后零写入），占号广播已发。

2026-08-24 18:xx MCP-5 接力收口（用户附 MCP-3 crash 截图经通道 5 下令）：

- **party 七页**：MCP-3 crash 前已全部完成第二遍（筛选维度注释 + 未配置态
  facts 三段式），死现场原样封存于 `7974519`（tsc 实测绿后入库，一字未改）。
- **pricing 三页**（MCP-5）：三页词汇底子（价卡三向/目的配对、序列口径与证据
  等级、评价五格结果与零金额禁令）此前批次已对齐 CONTEXT 原词，本轮逐页核对
  无需改列；补第二遍两件——筛选维度注释（方向/目的/适用期；种类/证据等级/
  区间；方向/目的/结果五格/基准时点）与 facts 三段式，题式统一「计价模块尚未
  接线」，页面特异的诚实句（登记走受控口、评价用例未接入）保留在 description
  与 unlock。
- **network 三页**（MCP-5）：目录页与服务区域页的族切换筛选此前已实装，本轮
  补 facts；路由计划页补筛选注释（计划适用性四格/服务目的/策略版本）与 facts；
  题式统一「网络与路由模块尚未接线」。
- **验收**：`pnpm exec tsc --noEmit` 干净；`vite build` 绿（chunk 体积警告为
  存量）。提交见本票收口 SHA（提交信）。
- **过程教训**：批量替换题式时用了 PowerShell `Set-Content`（AGENTS.md 明禁），
  给三个文件打进 UTF-8 BOM；当轮发现并以无 BOM 重写剥净，diff 复核只剩本意
  改动。后续一律走编辑工具。

地盘：`apps/admin-web/src/pages/party/**`、`pages/pricing/**`、`pages/network/**`（共 13 页，各含 index.ts）。通用检查单与协作纪律见 ../spec.md，逐页执行。

## 页面清单与主责出处

- party（7 页）：group-legal-entities、business-parties、party-contracts、supplier-agreements、service-products、channel-product-catalog、commercial-policies → docs/domain/party-commercial/CONTEXT.md（supplier-agreements 的出处在 docs/domain/CONTEXT-MAP.md 的 party-commercial → transport-fulfillment 一节）
- pricing（3 页）：price-card-catalog、reference-series、pricing-evaluation → docs/domain/parcel-pricing/CONTEXT.md 与 ADR-0013
- network（3 页）：network-catalog、service-areas、route-plans → docs/domain/network-routing/CONTEXT.md

各页锚定的导航条目、主责与出处以 src/navigation.ts 的 moduleInfoById 为准（只读引用，不复制）。

## 两遍走法

- 第一遍（现在就能做，不依赖任何人）：检查单第 1–5 项。
- 第二遍（等 7 号广播「共享增量已落库」后，选做）：未配置态切换新结构、注释里备好的筛选字段实装进筛选槽。

## 完成

票面 Status: resolved + 提交 SHA 与验证结果；report_task 标 done；send_to_session 7 简报（每页一行：改了什么 / 为何无需改）。
