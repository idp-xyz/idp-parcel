# 46 节点作业的承接与集运编排，自己的册读写失败时答 2xx`未决`、什么也没落：与收寄同形，一旦开了设备渠道，设备出队即丢

Category: bug
Status: in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（分支 `claude-oc46`）。此前 ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立并代决（票 26 序 2 研判中发现）
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

## 完成记录（2026-10-19，会话 idp-parcel-9b 的实施子代理，分支 claude-oc46）

**落点**（基线 `ccbab34f`）：

- `898575c8` 承接与执行事实编排：承接册、执行事实册读写失败，已存摘要形状认不出，并发读回失败或读不回，一律交回 error。`COLLABORATION_UNDECIDED` 结果格、它的两个原因与续办引用随之删去（ADR-0186 决定五：续办引用只给册上的记录）。
- `2f6c0ef5` 集运编排：单元库、包含索引、来源事实登记读写失败，已存摘要形状认不出（受理闸与并发赢家两处），并发读回失败或读不回，已有结果读回单元失败，领域拒后复查登记读不动，一律交回 error。`UNDECIDED` 结果格删去。前线导入去掉那一支：编排交回 error 时本行整笔回滚，记 `NO_ANSWER_FORMED`、未决，退出码为未决。
- `42461d0b` 两只构造器改交 `(handler, error)`，缺任一依赖即拒。今天缺依赖在运行期是空指针，不是冒充依赖故障；照 ADR-0186 一并收在构造期。
- 方向依据读不动没有动：它仍是依据记录里的一格取值，由 `DirectionalBasisJudge` 判，照样落册（no-directional-basis/01）。

**各格对应用例**：

- 承接口：`TestCollaborationOwnStoreFailuresFormNoAnswer`。每格断言交回 error、结果格为零、不落册、不交意图。
- 集运口：`TestConsolidationOwnStoreFailuresFormNoAnswer`。断言同上；断单元库读不动那一格时，移入、移出、封装、开封、关闭五口逐一走到。
- 前线导入真库：`TestFrontlineConsolidationRowFormsNoAnswerWhenItsOwnRegisterFails`。移入行在单元写下之后来源事实写失败：断言本行未决、`NO_ANSWER_FORMED`，单元里没有成员，登记里没有事实；册恢复后重跑同一行答 `MEMBER_ADDED` 并留下事实。
- 装配：`TestCollaborationAssemblyRefusesMissingDependencies` 与 `TestConsolidationAssemblyRefusesMissingDependencies`。
- 越出地盘、改到最小的两处：`unversioned_payload_digest_test.go` 那一例原先断「认不出的形状答存储未决」，改名并改断交回 error；`judge_directional_basis_test.go` 两处装配改走 `mustConsolidateHandler`。

**先红后绿**：

- 承接口用例全部子例在基线代码上红：答 `COLLABORATION_UNDECIDED`，error 为 nil。`898575c8` 起绿。
- 集运口用例全部子例在 `898575c8` 上红：复查那一格答 `NOT_ACCEPTED`，其余答 `UNDECIDED`，error 都为 nil。`2f6c0ef5` 起绿。
- 真库用例在 `898575c8` 上红：本行答 `UNDECIDED` 而不是 `NO_ANSWER_FORMED`。临时放开该断言再跑，实测单元里留下成员 `SYN-HU-1`，登记里没有它的来源事实。这说明旧答法下事务照常提交、留下半截，重跑会被读成「成员已在本单元」的重放。`2f6c0ef5` 起绿。
- 装配用例全部子例在构造器只改签名时红（缺件仍装配成功），`42461d0b` 起绿。

**复现不了或没做的格**：

- 票面现象都已复现；真库上另量到上面那条半截落册，比票面所记的「什么也没落」更重。
- 「领域拒后复查登记读不动」票面没列，用例量到它答 `NOT_ACCEPTED`，一并改为交回 error：复查读不动时，分不出这是本就做不得，还是重放跑赢了受理闸。
- HTTP 5xx `NO_ANSWER_FORMED` 一格无从实测：两只编排今天都没有 HTTP 端点，`AcceptCollaborationHandler` 也没有生产调用方。
- 封装口的快照意图交在来源事实写之前。来源事实写失败时，意图随本行事务回滚，前线导入那一路成立；替身用例没覆盖这一格。
- 没给 ADR-0186 追加复核记录来扩大它的适用面：ADR 不在本票地盘，留会话定。

**门**：

- 改动的 `.go` 跑 `gofmt -l`，无输出。
- `go build ./...`、`go vet ./...` 退 0。
- `internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-frontline-import`、`./internal/architecture/...` 用 `-p 1 -count=1` 真库全 ok。
- 机制清点无差。

**证据层级**：S（隔离合成，`SYN-` 值）。

评审：待会话读 diff 审。
