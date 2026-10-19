# 45 HTTP 收寄路上，意图入队失败被吞成续办引用后照常提交：设备拿到 201 就出队，没人会重发

Category: bug
Status: resolved——2026-10-19 进 main（45、no-reception-identity/03、32 同批，代码验证钉 `77ffe641`），见「进 main 记录」。此前 in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（operator-channel/29 已进 main），分支 `claude-no4503`（与 no-reception-identity/03 同一分支，先 45 后 03）。此前 ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立（票 26 序 2 研判中发现；用户授权代决）
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

- [x] HTTP 路：意图入队失败时整笔回滚、答 500；重投后，收寄与意图在同一笔落地。前线导入路照旧。两条路各有回归用例。
- [x] go build ./...、go vet ./... 退 0，gofmt -l . 无输出；带 DSN 跑 internal/nodeoperations/...、cmd/parcel-api，全绿。
- [x] 走 Go 那条路（docs/agents/parallel-sessions.md）。作者半边（分支、作者自验）已做，评审与推送方全量未到。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 claude-no4503）

**落点**

- 编排 `internal/nodeoperations/application/receive_delivered_unit.go`：`handOff` 交回续办引用与那次入队的错；`ReceiveDeliveredUnitResult` 加读口 `IntakeHandoffError()`，与 `IntakeHandoffReference()` 同在场同缺席。吞错写法与续办引用不变，前线导入照旧只读引用（裁定三）。
- 装配 `cmd/parcel-api/assemble_reception.go`：`transactionalReception` 见 `IntakeHandoffError()` 非空即交回 `parcel-api: reception intent not handed off: %w`，整笔回滚，端点照既有映射答 500 `NO_ANSWER_FORMED`。头注改写：这条路为什么不接受续办，确定性拒收为什么不另成一格。`buildReceptionOrchestration` 拆出 `buildReceptionDeps` 与 `receptionWithin`，行为不变，为的是真库用例只换一只依赖、其余照生产装配。
- 收寄口 `internal/nodeoperations/adapters/http/reception_payload.go` 的 `receptionCommand`：来源标识超出 `eventing.MaxSubjectLength` 字节或带控制字符即包 `ErrMalformedRequest`，答 400（判断项二）。

**作者自定的判断项**

1. **错误形状。** 编排侧在结果上加读口，不另加返回值：`Handle` 签名、`nodeopshttp.ReceptionHandler` 接口与前线导入的调用点都不用动，形状同 sa-cc/29 的 `HandoffError()`。装配侧不另立哨兵，只用 `%w` 包住交接错误。端点上它与其他「没形成答案」同答 500 `NO_ANSWER_FORMED`，没有读者要按它分流；包住则保留了真实原因，`errors.Is` 认得出注入的那只错，真库用例钉了这一点。
2. **信封被框架确定性拒收，不另成一格；随设备而变的那一维前移到 Intake 答 4xx。** 先量了：NO 收寄意图的信封 ID 已是指纹形（`outboxintent.FingerprintEventID`，定长），不会因来源标识过长而超限，所以票面「信封 ID 超限」那句条件在 ID 上不成立。但 `OutboxNodeIntakeHandoff` 的 `Subject` 原样取来源标识，超过 512 字节或带控制字符，同样被 `Envelope.Validate` 确定性拒收。这一格本票之前被吞成续办、静默答 201；本票之后若不拦，就成了设备对着 500 永远重发。所以照裁定三末条的依据（ADR-0023：来源标识由设备签发）在 Intake 答 400。信封上其余各维：Scope 取认证结果的租户，ID 与分区键定长，载荷只有两个键。剩下能让信封被拒的只有服务端缺陷：答 500，设备留着这次接收等修好；答 4xx 则会让设备把一次如实记下的接收出队交人，错却不在它。
3. **Intake 上限直接取 `eventing.MaxSubjectLength`，不在本地另抄一个数。** 框架改上限时它跟着走。收寄口知道「意图交付把来源标识放进 Subject」，这件事在两处头注里互相点名；`TestTheWiredReceptionHandsOffTheIntentForTheLongestSourceIDTheIntakeAccepts` 守两边是同一个上限，意图交付若改了 Subject 的取法，这一格先变 500。

**各格对应用例**

