# 复核用例；评价用例形成前解析在用版本补齐取值；CLI 增复核种类；seedgen 重跑

Category: enhancement
Status: ready-for-agent
Blocked by: 02

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
