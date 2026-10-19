# 47 预占声明「需求来自准备结果的需求快照」时，不回读核那份快照

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [20](./20-capacity-reservation-and-release-decision-faces.md) 的完成记录。
Blocked by: [18](./18-transport-preparation-consumes-the-initial-route.md)、[20](./20-capacity-reservation-and-release-decision-faces.md)
出处：[ADR-0203](../../../docs/adr/0203-capacity-assurance-requires-a-reservation-on-every-pool-of-the-schedule-and-reservations-are-operation-decisions.md) 决定三与越权风险点「需求来源不回读」。

## 现象

票 20 的预占与释放必须声明需求来源，种类封闭为运营声明（`OPERATOR_DECLARATION`）与准备结果的需求快照（`PREPARATION_DEMAND_SNAPSHOT`）两种，引用原样保存。后一种此刻不回读：引用指向一份不存在的准备结果、指向别的班次或别的维度、或声明的量与快照里的量不一致，预占照样成立。CONTEXT 要每次容量申请固定「采用的测量/数量版本」，来源能对回去才算固定住。

## 证据（读码）

`internal/transportfulfillment/application/prepare_transport_opportunity.go` 的 `capacityDemandFrom` 只过构造门；编排的依赖里没有运输准备结果的读口（准备结果册由票 18 立，与票 20 同波）。

## 三类

回读与核对是机制；对不上时答哪一格、量不一致时以谁为准是产品策略。规模 S 到 M。

## 最小的诚实修法

票 18 的准备结果册进 main 之后，预占编排在来源种类为准备快照时按（租户、引用）读那份结果：不在册答一个新格（续办是改引用或改用运营声明）；班次或维度对不上答冲突；量只记差异、不拒（声明量可以小于快照，部分预占是合法的）。读不动答未决。

## 要裁的

- 声明量大于快照量时拒还是只记差异。
- 运营声明那一种要不要也有一本「声明版本」册可核。

## 既有票

none
