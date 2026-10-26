# 121 先于段登记的出发永不补问门禁——本该阻断也不形成阻断信号

Category: enhancement
Status: in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领（票 135、136 已进 main），分支 `claude-dcg121`（worktree `idp-parcel-dcg121`）。此前 ready-for-agent——2026-10-20 代决复核撤掉用户决定阻断（M5），见 [owner-review-20261020](../../owner-review-20261020/report.md)。此前 needs-triage——2026-10-20 票 [53](./53-movement-facts-recorded-before-their-segment-never-reach-tracking.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0224](../../../docs/adr/0224-tracking-backfills-movement-facts-recorded-before-their-segment-when-a-control-fact-puts-the-object-in-it.md) 决定五与越权风险点 3。
Blocked by: 无——用户决定一格已于 2026-10-20 代决撤掉（M5）（此前 用户决定（TF、CC owner：补问机制的形状——ADR-0205 越权风险点 2「补问要另立机制」））
地盘：TF 出发门禁读数（`internal/transportfulfillment/application/record_movement_fact.go`、`domain/departure_gate_reading.go`、移动事实登记册的读数列）；VE 出发门禁阻断信号（`DeriveOnMovementFactAdapter.signalBlocked`）。

## 现象（读码钉票 53 的分支，只记 `S`）

- 出发的门禁读数在登记那一刻问关务（ADR-0205 决定二）。声明的段此刻不在册时，本侧停下不问，读数落未决、停点 `SEGMENT_NOT_REGISTERED`，没有逐对象那一格。
- 票 53 让这类出发在对象日后凭交接或对象级场外揽收进段时补进投影（ADR-0224），但不补问关务：读数仍是那一份未决，阻断信号只看逐对象那一格的阻断，于是这件包裹即使关务本该阻断，也永不形成「出发门禁阻断」信号、不进分诊，追踪上显示成一次普通的 `DEPARTED`（ADR-0205 越权风险点 17）。
- 段已在册、对象后来才凭交接或揽收进段（控制事实的业务时间早于出发）时同理：登记那一刻段里没有它，读数里也没有它那一格。

## 三类

- 机制：对停在本侧（段不在册、那一刻段里没有在控对象）的出发，在段与参与齐了之后按原出发时点重问，追加一版读数；追加的读数里出现阻断就形成信号。与读不动的补问是同一套机制（ADR-0205 越权风险点 2），要先把确定性缺陷与暂时读不动分开，否则会对确定性缺陷无限重问。
- 产品策略：补问的唤醒（交接进段那一刻、定时扫、人工触发）与次数上限。
- 租户取值：无。

## 要裁的

- 读数是登记那一刻的记录、只插不改（迁移 0026）：补问是追加一版读数（新形状），还是另立「补问记录」挂在事实上。
- 补问来的阻断形成信号时，发作期的业务时间取出发时点还是补问时点。

## 既有票

- [53](./53-movement-facts-recorded-before-their-segment-never-reach-tracking.md)、[114](./114-deterministic-failures-answered-as-retryable-undecided-across-consumers.md)（确定性缺陷被答成可重试未决）

## 代决复核追加（2026-10-20）

出处：[owner-review-20261020](../../owner-review-20261020/report.md) 的 M5。

- **撤掉「Blocked by 用户决定」**：票面自己把补问归为机制、把唤醒与次数归为产品策略，租户取值为无，按 AGENTS 的三类都由开发方现在就定。
- **两问按机制自决**：补问追加一版读数，不改首版；补问得到的阻断信号以出发时点为业务时间。
- **前置条件**：先把段册、班次册的「装不回」与「连不上」分开，这一步并进票 114 或在本票内做。
- 分级：上线后尽快（关务读不动期间本该阻断的出发不会补问）。
