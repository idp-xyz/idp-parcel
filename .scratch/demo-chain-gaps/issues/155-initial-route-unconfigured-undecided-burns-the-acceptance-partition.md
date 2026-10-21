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
