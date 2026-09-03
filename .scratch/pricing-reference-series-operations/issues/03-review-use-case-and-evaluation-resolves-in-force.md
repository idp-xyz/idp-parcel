# 复核用例；评价用例形成前解析在用版本补齐取值；CLI 增复核种类；seedgen 重跑

Category: enhancement
Status: in-progress——MCP-3（2026-09-03，owner 在通道 3 指示「继续」）
Blocked by: 02（已 resolved，`1b09c2d`）

## 要建什么

按 ADR-0099 决定三、四，在 `internal/parcelpricing/application` 与 `cmd/parcel-pricing-register` 落地：

1. **`ReviewReferenceSeriesHandler`**：入参（租户、序列版本引用、复核责任方、结论、依据、复核时刻由 `ports.Clock` 给）；先经 `ReferenceSeriesRegister` 读回被复核版本（拿登记责任方给四眼门），再 `Record`。结果代数：`已记录` / `版本不在册` / `四眼不满足`（领域构造门拒） / `未决`。
2. **`EvaluatePricingHandler` 增可选依赖** `SeriesResolver`（组合 `ReferenceSeriesInForceResolver` + `ReferenceSeriesRegister.ResolveAt`）：在 `EvaluatePricing` 之前，对方案的每条绑定，若输入快照**尚无**该种类取值 → 解析在用版本（`at` = 评价形成时刻，取 `Clock`）→ `ResolveAt(计价基准时点)` → 取值进输入快照。已有取值的（重放、今天的全部调用方）不动。解析不到时**不在编排层编造结果**：让输入缺取值进入纯函数，由既有 `REFERENCE_SERIES_UNRESOLVED` / `EXCHANGE_RATE_UNRESOLVED` 落待判断；但把原因文字（无版本 / 未复核 / 期次缺口）附进解释——实现上可由编排把一条「解析备注」放进请求，或让纯函数按缺失原因取值；二选一，判据是**领域不读端口**且解释里读得出三格之一。
3. **计价基准时点从哪来**：核 `EvaluationRequest` / `PricingInputSnapshot` 今天携带的业务时间引用，用它；没有则本票补一个显式字段而不是拿形成时刻顶替（两个时点分开是 ADR-0099 的核心）。
4. **CLI `-kind reference-series-review`**：输入一份复核 JSON（租户、序列 ID、版本、复核责任方、结论、依据），供 seed 与受控批量。退出码沿既有四格。
5. **seedgen 重跑**：`PPC-4` 价卡产物；两条 SYN 序列各加一份复核种子（复核责任方用 `SYN-PRICING-REVIEW-01`，≠ 登记责任方 `SYN-PRICING-OPS-01`）。`scripts/demo-seeds/seed.sh` 顺序：序列登记 → 复核 → 价卡。

## 红线

- 编排零算术、不改评价语义；纯函数仍不读端口。
- 免复核来源的「系统代写复核」不在本票（属 06）。

## 验证

应用层替身测试：有取值不解析；无取值解析成功后评价完成且清单含序列版本；无在用版本时评价待判断且解释含原因格；重放不解析。CLI 单测三格。`seed.sh` 在真库跑通并能形成一条 `S` 评价。

## Comments

- 2026-09-03 · MCP-3（应 MCP-4 两次询问「本票能否转 resolved」而答）：**不能，但卡的只有验证行
  最后一格，「要建什么」五项已在 `f62d619` 全部落地。**

  **五项逐条对得上**：`ReviewReferenceSeriesHandler`（含伴生读口 `ports.ReferenceSeriesVersionLoader`
  与七格结果代数）、`EvaluatePricingHandler` 的 `InForce` + `SeriesVersions` 成对可选依赖、计价
  基准时点与形成时刻分开取（形成时刻定版本、基准时点定期次）、CLI `-kind reference-series-review`、
  两份 SYN 复核种子与 `seed.sh` 的登记 → 复核顺序。

  **欠的那一格是「`seed.sh` 在真库跑通并能形成一条 `S` 评价」，它没跑，原因写在 `f62d619` 的提交信
  里：WSL 够不到 Windows 回环上的 PG 门禁容器。** 这是本机环境限制不是遗漏——`seed.sh` 是 bash
  脚本，只能在 WSL 侧跑，而那一侧连不上跑在 Windows 侧的库。**同一台机器上 Go 的真库用例是跑得通
  的**（TF postgres 包在 `828dbfa` 上实跑过），所以「PG 起着」与「seed.sh 跑得了」是两件事，别把
  前者当成后者的证据。

  **对票 04、05 的实际意义：它俩不必等本票的票面。** 它俩依赖的是代码，而代码已在库里
  （`f62d619`，已是主线祖先）。挡着它们的只是本票状态栏那四个字，而那四个字挡的是一次**真库
  冒烟**、不是任何一处它们要调的东西。认领 04 或 05 的人可以直接开工；本票留 in-progress，等
  那一格有人能跑时补上，或者由 owner 裁定把它拆成单独一票。

  **本条不改结论只补出处**：状态从 `ready-for-agent` 翻到 `in-progress` 那一笔此前一直躺在工作树
  里没提交（MCP-4 于 `a01cee4` 时点报出），随本条一并入库。
