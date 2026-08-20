# 计价参考序列登记册缺失,燃油与汇率序列无处登记

Category: enhancement
Status: needs-triage

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W15。

## 墙

`REFERENCE_SERIES_UNRESOLVED`、`EXCHANGE_RATE_UNRESOLVED`(`parcelpricing/domain/evaluation.go`,评价落 `EvaluationPending`)。墙正确:序列解析不到时评价挂起,不编数值。

## 现状:门四件全缺

ADR-0013 已裁「计价拥有计价参考序列的登记、版本化、发布治理与按基准时点的解析」,但机制未建:`reference_series.go` 是纯内存领域对象,无表、无装载口、无写入方、无登记口。序列今天只能作为评价入参由测试构造。

## 缺的最小机制件

1. 序列登记册:表 + 迁移(来源标识、生效区间、逐期取值、取值凭证引用、登记责任方;取值更正形成新序列版本,不追溯改写)。
2. 装载口:按计价基准时点解析取值,供评价消费并写入版本清单。
3. 登记口:登记用例(缺可复核凭证的期次只有断言强度,只准隔离验证——登记结构须能表达这个等级)。

## 红线

- 序列数值由外部产生(燃油=承运商公布,汇率口径=party-commercial 商业价格政策声明);计价只登记不生产,不选定商业口径(ADR-0013)。
- PAR-SET-11 实例(两个序列的真实期次)待提供是常态;隔离取值不得冒充生产序列(PN-07 准入门槛原文)。

## 参照

ADR-0013;PAR-SET-11;`docs/domain/parcel-pricing/CONTEXT.md`。

## Comments

- 2026-08-20 MCP-2：对 `3324ecb` 重核四件，**结论不变：无门，四件全缺**。
  `reference_series.go` 仍是纯域内对象（`parcelpricing/domain/`），序列无表
  （`migrations/parcel_pricing` 仍只有 `0001_evaluation.sql`）、无端口
  （`ports/ports.go` 三口如旧）、无适配器（`adapters/postgres` 仅评价两件）、无进程
  入口（`cmd` 零引用）。基线 `49a2ab0` 以来 parcel-pricing 零提交，票面与代码无矛盾。
  与票 07 同根：两族配置的门都等同一套 PP 持久化骨架，先后与合并由 triage 定。