| 格 | 用例 |
|---|---|
| HTTP 路入队 Go 层失败：答 500 `NO_ANSWER_FORMED`、不带 outcome，零行零封；编排交回的错包着入队的错；恢复后同一来源重投答 201、带关联，一行一封 | `cmd/parcel-api/reception_intent_handoff_test.go::TestTheWiredReceptionRollsBackWhenItsIntentIsNotHandedOff` |
| 前线导入路照旧：同样失败时本行落地、INTAKE_FORMED、退出码不变、明细带续办引用；收寄已提交、零封；恢复后重跑答重放、明细不再提、一封 | `cmd/parcel-frontline-import/intake_handoff_continuation_test.go::TestFrontlineImportKeepsTheContinuationWhenTheIntentIsNotHandedOff` |
| Intake 上限：恰 512 字节照收、逐字进命令；513 字节、171 个三字节汉字、带 Tab 都答坏报文 | `internal/nodeoperations/adapters/http/work_fact_intake_test.go::TestWorkFactReceptionIntakeBoundsTheSourceIDByTheIntentEnvelope` |
| 上限上的来源标识在生产装配上照常交出意图 | `cmd/parcel-api/reception_intent_handoff_test.go::TestTheWiredReceptionHandsOffTheIntentForTheLongestSourceIDTheIntakeAccepts` |
| AT-NO-027 既有用例语义不变 | `TestReplayConflictAndFailedIntentStayDisciplined` 只加两条断言：首投时 `IntakeHandoffError` 包着下游的错；重发成功后续办两格都空 |

**先红后绿**

- 红：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/no45-red.log`。红跑之前先拆出 `buildReceptionDeps` / `receptionWithin`（无行为改动），用例才编得过。现象原话：HTTP 用例「意图没交出去 201 {"outcome":"INTAKE_FORMED",…,"serviceResultCheck":"CHECKED"}（收寄 1 行、意图 0 封）, want 500 NO_ANSWER_FORMED 且不带 outcome」，即票面现象：收寄已提交、意图没入队、设备拿到 201。Intake 用例三个子格「err = <nil>, want ErrMalformedRequest」。前线导入用例与上限真库用例修前即绿，是守护格。
- 绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/no45-green.log`。

**门**（钉代码笔 `886ceff3`）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN `go test -p 1 -count=1 ./internal/nodeoperations/... ./cmd/parcel-api/... ./cmd/parcel-frontline-import/... ./internal/architecture/...` 全 ok。

**证据层级**：`S`（隔离合成夹具，`SYN-` 值）。

**AT 落点**：不变。新用例不点 AT 号——裁定一写明 AT-NO-027 的投递失败指提交之后由 Outbox 中继重投的那一步，入队失败不是它。`.scratch/at-coverage-inventory.md` 未改。

**越出地盘**：`internal/nodeoperations/adapters/http/reception_payload.go` 与同目录 `work_fact_intake_test.go`（Intake 上限，判断项二）；`cmd/parcel-frontline-import/intake_handoff_continuation_test.go`（新文件，判据要求的导入路回归用例）。

**没做**

- 前线导入的来源标识（`FTI/…/批次/行事实号`）同样原样进 Subject：超过 512 字节时信封被确定性拒收，报告却写「重跑同一文件会重发同一份」，而重跑仍会失败。票面「前线导入路照旧」，没动，记此一句。
- 「走 Go 那条路」的评审与推送方全量未到。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `32b14c73`）

两轴合审，0 阻断。

- **Spec**：裁定三条落到。编排吞错的写法不变，只把入队错随结果交出（`IntakeHandoffError`，与续办引用同在场同缺席，形状同 sa-cc/29 的 `HandoffError()`）；`transactionalReception` 见错整笔回滚、答 500 `NO_ANSWER_FORMED`，入队恢复后同一来源重投 201、一行、一封意图；前线导入路照旧留续办（守护用例）。重放路 `existingResult` 的补发同样受这道门：补发失败也回滚答 5xx，设备留队重发。
- **作者自定的两格认可**：不另起哨兵，以 `%w` 包住真因；信封确定性拒收不另成一格——NO 信封 ID 是定长指纹，随设备而变的只有 Subject 里的来源标识，接入时按 `eventing.MaxSubjectLength` 与控制字符拒成 400（ADR-0023 来源标识由设备签发），上限与信封同源，真库用例钉了最长可受理的那一个照常交出。
- **Standards**：头注重写只写为什么；装配拆成 `buildReceptionDeps` / `receptionWithin`，理由是用例只换一只依赖、不另抄装配。
- **非阻断（另立票）**：前线导入铸出的来源标识没有同一道门，超长时报告说「重跑会重发」而重跑永远交不出去 → operator-channel/54。

### 进 main 记录（2026-10-19，会话 idp-parcel-9b）

- operator-channel/45 与 no-reception-identity/03（分支 `claude-no4503`）、operator-channel/32（分支 `claude-oc32`，暂停前基于 `b58b10ef`，续做时 rebase 过）叠成一串，rebase 到远端 main 之上快进推送。期间远端进了 SELL 客户费用链（另一会话，改 `cmd/parcel-dispatch`、SA、PP 与演示种子），先在旧基上起的那次全量作废，rebase 后重跑。机制清点在叠好的树上重生成（cmd 测试文件 154）。
- 推送方全量一次：钉 `77ffe641` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 144 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。验证之后远端只前进了一笔纯票面认领，再 rebase 一次，未重跑。
