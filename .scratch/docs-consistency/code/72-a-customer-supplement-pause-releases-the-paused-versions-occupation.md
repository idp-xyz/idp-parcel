# 72 停在等待受控补充的那一轮也释放本版本已成立的占用

Category: bug
Status: ready-for-agent——R2（上线后尽快）：Decide 判出资料不足、停在`等待受控补充`时，本版本已成立的冻结或额度占用一直占着，直到客户补件（code/20 在取代时放）或撤回。客户不补件也不撤回时，那一笔就一直占下去。协调方已裁定照下文做（见「要裁的」）。2026-10-21 协调方据 code/20 隔离评审与越权风险点 2 立。
Blocked by: 无
出处：code/20 完成记录越权风险点 2；code/20 隔离评审。

## 现象

`FormAcceptanceDecisionHandler.pauseForExternalResume` 把等待态写进聚合之后交回 `CustomerSupplementPending`，不发释放。只有授权处置选`交客户补充`那一路会在处置时释放（[ADR-0132](../../../docs/adr/0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md) 决定一）；Decide 自己停进`等待受控补充`的那一路不放。

## 证据

- ADR-0132 决定一：选`交客户补充`那一刻，这一份提交版本的接受已确定不成立——「它不会再被判，占着客户的资金或额度等一份不会到来的接受没有依据」。
- [ADR-0106](../../../docs/adr/0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md) 决定三：`等待受控补充`只由新提交版本的信封续办，停住的那一版不会再被判。
- PS CONTEXT 词条「控制项结果」：接受确定未成立时必须按原业务关联请求释放。

## 三类

机制（把 ADR-0132 的理由扩到它原本没覆盖的一条路）。无租户取值。

## 规模

S：`pauseForExternalResume` 在等待态是`等待受控补充`、且暂停已落库之后，按 `OccupationFormed()` 释放本版本；补用例。

## 最小的诚实修法

1. 暂停 `Save` 成功之后、交回 `CustomerSupplementPending` 之前，本版本 `OccupationFormed()` 为真即按原关联释放。
2. 释放失败照 `ControlReleasePending` 留补偿续办引用。暂停不撤销：等待态照旧随本份投递入账。
3. 客户补件之后，接受链的 `releaseSuperseded`（code/20）对同一笔再请求一次，提供方按幂等答原释放。
4. PS CONTEXT 生命周期「判断遇到客户可补充缺口 → 等待受控补充」补一句：停住的那一版已成立的占用随暂停释放，并引 ADR-0132 决定一。
5. 补编排与真库用例：资料不足 → 已冻结 → 停等补充 → 冻结已释放；补件后新版本照常冻结。

## 要裁的

已裁（协调方 2026-10-21）：照上面做，不另写 ADR。理由是 ADR-0132 决定一「它不会再被判，占着……没有依据」，加上 ADR-0106 决定三——那一版同样不会再被判。落地时完成记录的越权风险点写「暂停即放是把 ADR-0132 的理由扩到 Decide 停等补充这一路，待 PS owner 复核」。
