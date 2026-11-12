# 27 管理台写明估价申报缺什么、谁来补

Category: enhancement
Status: resolved——2026-11-12 进 main
Blocked by: 26（已解除：票 26 实现与评审完毕，拣入 `feat/goods-details-admin` 为 `cd2474100`；详情新字段 `lastAttemptEstimateGapCodes`，与票 23 的 `lastAttemptSettlementReason`、`lastAttemptServiceProduct` 同组）

依据：[ADR-0381](../../../docs/adr/0381-an-incomplete-estimate-declaration-waits-for-customer-supplement-or-operator-registration-by-who-supplies-the-gap.md) 决定五；票 24 的停因展示（续办方取处理尝试那一格）与票 26 钉下的字段。

## 要做的

- 委托详情的停因明细：
  - 等受控补充＋结算原因 `ESTIMATE_DECLARATION_INCOMPLETE` → 「估价没形成：申报缺了计价要的资料（……）——客户补充后自动续办」，括号里逐条列缺口（任务缺口那一面：指件的带件、委托级的写「委托缺……」）。
  - 等运营登记＋同一结算原因（缺项含 `ZONE` / `ADDRESS_NATURE`）→ 写明这张卡做不了受理前估价与该登什么，例如「估价没形成：在用的卡要直接给分区，受理前估价给不出——登记一张带分区目录、生效期覆盖提交时点的售价卡后自动续办」；词取 `pages/pricing/presentation.ts` 里 `estimateMissingLabels` 既有的运营译法，缺项码取票 26 处理尝试新那一格。
- 补件队列与详情的缺口翻译（`supplement-gaps.ts` 的 `missingWords`）补四个新原因码的中文词条：`ESTIMATE_MEASUREMENT_MISSING`（缺重量或量纲）、`ESTIMATE_DIMENSIONS_MISSING`（缺尺寸）、`ESTIMATE_RECIPIENT_POSTAL_MISSING`（缺收件邮编）、`ESTIMATE_ORIGIN_POSTAL_MISSING`（缺寄件邮编）；码不与别组撞，未知码照旧原样显示。
- `stall-reasons.ts` 的按码词条补齐这一类（票 24 已在那里挂过词条）。
- 计价缺项码新两码（`ORIGIN_POSTAL`、`DIMENSIONS`）在 `estimateMissingLabels` 补运营译法。

## 完成判据

- 单测盖：等受控补充的停因文案（带缺口清单）、等运营登记的卡侧缺项文案、四个新缺口词条、未知码回退。
- `tsc`、`run-tests`、`vite build` 过。

## 不做的

- 后端字段（票 26）；浏览器手点（与票 24 同一口径，记在完成记录里）。

## 完成记录（作者一侧，2026-11-12）

