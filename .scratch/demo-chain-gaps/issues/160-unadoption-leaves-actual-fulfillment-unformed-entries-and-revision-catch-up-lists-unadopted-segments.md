# 160 解除采用之后，实际履约未形成册上那一段的条目不作罢，换版补形成按委托列段也不分仍在采用与已解除

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-21 票 [88](./88-actual-fulfillment-not-formed-when-adoption-or-agreement-arrives-late.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0231](../../../docs/adr/0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md) 越权风险点 13。
Blocked by: 票 [57](./57-segment-commission-adoption-has-no-correction-and-start-does-not-rederive.md)（解除采用）

## 现象

- 票 88 立了实际履约未形成册（ADR-0231 决定六）与两封补形成门铃。写册的只有形成后半那一处；采用门铃的补形成先复核「此刻仍采用着」，没有采用连线就入账、不补，但不碰册。
- 今天采用不解除，这两格走不到。票 57 开了解除采用之后：
  - 一段解除采用时，册上那一段已有的未形成条目照留，查阅页仍把它们列成要换版去补的待办，而那一段已经不按那份委托采购了。
  - 换版门铃的补形成按委托读采用册全表列段（`SegmentCommissionAdoptions.ListByCommission`）。解除若另记一表或在采用行上加标记，列段会把已解除的段也列进来，对它们按那份委托再补形成。

- 采用门铃的信封 ID 已带这一次采用的登记时刻（票 88 评审）：解除之后同一段再采用同一份委托，第二封门铃不会被当成第一封的重发吞掉、两次采用之间结束的参与照样补形成。票 57 让采用册按代记之后，指纹里的登记时刻换成代次（`OutboxSegmentCommissionAdoptionHandoff.HandOffSegmentCommissionAdoption`）。

## 要做的

- 解除采用那一笔交一封门铃（或复用采用门铃的消费口另立一种类型），补形成看到段已不采用时把册上那一段的未形成条目作罢（册多一种状态，或复用了结的一格并记原因）。
- 按委托列段只列此刻仍在采用的那几段，与解除采用的记法同一处判。
- 三类：机制。

## 规模

S

## 记录（2026-10-21，票 57 变基时顺手做的一半，分支 claude-dcg57）

「按委托列段只列此刻仍在采用的那几段，与解除采用的记法同一处判」已做：`SegmentCommissionAdoptions.ListByCommission` 改按 `currentAdoptionCondition`（与 `FindBySegment` 同一个判法）只交当前那一代，解除编排重新派生开始也调它，票 57 原来那一口 `FindCurrentByCommission` 删去（真库用例 `TestListingByCommissionShowsOnlyTheCurrentAdoptions`）。

未做、仍归本票：解除采用时把实际履约未形成册上那一段的条目作罢（要多一种状态或一封门铃）；采用门铃信封指纹里的登记时刻换成代次（今天登记时刻已能分开解除前后两次采用，换不换不影响正确性）。另：票 57 让自动形成遇已失效链头答已存在、把未形成册上那一条了结为已形成（ADR-0232 决定五、越权风险点 16），与作罢不是一回事。
