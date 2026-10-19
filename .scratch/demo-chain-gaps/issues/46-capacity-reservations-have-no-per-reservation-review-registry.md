# 46 逐份预占与释放明细没有查阅册：管理台只看得到池级三个和

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [20](./20-capacity-reservation-and-release-decision-faces.md) 的完成记录。
Blocked by: [20](./20-capacity-reservation-and-release-decision-faces.md)
出处：[ADR-0203](../../../docs/adr/0203-capacity-assurance-requires-a-reservation-on-every-pool-of-the-schedule-and-reservations-are-operation-decisions.md) 越权风险点「逐份预占没有查阅册」。

## 现象

票 20 让预占与释放成了运营决定，每份预占固定了需求来源与判断时点，每次释放留了明细（身份、数量、来源、时点）。但查阅口 `/transport-fulfillment-records?registry=capacity-pool` 只列池级的已预占、已释放、实际使用三个和，看不到是哪几份预占、各自来自哪一版声明或哪份准备结果、什么时候被放了多少。运营人员要释放某一份，只能记住自己当初起的预占引用；评审一次超配也找不到是谁占的。

## 证据（读码）

`internal/transportfulfillment/adapters/postgres/review_catalogue.go` 的 `ListCapacityPools` 对 `capacity_reservation` 求和后只交池级一行；`ports.CapacityPoolCatalogueRow` 没有逐份预占的格；`capacity_release` 子表（TF 迁移 0030）没有任何读口读它，除了池册重建。

## 三类

机制（查阅读面）。规模 S 到 M。

## 最小的诚实修法

查阅口加一册 `capacity-reservation`：逐份预占一行（池、预占引用、三量、来源种类与引用、判断时点、有效期），释放明细作为该行的子列表照实转写；挂主数据与运营查阅读，按租户读。管理台池区逐池展开预占，属前端切片另做。

## 要裁的

- 已过期的预占列不列、怎么排（按判断时点还是按有效期）。
- 释放明细放在预占行下还是另立一册。
- **池行时刻的语义**（票 20 隔离评审并入）。`CapacityPools.Replace` 每次换存都把 `capacity_pool.recorded_at` 改写成此刻；票 20 给它接上生产调用方之后，每次预占或释放都改它，池登记口重放 `EXISTING_POOL` 回显的 `recordedAt` 与查阅册池行的时刻都成了最后一次换存的时刻，不再是建池时刻。二选一：池行时刻保持建池时刻（`Replace` 不动 `recorded_at`，编排 `replacePool` 也不改记录上的时刻，逐份预占的时刻由本册各行自带）；或保留「最后一次换存」，在池登记口与查阅册的文档里写明。前者让登记口重放稳定，倾向前者；交 TF owner 定。

## 既有票

none
