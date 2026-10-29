# 01 国家 / 地区、币种、计量单位没有基础数据目录

Category: enhancement
Status: in-progress——2026-10-28 作者认领，分支 claude-refdata（起点 `f9dc00a6a`）。
Blocked by: 无

## 现象

取证于 main `f9dc00a6a`：币种只有 `parcelpricing/domain` 的 `Currency`（三位大写形状加 `MinorUnitScale` 内置小表）；国家只有 `networkrouting/domain` 的 `isCountryCodeShape` 与 `partycommercial/domain` 的两位大写形状校验，注释写“不内置码表”；计量单位是 `parcelpricing/domain` 的 `WeightUnit`、`LengthUnit` 与各处字面量，重量单位不一致报 `ErrWeightUnitMismatch`、没有换算。三者都没有名称、没有表、没有页面，操作者无处查“LB 是什么”“THB 小数几位”，管理台把国家码、币种码原样摆出来。

## 做什么

设计与取舍在 [ADR-0335](../../../docs/adr/0335-countries-currencies-and-units-are-product-built-in-read-only-reference-data-released-with-the-code-and-unit-conversion-keeps-precision-and-is-not-yet-wired-into-pricing.md)（会话代产品 owner 接受、待复核）。

1. `internal/platform/refdata`：国家 / 地区（ISO 3166-1 alpha-2 全量）、币种（ISO 4217 常用集带小数位）、计量单位（四类别、基准单位加精确系数）三份只读目录；`Country`、`Currency`、`Unit`、`Convert`。`Currency.MinorUnitScale` 改查目录，行为不变。
2. 三个运营查阅读口 `GET /reference-data/countries`、`/currencies`、`/units`（`?q=`、`?region=`、`?category=`）。
3. 管理台“设置”组新页“基础数据”（id `reference-data`），三签只读；译名显示件 `countryLabel` / `currencyLabel` 接进三个现在直接摆码的列表页。

## 完成判据

- 目录完整性单测：码唯一、全大写、币种小数位与旧表一致、国家数等于 ISO 已分配数、币种常用集覆盖、每个单位的基准单位存在且同类别。
- `Convert` 单测：LB→KG、KG→G、跨类别拒绝、往返精度。
- 读口 HTTP 用例：检索、分类过滤、租户必取自信封、未授予 403、空检索答全表。
- `internal/architecture` 门禁过；前端 `countryLabel` / `currencyLabel` 与页面纯逻辑单测。
- 不收紧任何现有校验，不把 `Convert` 接进计价。
