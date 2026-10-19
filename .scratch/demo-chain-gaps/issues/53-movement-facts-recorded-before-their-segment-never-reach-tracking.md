# 53 先于段登记的移动事实永远进不了全程追踪

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [33](./33-movement-fact-recorded-envelope-and-tracking-projection.md) 的完成记录（代决与越权风险点）。
Blocked by: 无
出处：[ADR-0205](../../../docs/adr/0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 决定四（段不在册照记）与文末补记越权风险点 14。

## 现象

移动事实可以先于它声明的段登记（ADR-0205 决定四），TF 照记。VE 消费移动事实信封时按键回读、沿声明的段取那一刻在控的对象；段此刻不在册就入账不派生——等不来，派发的失败预算也撑不到段登记那一天。段日后登了，没有任何东西回头让这封信重投，于是这条出发（以及它读数里的阻断）永远不进投影、不形成信号。

离线补登、交接口与作业事实口各走各的设备时，这种先后颠倒在一线是常态，不是偶发。

## 证据（读码）

- `internal/visibilityexception/adapters/transportfulfillment/derive_on_movement_fact.go` 的 `objectsOf`：段不在册交回空，入账不派生。
- `internal/transportfulfillment/application/record_movement_fact.go`：信封只在登记那一笔交出一次；同内容重投会重交，但没有人会为「段登了」去重投一条已在册的事实。

## 三类

机制。规模 M。

## 最小的诚实修法

两条路，二选一由 TF / VE owner 裁：

1. TF 在段首次成立（`EstablishSegmentWith*` 落册）那一笔里，按段查「声明了这一段、发生时刻落在段内某个参与区间里」的在册移动事实，逐条重交它们的已形成信封（ID 不变，入队那一步吞掉已交过的；已交过而 VE 那边入账了的，要换一个「段已成立」的唤醒信封，否则重交的同 ID 被吞）。
2. VE 订阅段成立（或参与进入）信封，回头按段查在册移动事实再派生。要 TF 给一个按段列移动事实的读口。

第二条把「谁来补」放在消费方，与 ADR-0066 消费侧展开同向；第一条要 TF 知道 VE 的需要。

## 要裁的

- 走哪一条；补投时对象仍按事实发生那一刻在控取（与 ADR-0205 决定四同口径）。
- 读数是登记那一刻问的（段不在册答未决），补投不重问关务；要不要连带补问，归补问机制（ADR-0205 越权风险点 2）。

## 既有票

none
