# 46 节点作业的承接与集运编排，自己的册读写失败时答 2xx`未决`、什么也没落：与收寄同形，一旦开了设备渠道，设备出队即丢

Category: bug
Status: ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立并代决（票 26 序 2 研判中发现）
Blocked by: 无
父票：[26](./26-non-author-review-backlog-for-claude-code-landings.md)
出处：研判簇 C1 派生草案（承接与集运本方册读写失败）
地盘：
- internal/nodeoperations/application/accept_collaboration.go
- internal/nodeoperations/application/accept_collaboration_test.go
- internal/nodeoperations/application/consolidate_parcels.go
- internal/nodeoperations/application/consolidate_parcels_test.go
- cmd/parcel-frontline-import/consolidation_import.go
- cmd/parcel-frontline-import/consolidation_vertical_test.go

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 裁定（2026-10-18，会话 idp-parcel-9b 代决）

本方册读写失败交回 error（HTTP 5xx NO_ANSWER_FORMED / 前线导入记未决），依据 ADR-0022「返回 error 就是没形成」与 ADR-0186 决定三；跨上下文依据读不动按 no-directional-basis/01 的依据格记「读不动」。前线导入集运行的去向随之核对。no-directional-basis/01 偏离标记半边与 operator-channel/29 都已进 main，本票不再被挡。

## 现象（钉 main 8d22729f）

- internal/nodeoperations/application 的 AcceptCollaborationHandler：承接与执行事实两条路上，自己的册读写失败时答`未决`（走 acceptanceStoreUndecided、factStoreUndecided），error 为 nil，什么也没落。
- 同包的 ConsolidateParcelsHandler：Store.Save、Store.Update 或读回失败时，答 ConsolidationUndecided，error 为 nil，什么也没落。
- 这与草案 C1-T1 修之前的收寄同形。一旦有了设备渠道，ADR-0022 让设备在 2xx 时出队，这条已经发生的作业事实就在服务端没有记录。
- 为什么今天潜伏：AcceptCollaborationHandler 没有生产调用方；ConsolidateParcelsHandler 只在前线导入里构造，它的 ConsolidationUndecided 记作未决、重跑续办，不丢事实。

## 要裁的问题

1. 这两只编排的`未决`，在各自的用例里算不算「形成了的答案」？要读 UC-NO-003 与协作承接用例的结果语义契约。跨上下文依据读不动，和本方册读写失败，要不要分开处理？
2. 建议方向：
   - 本方册读写失败，交回 error。依据是 ADR-0022「返回 error 就是没形成」，以及新 NO ADR 的第三条。
   - 跨上下文依据读不动，按 no-directional-basis/01 依据记录的格值记。
   - 前线导入集运行的去向，随之核对。

## 做什么

- 把裁定写进票面；需要的落 ADR，或在草案 C1-T1 那份 ADR 后追加一条复核记录，扩大适用面。
- 改两只编排、前线导入集运行的去向，以及各自的测试。

## 不做

- 不改收寄编排，那是草案 C1-T1 的事。
- 不改依据记录的设计，那是 no-directional-basis/01 的事。

## 完成判据

- [ ] 要裁的问题已裁，写进票面；难逆转的已进 ADR。
- [ ] 本方册读写失败的每一种情形都有回归用例：交回 error，零写入；前线导入记未决。
- [ ] Go 门禁全过，并走 Go 那条路（docs/agents/parallel-sessions.md）。
