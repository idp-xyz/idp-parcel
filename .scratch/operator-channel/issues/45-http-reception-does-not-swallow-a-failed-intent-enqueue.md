# 45 HTTP 收寄路上，意图入队失败被吞成续办引用后照常提交：设备拿到 201 就出队，没人会重发

Category: bug
Status: ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立（票 26 序 2 研判中发现；用户授权代决）
Blocked by: operator-channel/29 进 main（同改 cmd/parcel-api/assemble_reception.go 与收寄编排）
父票：[26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)（票 26 评审遗留的后续）
出处：
- 簇 C1 终裁 C1-Q3 第六条；
- UC-NO-002 步骤 8 与 AT-NO-027；
- 票 sa-cc-funds-and-credential-seams/29 裁决 2。
地盘：
- cmd/parcel-api/assemble_reception.go
- cmd/parcel-api/assemble_reception_test.go（或新的真库用例文件）
- internal/nodeoperations/application/receive_delivered_unit.go（若要把交接错误随结果交出）
- internal/nodeoperations/application/receive_delivered_unit_test.go

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

## 现象（钉 main 8d22729f）

- 收寄编排的 handOff 把 Downstream.HandOffNodeIntake 报的错吞掉，换成意图续办（IntakeHandoffReference），结果照常交回；transactionalReception 随之提交。
- HTTP 收寄响应里没有这一格：writeOutcome 只写 ContinuationReference。设备按 ADR-0022 收到 2xx 就出队。于是收寄已提交、意图没入队、设备也已出队，不会有人按这条引用去重放。existingResult 会重发意图，但前提是有人重投同一来源。
- 入队一步若报 SQL 级错误，事务会中止，提交时得到 commit unexpectedly resulted in rollback，整笔回滚，答 500，零行（业务后果核验实测）。可见「结果已提交而意图没跟上」只在入队以 Go 层错误失败时出现。transactionalReception 头注里与此不符的那句，operator-channel/29 已改正。
- 前线导入路上，这一格有人读：报告明细写的是「意图未交出（…），重跑同一文件会重发同一份」。

## 裁定

照 sa-cc/29 裁决 2 的分法：续办引用有人读、有响应格的路，保留续办；没人读的路，不接受续办。

1. HTTP 设备路（transactionalReception）不接受意图未交出的结果。做法是交回 error、整笔回滚，端点答 500 NO_ANSWER_FORMED，设备留在队里重发；重发时收寄重新形成，意图与收寄同一笔入队。
   理由有三：
   - UC-NO-002 步骤 8：「原子提交收寄判断、控制、审计和对应发布意图｜投递失败只重试原发布意图」。
   - ADR-0049 记下了 ADR-0043 落到库里的形态：「发布意图与业务结果同一提交」。
   - AT-NO-027「收寄结果提交成功但事件投递失败」说的投递失败，指提交之后由 Outbox 中继重投承担的那一步，不是入队失败。
2. 前线导入路照旧。报告把续办写给内勤，ADR-0043 决定三照用：「首次交付失败不改写业务结果…另留一条发布续办引用」。
3. 编排吞错的写法不改，前线导入还要用它。
   - HTTP 路若需要分格，照 sa-cc/29 的做法，让编排把交接错误随结果一起交出，具体形状由作者定。信封被框架确定性拒收时要不要另成一格，作为判断项写明。
   - 若信封 ID 超限是因为设备签发的来源标识过长，应在 Intake 先答 4xx。依据 ADR-0023：来源标识由设备签发。

## 做什么

- transactionalReception：结果带意图续办时，交回 error。用新哨兵，还是包住交接错误，由作者定。同时改头注。
- 真库用例：
  - 注入一次入队 Go 层失败：答 500，零行；
  - 入队恢复后，同一来源重投：答 201，一行，意图一封。
- AT-NO-027 的现有用例 TestReplayConflictAndFailedIntentStayDisciplined 语义不变。若 AT 的落点有变，.scratch/at-coverage-inventory.md 随之改。

## 不做

- 不改 ADR-0043。
- 不碰其他上下文里同形的 HandoffReference，例如 customscompliance 的各编排。
- 不改前线导入的意图续办。

## 完成判据

- [ ] HTTP 路：意图入队失败时整笔回滚、答 500；重投后，收寄与意图在同一笔落地。前线导入路照旧。两条路各有回归用例。
- [ ] go build ./...、go vet ./... 退 0，gofmt -l . 无输出；带 DSN 跑 internal/nodeoperations/...、cmd/parcel-api，全绿。
- [ ] 走 Go 那条路（docs/agents/parallel-sessions.md）。
