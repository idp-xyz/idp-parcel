# 27 管理台写明估价申报缺什么、谁来补

Category: enhancement
Status: ready-for-agent
Blocked by: 26

依据：[ADR-0381](../../../docs/adr/0381-an-incomplete-estimate-declaration-waits-for-customer-supplement-or-operator-registration-by-who-supplies-the-gap.md) 决定五；票 24 的停因展示（续办方取处理尝试那一格）与票 26 钉下的字段。

## 要做的

- 委托详情的停因明细：
  - 等受控补充＋结算原因 `ESTIMATE_DECLARATION_INCOMPLETE` → 「估价没形成：申报缺了计价要的资料（……）——客户补充后自动续办」，括号里逐条列缺口（任务缺口那一面：指件的带件、委托级的写「委托缺……」）。
  - 等运营登记＋同一结算原因（缺项含 `ZONE` / `ADDRESS_NATURE`）→ 写明这张卡做不了受理前估价与该登什么，例如「估价没形成：在用的卡要直接给分区，受理前估价给不出——登记一张带分区目录、生效期覆盖提交时点的售价卡后自动续办」；词取 `pages/pricing/presentation.ts` 里 `estimateMissingLabels` 既有的运营译法，缺项码取票 26 处理尝试新那一格。
- 补件队列与详情的缺口翻译（`supplement-gaps.ts` 的 `missingWords`）补四个新原因码的中文词条：`ESTIMATE_MEASUREMENT_MISSING`（缺重量或量纲）、`ESTIMATE_DIMENSIONS_MISSING`（缺尺寸）、`ESTIMATE_RECIPIENT_POSTAL_MISSING`（缺收件邮编）、`ESTIMATE_ORIGIN_POSTAL_MISSING`（缺寄件邮编）；码不与别组撞，未知码照旧原样显示。
- `stall-reasons.ts` 一带如有按码的词条，补齐这一类。
- 计价缺项码新两码（`ORIGIN_POSTAL`、`DIMENSIONS`）在 `estimateMissingLabels` 补运营译法。

## 完成判据

- 单测盖：等受控补充的停因文案（带缺口清单）、等运营登记的卡侧缺项文案、四个新缺口词条、未知码回退。
- `tsc`、`run-tests`、`vite build` 过。

## 不做的

- 后端字段（票 26）；浏览器手点（与票 24 同一口径，记在完成记录里）。
