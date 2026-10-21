# 155 NR 初始路由的未配置类未决照样重投到 ABANDONED

Category: bug
Status: needs-triage——2026-10-21 Claude Code 会话立（票 131 实施时读代码发现，与 131 同族；用户「要我裁定的部分，你作为业务和系统专家，请你自决吧」授权）
Blocked by: 无
出处：票 [131](./131-network-reassessment-undecided-is-split-by-reason.md) 实施；[ADR-0235](../../../docs/adr/0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 决定六

## 现象

初始路由一路（接受决定信封 → NR 初始路由，UC-NR-001）由 `nrparcelshipment.outcomeToConsumption` 折成消费门两格：只要有一件包裹停在未决，就交 `ErrRouteHandoffUndecided`，整封回滚重投。

包裹级未决原因里有几格重投推不动：网络证据未配置、路由策略版本没声明排序形态、候选成本取数侧没接计价输入路径。它们与票 131 修掉的复核那三格同族，信封会重投到派发预算耗尽、落 ABANDONED，同一分区之后的信封排在它后面。

初始路由一直不形成，之后这件包裹的每一次复核都落「没有路由历史」，入账未判。

## 三类

机制。规模 M。租户取值：无（未配置那几格本来就答未配置，不补默认）。

## 最小的诚实修法

照 ADR-0235 决定一、二的形状办：初始路由的未决原因按恢复动作分格，重投推不动的那几格入账、留下原因，依赖类的照旧重投。

与复核不同，初始路由是一次交接里逐件包裹的结果，一封信封里可能有的包裹判成、有的停在未配置。要先裁两件：整封入账时，判成的那几件照常提交，停住的那几件各留一行原因；原因落 NR 的哪一册（复用复核的未判登记，还是初始路由另立一册）。续办并入票 [154](./154-network-reassessment-not-judged-triggers-resume-after-registration.md) 的同族修法。

## 要裁的

逐包裹的未判原因落哪一册；同一封交接里判成与未判并存时消费门怎样折。归 NR owner。

## 既有票

[131](./131-network-reassessment-undecided-is-split-by-reason.md)（复核一路，已修）；[154](./154-network-reassessment-not-judged-triggers-resume-after-registration.md)（续办）。

## 文档自洽审计复核（2026-10-21）

审计条目：[audit/nr-no.md](../../docs-consistency/audit/nr-no.md) H1（归 C，高）落在本票，不另立；它把本票的范围扩大了两处：

1. **不只是未配置类**：最低成本并列（`domain.RankingTied` → `RouteCandidatesTied`）、成本待判断、不可计价、币种不齐也一样落包裹级未决并整封回滚重投，重投多少次都不变。NR CONTEXT「路由形成与选择」要的是「初始路由遇并列既不形成计划也不形成无当前有效路由，保留全部候选与并列理由并保持可续办」；今天并列候选与理由只在内存结果里，信封落 `ABANDONED` 之后什么也没留下。CONTEXT 承认首发形态下「单维恰恰最容易打平」，并列会是常见情形。
2. **兄弟包裹连坐**：`adapters/postgres/initial_route.go` 的 `InitialRoutes.Save` 走 `RequireExecutor`、与消费门同一事务，整封回滚时同一委托里已判成的包裹计划一起撤掉，与 UC-NR-001「某个包裹未决或无路由不回滚其他包裹已经合法形成的计划」、`AT-NR-012` 相反。

按 main `67438a0f` 复核，`adapters/parcelshipment/route_on_acceptance.go` 的 `outcomeToConsumption` 仍是「任一件 `ParcelRouteUndecided` 即 `ErrRouteHandoffUndecided`」，现象全在。

扩充后的最小修法：初始路由的持久未决（并列、各未配置格、成本待判断、不可计价、币种不齐）照 ADR-0235 决定一、二的形状入账并留下原因；并列候选与并列理由落库；逐包裹提交，判成的那几件照常形成计划，不连坐。「要裁的」两件不变，再加一件：并列的续办入口（授权角色人工选择，UC-NR-001「人工选择」）是否同票。

分级建议（文档自洽 02）：R1——第一天就撞（单维成本并列常见），后果是整单无路由且不留痕，无绕法。由协调方定稿。

## 备注（2026-10-21，票 demo-chain-gaps/115 隔离评审）

同族一格：候选成本取数侧的确定性错误也落进了重投桶。`internal/networkrouting/adapters/parcelpricing` 的 `errUntranslatableEvaluation`（含候选没有线路引用、比较币种译不出）与 `errNotACostEvaluation`（方案方向或用途配错）重投多少次都一样，初始路由却把它们与依赖读不回一起答 `COST_SOURCE_UNAVAILABLE`、回滚重投。票 115 让复核照同一口径接取数侧，无路由那条线上同样答 `REASSESS_COST_SOURCE_UNAVAILABLE`、回滚重投，继承了这一格。修时两路一起分：确定性错误不进未决哨兵。
