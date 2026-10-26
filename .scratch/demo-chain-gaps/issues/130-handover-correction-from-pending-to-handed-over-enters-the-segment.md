# 130 交接更正由待确认或拒收改为已交接时，对象不结束先前段参与也不进新段

Category: bug
Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-dcg130` tip 为写完成记录的那一笔（代码与文档最后一笔见文末「完成记录」提交表）。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领（票 126 已进 main），分支 `claude-dcg130`（worktree `idp-parcel-dcg130`）。此前 ready-for-agent——2026-10-20 代决复核立（上线后尽快），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M7（第二件）；ADR-0216 越权风险点 2

## 现象与修法

现象、证据与修法的完整写法以复核报告 M7（第二件） 一条为准（单一权威），此处只记要点：

前版待确认或拒收、本版已交接的交接更正改走进段，先例是 label-channel-service-first-release/31 的分路。

## 文档自洽审计复核（2026-10-21）

审计条目：[audit/tf.md](../../docs-consistency/audit/tf.md) M17（归 C，审计已点名本票，不另立）。审计钉 `6b0074d0`；按 main `67438a0f` 复核，现象仍在：`application.rederiveFulfillmentParticipation` 在前版没有参与时仍答 `SegmentEntryRefusedNoParticipationToRederive`，对象不进新段；票 88、146、94 进 main 都没有碰这一路。

分级建议（文档自洽 02）：R2，与本票既定「上线后尽快」一致——由待确认改为已交接的更正不常见，对象仍挂在先前段里看得见。

文档侧由修订票 docs-consistency/08 做，本票不改文档：UC-TF-005 补一条 AT「更正为`已交接`即进段、结束先前段参与」，标「待票 130」；UC-TF-006 `AT-TF-289` 的括注写明那是本票之前的过渡态。本票落地时去掉这两处标注。

文档自洽 08（2026-10-21）已照上一段做：UC-TF-005 新立 `AT-TF-329`（待确认或拒收更正成`已交接`即进段、结束先前段的参与），「给开发的交接」记本票为它的现状缺口；UC-TF-006 `AT-TF-289` 括注写明那一种是本票之前的过渡态。本票落地时去掉这两处标注，`AT-TF-329` 补 Covers 点名。

## 完成记录（2026-10-26，Claude Code 作者子代理，协调方 `tops-20`，分支 `claude-dcg130`）

起点 origin/main `803b97e5f`（含认领；票 126 已在）。单一权威：复核报告 M7 第二件；落成规则见新 [ADR-0307](../../../docs/adr/0307-a-handover-corrected-from-pending-or-refused-to-handed-over-enters-the-declared-segment.md)。票面与报告不相抵。

### 提交

| SHA | 内容 |
|---|---|
| `6777200e9` | 代码：段声明留存（端口、编排、PostgreSQL 适配器、TF 迁移 `0053`）；更正的段那一半分路（`correctionSegmentHalf`）与首登共用 `handOverIntoSegment`；新格 `LATER_CONTROL_FACTS`；派送两处注释改成票 130 之后的说法；编排、派送一拍、真库段册与登记册、派发进程真库派送与全程追踪补派生各用例；`AT-TF-289` 用例改用信封晚到那一种 |
| `55b46286d` | 用例：派发进程真库下与实际履约发生项自动形成（ADR-0218）的交互 |
| `71818bd69` | 文档：ADR-0307（新）；ADR-0216、ADR-0230、ADR-0112 Status 注记与 Links；UC-TF-005、UC-TF-006；AT 盘点 |
| 本笔 | ADR-0307 两处措辞；本完成记录与 Status |

### 改了什么

- **端口**：`ports.TransportHandoverRecord` 加 `SegmentDeclaration`（`HandoverSegmentDeclaration`：段、计划段、段服务动作三格原串）。
- **编排**（`internal/transportfulfillment/application/`）：
  - `register_transport_handover.go`：首登取命令三格留存（空白读作没声明，带 NUL 答未受理）；首登的段那一半收成 `handOverIntoSegment`（先结束此刻在场的参与、再进声明的段），`endPreviousParticipation` 与 `establishSegment` 改按交接自身与留存的声明取值；更正那一版照前版承继声明，落库后交给 `correctionSegmentHalf`。
  - 新文件 `correct_handover_segment_half.go`：先问重派生（ADR-0112 不变）；本版`已交接`、前版没转出控制、重派生答无可替代时，先看对象各段参与链链尾有没有晚于这一版业务时间的入场或离场（`laterControlFacts`），有就答 `LATER_CONTROL_FACTS`、一笔不写，没有就走 `handOverIntoSegment`。
  - `enter_fulfillment_segment.go`：`SegmentEntryRefusal` 加 `SegmentEntryRefusedLaterControlFacts`（`LATER_CONTROL_FACTS`）。
  - 注释：`rederive_fulfillment_participation.go` 头注一句、`trigger_dispatch_on_handover.go` 与 `trigger_delivery_dispatch.go` 里「待确认更正为`已交接`而对象仍挂在先前的段里」改成票 130 之后的说法（取段理由也改：交接记录现在留存段声明，但声明不是进成了）。
- **PostgreSQL**：`transport_handover_registry.go` 三条读路与写路带三列；迁移 `migrations/transport_fulfillment/0053_transport_handover_segment_declaration.sql`（三列可空、不得空白；0052 归票 121）。
- **HTTP**：`tfhttp` 交接答复两处字段注释（`segmentEntryRefusal`、`participationEnd`），线格式不变。
- **cmd**：装配不动（更正与首登本就是同一个编排、同一份 `ParticipationEnds`）。`cmd/parcel-dispatch` 只加用例。
- **文档**：
  - ADR-0307（新，代 TF owner 决）。
  - ADR-0216 Status 索引注记加「由 ADR-0307 回答越权风险点 2 第二种情形；决定一理由『交接记录本身不带段号』与候选与反方『交接记录不存段号』所说的事实不再成立、取段的决定不变」，Links 加一行；正文不动。
  - ADR-0230 Status 索引注记加「越权风险点 12 由 ADR-0307 回答」，Links 加一行；正文不动。
  - ADR-0112 Status 索引注记加「由 ADR-0307 补一格，无条款停用」，Links 加一行；正文不动。
  - UC-TF-005：输入表加「进段声明」一行；「一致性、幂等与并发」加一条（待确认或已拒收更正成`已交接`即对象的这一次交接，晚于后续控制事实时不写）；`AT-TF-329` 去掉「现状缺口见『给开发的交接』」、写明段取首登声明与往回更正的走法；新立 `AT-TF-347`；「给开发的交接」删票 130 那一行。
  - UC-TF-006：`AT-TF-289` 场景括注改为「信封晚到，对象已被下一次交接带进别的段；或那一版是没能带对象进段的更正，见 UC-TF-005 `AT-TF-347`」（去掉过渡态标注）；「给开发的交接」删票 130 那一行。
  - TF CONTEXT、GLOSSARY、CONTEXT-MAP **未改**：「已交接使控制转入接收方」、生命周期「待确认 → 已交接或已拒收」与「再次进入本段只能经更正回`已交接`从失效版本长出替代版本」已是现行规则。
  - AT 盘点：`AT-TF-329` 由④移到①（点名用例见下），`AT-TF-347` 入①，文末追记一条；总览计数与 ADR 索引没重生成（`-check` 只报生成物过期）。

### 两种前版、往回更正与三路交互（派单要求想清楚的几件）

- **拒收→已交接 与 待确认→已交接完全同路**：两格都不转出控制、都不进段，在段与参与上留下的东西一样（都没有，或都只是一版失效参与）。分路不看前版是哪一格，只看「本版转出控制、前版没转出、无可重派生」（ADR-0307 决定一）。用例逐格跑两种前版，外加首登即为`已交接`作对照，三格答复逐格相同。
- **往回更正（已交接→待确认/拒收）照 ADR-0112 决定四，本票不改**：凭更正进了段的那一版再被撤回，在它进的那一段长失效参与版本；先前段被结束的那一条不还回去（CONTEXT「不回退到更早的控制版本」），对象此刻不在任何段里，NO 一侧记「控制来源链待重算」。再更正回`已交接`时重派生找得到失效链尾，从它长出替代版本、回到同一段，不另立参与、不再结束参与（用例 `TestAHandoverCorrectedBackAndForthWalksOneParticipationChain`；派送一侧同链答`已在册`）。
- **与派送触发（票 129 期望入场依据）**：更正版那一封核得过期望交接、形成任务；往回再往前那一封沿替代链认出先前那一项、答`已在册`；`ENTRY_NOT_THIS_HANDOVER` 只剩信封晚到与答了 `LATER_CONTROL_FACTS` 的那种更正。票 129 的规则与代码不动（`AT-TF-289` 用例改用信封晚到的场景，因为「待确认更正后仍挂在先前段」这一状态已不可达）。ADR-0230 越权风险点 12（补拍与重问替先前段形成任务）随之消失：对象已离开先前的段。
- **与 VE 补派生（票 136 按对象扫全段）**：更正版那一封是转出控制的那一版，补派生扫到新进的段，补出先于进段登记、声明了那一段的出发（派发进程真库用例）。VE 代码不动。
- **与实际履约自动形成（ADR-0218）**：先前段那一条由更正那一版结束，更正版那一封认出它，段采用了委托就形成发生项、业务时间取交接时刻（派发进程真库用例）。此前这一封答「没有参与被结束」，先前段这一程不计费。

### 代决与越权风险点

代决（详见 ADR-0307）：

1. **段从哪来：随交接版本留存首登的段声明、更正承继（迁移 0053），不让更正命令带段号。** 读了 ADR-0112 决定二、ADR-0213、ADR-0216 决定一与候选、交接领域 `Correct` 的承继口径、lc/31 判断命令；没读 ADR-0263 正文。owner 若要更正带段：`CorrectTransportHandoverCommand` 与 `HandoverCorrectionPayload` 加三键，要部分停用 ADR-0112 决定二一句；留存可留可撤。
2. **分路判据：先问重派生，无可替代且「本版转出、前版没转出」才走进段**，不照 lc/31 只看前版裁决——链尾失效那一种（已交接→待确认→已交接）前版也没转出控制，却要回原段。
3. **晚于后续控制事实时一笔不写、答新格 `LATER_CONTROL_FACTS`**（新加一个答格是产品判断，由 ADR-0307 决定三作出）。不加这道，更正会让对象两段同时在场（此后每次交接在结束参与那一步响亮失败）或离场后重新在场。失效链尾也算更晚（保守）。owner 若要就地重算，改 `laterControlFacts` 之后那一步。
4. **带 NUL 的段声明答未受理**：此前首登段号带 NUL 是照登、静默不进段，现在要落库，同节点声明存不下的答法（ADR-0213）。这是首登受理面的一处变化。
5. **需要新 ADR**：分路、留存与新答格都是难逆转的取舍，号 0307；对 ADR-0216、0230 是回答、对 ADR-0112 是补一格，都无条款停用，只在 Status 注记与 Links 挂指针。
6. **TF CONTEXT 不改**（见上）。

越权风险点（详见 ADR-0307「越权风险点」）：晚到的更正不重算；首登晚到同样落进两段同时在场（见下）；声明错了改不了；失效链尾也算更晚；前版已交接而没进过段的更正仍不进段（与票 179 欠账续办相连）；结束参与那一口成了更正的硬依赖；存量没有段声明；没读 ADR-0263 正文与 NO 认更正链的代码。

**首登晚到（不在本票修、不另立票）**：首登`已交接`晚到（业务时间早于对象此刻那条参与的起点）时，结束参与那一步答未受理、进段照走，对象两段同时在场，此后它的交接都在结束参与那一步以 `ErrObjectActiveInSeveralSegments` 响亮失败，离线补登就会发生。协调方 2026-10-26 按用户收窄范围的要求定为不立票，留在 ADR-0307 越权风险点 2：修法建议是首登在 `handOverIntoSegment` 之前同样先问 `laterControlFacts`、答同一格。

### 修法 / 完成判据 → 用例

| 修法（M7 第二件、票面） | 用例或落点 |
|---|---|
| 前版待确认或拒收、本版`已交接`改走进段：结束先前段参与、进新段（`AT-TF-329`） | `application/handover_correction_enters_the_segment_test.go::TestAPendingOrRefusedHandoverCorrectedToHandedOverEntersTheDeclaredSegment`（两种前版 + 首登对照）、`::TestACorrectionIntoTheSegmentAnswersLikeAFirstRegistration`（没声明段、此前不在段里、重放不再进段）；真库 `adapters/postgres/handover_correction_enters_the_segment_db_test.go::TestAPendingHandoverCorrectedToHandedOverEntersTheDeclaredSegmentInTheDatabase` |
| 段从首登声明来、更正承继 | `application/...::TestAHandoverKeepsItsSegmentDeclarationAndACorrectionInheritsIt`；真库 `adapters/postgres/transport_handover_segment_declaration_test.go` 两例（三条读路往返、空白被 CHECK 拒） |
| 往回更正（反向）今天怎么走 | `application/...::TestAHandoverCorrectedBackAndForthWalksOneParticipationChain` |
| 晚于后续控制事实（`AT-TF-347`） | `application/...::TestACorrectionIntoTheSegmentAfterLaterControlFactsWritesNoSegment`（下一次交接、已离场、同一刻不算）；真库 `...db_test.go::TestACorrectionIntoTheSegmentAfterAnOnwardHandoverWritesNoSegmentInTheDatabase` |
| 结束参与口没接线 | `application/...::TestACorrectionIntoTheSegmentWithoutAParticipationEnderFormsNoAnswer` |
| 与派送触发（票 129）的交互 | `application/trigger_dispatch_supersession_test.go::TestAHandoverCorrectedFromPendingIntoADeliverySegmentTriggersOnTheCorrection`；派发进程真库 `cmd/parcel-dispatch/delivery_dispatch_supersession_test.go::TestAPendingHandoverCorrectedIntoTheDeliverySegmentFormsTheTaskOnTheCorrection`；`AT-TF-289` 用例改为信封晚到 `::TestAHandoverThatDidNotBringTheObjectIntoTheSegmentIsNotItsTrigger` |
| 与 VE 补派生（票 136）的交互 | 派发进程真库 `cmd/parcel-dispatch/movement_fact_backfill_test.go::TestAPendingHandoverCorrectedIntoTheSegmentBackfillsTheDepartureRecordedBeforeIt` |
| 与出发门禁补问（票 121，ADR-0305）的交互 | 派发进程真库 `cmd/parcel-dispatch/departure_gate_reask_handover_correction_test.go::TestAPendingHandoverCorrectedIntoTheSegmentReasksTheStoppedDeparture`（待确认那一封入账不补问；更正版那一封补问与补派生两路各入账一次、不报错，补问追加第 1 次、读数阻断，首版不改） |
| 与实际履约自动形成（ADR-0218）的交互 | 派发进程真库 `cmd/parcel-dispatch/actual_fulfillment_handover_correction_test.go::TestAPendingHandoverCorrectedToHandedOverAtTheEndOfAnAdoptedSegmentFormsTheOccurrence` |
| 去掉 `AT-TF-329`、`AT-TF-289` 的标注，`AT-TF-329` 补 Covers，盘点改桶 | UC-TF-005、UC-TF-006、AT 盘点（`71818bd69`） |

提交前变异抽验：让更正的段那一半只走重派生（退回票 130 之前），「修法 / 完成判据 → 用例」表里编排、真库、派发进程各新用例全红，其余用例不受影响；去掉 `laterControlFacts` 那道，`AT-TF-347` 的编排与真库用例转红。

### 自验（含 PG）

- `go build ./...`、`go vet ./...` 退 0；`gofmt -l cmd internal migrations` 空。
- 带 DSN（`127.0.0.1:55432`）`go test -p 1 -count=1`：`./internal/transportfulfillment/...`、`go list` 反查出依赖 TF 的 `./cmd/parcel-api/...`、`./cmd/parcel-dispatch/...`、`./cmd/parcel-transport-register/...`，VE 的 TF 适配器 `./internal/visibilityexception/adapters/transportfulfillment/...`，`./internal/architecture/...`、`./migrations/...`、`./internal/platform/migrate/...`：19 包 ok、0 FAIL、1 包无测试（`transportfulfillment/ports`）。钉在代码笔 `55b46286d` 的树上跑（之后只有 `.md`）。
- 新真库用例逐个 `-v` 确认 PASS 非 SKIP：TF postgres 四例（`TestAPendingHandoverCorrectedToHandedOverEntersTheDeclaredSegmentInTheDatabase`、`TestACorrectionIntoTheSegmentAfterAnOnwardHandoverWritesNoSegmentInTheDatabase`、`TestAHandoverSegmentDeclarationRoundTripsThroughEveryReadPath`、`TestABlankSegmentDeclarationIsRefusedByTheDatabase`），派发进程三例（派送、全程追踪补派生、实际履约发生项）。
- `go run ./tools/adr-index -check` 与 `go run ./tools/at-coverage-counts -check` 都只报生成物过期（预期，推送方重生成）；试跑 ADR 索引生成器确认 ADR-0307 标〔代决·待 TF owner 复核〕，已复原。
- 全量 `scripts/test-full.sh` 未跑（推送方的事）。

### 给评审与进 main 的提示

- **与票 121 的文本重叠已在变基时合好**：AT 盘点①两边都留、`AT-TF-342`–`347` 按号顺接，文末追记 121 在前、130 在后；UC-TF-005 自动合并，`AT-TF-347` 留在 `AT-TF-329` 下面（它的场景写的是「同上」）。迁移号（121 是 TF `0052`、本票 `0053`）与 ADR 号（121 是 0305、本票 0307）不撞。
- **与票 121 的行为交互**：121 已先进 main，本票已在其后补了交互用例（见上表）。
- 评审重点：`correctionSegmentHalf`（`correct_handover_segment_half.go`）的分路条件（先重派生、再三件判据）；`laterControlFacts` 看链尾的口径（含失效、同一刻不算）；首登受理面因 NUL 多出的未受理；`TestAHandoverThatDidNotBringTheObjectIntoTheSegmentIsNotItsTrigger` 改了场景（`AT-TF-289` 的内容未变窄：信封晚到那一种仍钉住）。

### 补记：变基到 121 之后（2026-10-26，协调方派的一小步）

- `git rebase origin/main`（`962c3d8e6`，含票 121）：冲突只在 AT 盘点（①相邻插行、文末追记），两边都留、按号顺接；其余自动合并。变基后本票各笔 SHA 换了，以分支上的为准（代码笔 `baa04b229`、用例笔 `047f17e46`、文档笔 `612081666`、完成记录 `df6129859`）。
- 补交互用例 `cmd/parcel-dispatch/departure_gate_reask_handover_correction_test.go`；`movement_fact_backfill_test.go` 把待确认首登与更正两步抽成两个辅助函数，两条用例共用。变异抽验：让更正只走重派生，新用例转红。
- ADR-0307：越权风险点 2 改为不另立票、写明复用 `laterControlFacts` 的修法；决定五与索引注记补上出发门禁补问一路，Links 加 ADR-0305。AT 盘点 `AT-TF-329` 一行补点名新用例。
- 自验（含 PG）见本节之后的提交说明与汇报：`go build ./...`、`go vet ./...`、`at-coverage-counts -check`，带 DSN 的 TF 全部包、`cmd/parcel-dispatch`、`migrations`。
