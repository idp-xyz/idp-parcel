# 28 票 25 评审尾巴：估价停格分清溢出与负数、申报单位与正式评价同口径、留痕时点跨重驱的前提

Category: enhancement
Status: ready-for-agent——2026-10-13 19:3x 通道 1 立（用户 19:2x 经队列点头「按你的建议开始吧」），出自[票 25](./25-pre-acceptance-estimate-formed-by-settlement.md)两段非作者评审的非阻断：第二段 Standards 2、3 与 Spec 1（`task-386c6296`），其中 Standards 2、3 即第一段 Standards ②、①（`task-c7cfb347`），原文摘在票 25 的 Comments。
Blocked by: 无
地盘：`internal/settlementaccounting/domain`（`FormPerParcelSellTrialSum` 与停格）、`internal/settlementaccounting/application`（`NotFormedReason` 与停格到原因的映射）、`internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control.go`（只动 `notFormedOutcomeFor` 跟新原因）、`internal/parcelpricing/adapters/parcelshipment`（`measurementOf`）、必要时 `internal/parcelpricing/adapters/transportfulfillment/pricing_input.go` 的 `weightUnit` / `lengthUnit`、`internal/settlementaccounting/adapters/postgres` 估价留痕册的用例与头注、`cmd/parcel-dispatch` 的纵向用例；各处用例随改。不碰 PS 领域、PS 提交版本申报读口与 PS 编排（[票 27](./27-pre-acceptance-control-reads-only-the-current-submission-version.md)的地盘）。
出处：[ADR-0179](../../../docs/adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 决定二（停法）、决定三（与正式 SELL 评价同口径）、决定六（留痕）；票 25 判断项 8、9。

## 现象（读码，钉 main `6e4d9a1d`）

1. SA 领域 `FormPerParcelSellTrialSum` 对单份为负、加不进 int64 与合计不是正数同答 `PreAcceptanceEstimateNotPositive`，SA 应用层照映为 `EstimateNotPositive`（`ESTIMATE_NOT_POSITIVE`），经 PS `notFormedOutcomeFor` 落进处理尝试与续办引用。票 25 判断项 9 说分格「是为了续办引用与处理尝试里留下停在哪一格」，这一格把三种停法并成了一个。决定二只写了「合计不是正数」，单份为负而合计为正时也停，是多出来的一格。
2. PP `adapters/parcelshipment` 的 `measurementOf` 先 `strings.ToUpper` 再认单位；正式 SELL 输入解析 `adapters/transportfulfillment/pricing_input.go` 的 `weightUnit` / `lengthUnit` 只认大写原词。申报 `kg` 时受理前估得出冻结额，受理后正式评价却报单位在表外。`cmd/parcel-dispatch` 的 `preAcceptanceTrials` 把受理前试算接上生产图之后，这一差生产可达。
3. 估价留痕册 `PreAcceptanceEstimateTraces.RecordEstimate` 判重放与冲突时连 `trial_at` 一起比，两本账不比时刻。票 25 判断项 8：组合策略下冻结已落、账期那一项停`未配置`时事务照样提交，留痕随之已落；登记后重驱若换了判断时点，同一请求身份撞留痕冲突、落`请求冲突`，推不动。今天唯一形得成的时点语义是「提交接收」（PS `adapters/partycommercial` 的 `AsOfValueSource` 头注：折成该提交版本的系统接收时间），跨重驱不变——但票 25 没举证，也没写下这个前提。

## 做什么

1. 停格分清：溢出与单份为负各给一格，领域停格、SA 原因、PS 映射三处同笔，PS 侧仍落`未形成`。按决定二，单份为负而合计为正不在停法里；若留现状照停，写判断项并标越权风险点。
2. 单位同口径：`measurementOf` 与正式评价的单位解析取同一口径。两条路选一条并写判断项——受理前也不折叠（`kg` 当表外单位，与正式评价同停），或两边都折叠（改的是正式 SELL 评价的输入解析，那一侧既有用例要跟）。前者改动小、不动正式评价的行为。
3. 留痕时点：补一条用例，钉住同一请求在「提交接收」语义下跨重驱是重放、不是冲突（纵向或 SA 适配器层，开工时核）；在留痕端口或册的头注写明这个前提——换成会随重驱移动的时点语义时，这里会落`请求冲突`。

## 不做

- 不改 ADR-0179；读法与决定二、三对不上时停下报通道 1。
- 不碰票 27 的地盘；PS `pre_acceptance_control.go` 只动 `notFormedOutcomeFor`。
- 不改留痕表结构，迁移 `0035` 不动。

## 完成判据

- 第 1 条：三种停法各有用例、各落不同原因串；把任一格并回 `NOT_POSITIVE` 的变异转红。
- 第 2 条：同一份申报 `kg` 在受理前与正式评价两条路得同一答复，有用例。
- 第 3 条：用例为 PASS 非 SKIP（真库那条带 DSN `-v` 确认）；头注写明前提。
- 架构门禁、机制清点、受影响包组绿，口径照 [parallel-sessions.md](../../../docs/agents/parallel-sessions.md)「全量只跑一次」。证据层级只记 `S`。