- 提交：`feat/goods-details-admin` 上的 `8e9054004`（叠在票 26 的 `cd2474100` 上），只含 `apps/admin-web/**`，一笔，未推送。
- 落点：`api.ts` 加 `lastAttemptEstimateGapCodes?: string[]`；`stall-reasons.ts` 底句改「申报缺了计价要的资料」、补 `CUSTOMER_SUPPLEMENT_WAIT_NOT_SAVED` 词条、新出卡侧缺项词表 `estimateGapCardSideWords`（ZONE / ADDRESS_NATURE 的卡侧说法与该登什么）；`last-attempt.ts` 的 `declarationIncompleteViewOf` 按缺项该谁补说细，卡侧缺项时续办句改说该登什么样的售价卡并照旧链价格表；`supplement-gaps.ts` 补四码词条（缺重量或量纲、缺尺寸、缺收件邮编、缺寄件邮编，未知码照旧原码回退）；`pricing/presentation.ts` 的 `estimateMissingLabels` 补 `ORIGIN_POSTAL`、`DIMENSIONS` 两条运营译法。
- 文案：等受控补充——「估价没形成：申报缺了计价要的资料（委托缺收件邮编、DPCL-2 缺重量或量纲）——客户补充后自动续办」；等运营登记（缺项含 ZONE）——「估价没形成：在用的卡要直接给分区，受理前估价给不出」＋「谁来续办：登记一张带分区目录、生效期覆盖提交时点的售价卡后自动续办」（链 `#/price-card-catalog`）；ADDRESS_NATURE 与两码同场各有句式。
- 门禁（apps/admin-web，Node 24）：`tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 1412 过 0 败（票 24 基线 1406，新增 6）；`vite build` 退 0。登录壳默认 Node 18 跑基线本就有 3 条与本票无关的失败（`crypto`/`File` 全局缺失），换 Node 24 全绿——环境问题，记录在案。
- 与票面的偏离：`CUSTOMER_SUPPLEMENT_WAIT_NOT_SAVED` 词条一并补（契约测试对着 Go 源核词表，不补即红，归「按码词条补齐这一类」）；卡侧停因句按票面例句原样落、不逐字拼 `estimateMissingLabels` 长括注（词条表注释注明对应）；缺口清单委托级在前（照 `supplementGapLinesOf` 既有排法，票面未钉序）；服务产品接在缺口括号后。
- 未做：浏览器手点；与后端联调（照 `query_shipment_request_views.go` 钉死的字段名写）；四个新缺口码对 Go 源的契约测试（既有契约不盖补件缺口码，票面未要求）——评审判此条该补，已随评审修订落。
- 评审（非作者，2026-11-12）：无阻断，结论可收口。评审员逐字核了文案对 ADR-0381 决定五（等运营登记的停因句与续办句与 ADR 例句逐字一致）、字段对 `query_shipment_request_views.go`（`lastAttemptEstimateGapCodes`、四个缺口码、`CUSTOMER_SUPPLEMENT_WAIT_NOT_SAVED` 均逐字对齐）、询查面完整性（缺口词条的页面消费者仅详情一处，经同一函数覆盖）、四条自报偏离全部成立；并复跑三道门禁全绿。两条应改、两条顺手措辞随评审修订提交 `d93e85a01` 落：
  - 契约护栏：`stall-reasons.contract.test.ts` 照既有读 Go 源提取常量字面的手法补两条——四个补件缺口码对 `pre_acceptance_control.go` 的 `gap…Missing` 常量双向核（`supplement-gaps.ts` 新出 `knownEstimateSupplementGapCodes` 供核）；计价六码封闭集对 `form_estimate_evaluations.go` 的 `EstimateMissing*` 提取，断言 `estimateGapCardSideWords` 键集落在集内、`estimateMissingLabels` 键集与封闭集相等（动机：`declarationIncompleteViewOf` 用字面码判卡侧缺项，计价改码时这里先红、不再静默退化）。
  - 文案组合钉整 view：客户补充「缺口清单＋服务产品同场」一条整 `deepEqual`；卡侧两码同场由只断续办句改成整 view（停因句「在用的卡要直接给分区、要收件地址性质，受理前估价给不出」，顺序随词表不随来码）。
  - 措辞：`estimateMissingLabels` 补 `MEASUREMENT` 词条（封闭集六码至此全覆盖，与上述契约测试互为支撑）；`declarationIncompleteViewOf` 头注点明卡侧分支有意不看续办方（后端若违约交来卡侧码＋客户补充续办方，宁可少一句也不照错的那一格催客户）。
  - 留给下笔：详情与复核页技术详情未随缺项码补原码行（票 24 给结算原因码、续办方加过原码行，`lastAttemptEstimateGapCodes` 尚无对应行）。
  - 修订后门禁（apps/admin-web，Node 24）：`tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 1414 过 0 败；`vite build` 退 0。分支头 `d93e85a01`，仅动 `apps/admin-web/**`。
## 进 main 记录

- 2026-11-12 随 declared-goods-lines 链进 main，main 尖 `c5001b3de`；重放、改号（分支 0378、0380 落地为 ADR-0382、ADR-0383）、生成物与门禁的完整记录见 [25 号票](./25-an-incomplete-declaration-for-the-estimate-waits-for-customer-supplement.md)「进 main 记录」。
