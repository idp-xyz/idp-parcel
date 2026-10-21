# 129 派送触发不核期望入场依据，证据更正后自动另立第二项派送任务

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-dcg70（与 70 同批）。此前 ready-for-agent——2026-10-20 代决复核立（上线前必修），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M7（第一件）、M8；ADR-0216 越权风险点 2、7

## 现象与修法

现象、证据与修法的完整写法以复核报告 M7（第一件）、M8 一条为准（单一权威），此处只记要点：

执行器命令加可选「期望入场依据」，由交接触发传入本版交接按 ADR-0221 铸出的依据，链尾入场依据不是它时答「不是触发事实」；执行器查在册时沿参与替代链回看，前版参与已形成任务就答「已在册」并交回那一项。票 70 的派送待形成册按同一认法。

## 认领记录（2026-10-21）

in-progress——2026-10-21 并入票 [70](./70-delivery-dispatch-left-unformed-has-no-trace.md) 同一分支 `claude-dcg70` 同批做（协调方派单：与 70 改的是同一个派送触发执行器）。完成记录见文末。

## 完成记录（2026-10-21，作者子代理，分支 claude-dcg70）

in-progress——完工待评审与进 main，与票 70 同批同分支。基线 `640f67de`。裁定照复核报告 M7（第一件）与 M8 落，决定写进 [ADR-0230](../../../docs/adr/0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 决定六（与票 70 共一份 ADR，不另立）。

### 提交

- `276e81ea`：执行器期望交接、沿替代链认前版任务、真库用例（SHA 是变基到 main `c1828dd1` 之后的）。
- 文档随票 70：`a37f8270`（ADR-0230 决定六；ADR-0216 越权风险点 2 第一种情形与 7 的前向指针）、`a3fac644`（`AT-TF-289`、`290`）。

### 代决

1. **「期望入场依据」传的是那一版交接本身**（`TriggerDeliveryDispatchCommand.ExpectedHandover`），不传一串依据。认「这条参与是不是凭这一版交接进的段」只在 `FulfillmentParticipation.EnteredByHandover` 一处，旧拼法的依据要按版本与起点认，比串比不出来。
2. **新拒绝格 `ENTRY_NOT_THIS_HANDOVER`**，不并进既有各格：续办动作不同，要去看对象此刻凭哪一次交接进的段。处置照`不是触发事实`入账。
3. **核在查在册之前**：这次交接没带对象进这个段，不论段上有没有任务，这一拍都不是它的触发。
4. **只有随交接信封那一路给期望交接**；补拍与按册重问不给，问的是对象此刻的链尾。越权风险点见 ADR-0230 第 12 点：票 130 修好之前，待确认更正为`已交接`那一类仍可能经补拍形成任务。
5. **沿替代链回看**：链尾那一版的键（新铸法、旧拼法）查不到，再从链尾往前逐版查。前版已形成这个对象的派送任务就答`已在册`、交回那一项。
   - 失效版本跳过。
   - 前版键上立着别的任务不算在册，也不答引用被占。
   - 旧拼法串恰合新形状时不认，与链尾那一处同一条防线。
   - 铸法不改，已落库身份不追溯改写。越权风险点见 ADR-0230 第 13 点：跨过失效版本也认最初那一项。
6. **票 70 的派送待形成册按同一认法**：键（租户，段，对象），替代参与是同一条。

### 用例

- `internal/transportfulfillment/application/trigger_dispatch_supersession_test.go`：
  - `TestAHandoverThatDidNotBringTheObjectIntoTheSegmentIsNotItsTrigger`：待确认更正为`已交接`、对象仍挂在先前的派送段，答 `ENTRY_NOT_THIS_HANDOVER`、不问所有者、不形成任务；带对象进段的那一版照常形成。
  - `TestACorrectionOfTheHandoverThatBroughtTheObjectInDoesNotMintASecondTask` 三个子用例：前版已形成，更正版答已在册；前版信封晚到不是触发；补拍不带期望交接，也认前版。
  - 去掉修法时这两条用例都变红（实测）。
- ★ `cmd/parcel-dispatch/delivery_dispatch_supersession_test.go`：
  - `TestACorrectedHandoverEnvelopeFindsTheTaskThePredecessorFormed`：真更正、真替代参与，库里仍一项任务。
  - `TestALateEnvelopeOfASupersededHandoverDoesNotTriggerForTheCorrection`：前版晚到入账、不问所有者，更正版形成一项。
- 册那一半：`application/delivery_dispatch_pending_test.go::TestASupersedingParticipationIsTheSamePendingEntry`。

### 验证

随票 70 一起跑，见票 70 完成记录「验证」：65 个包，61 ok、0 FAIL。

### 没做与去向

- M7 第二件（待确认或拒收更正为`已交接`改走进段）：票 130。

## 评审记录（2026-10-21，隔离评审，无阻断项）

与票 70 一份评审，逐条去向见票 70「评审记录」。落在本票这一半的：

- **Status 行**改为「in-progress——完工待评审与进 main，分支 claude-dcg70（与 70 同批）」（`1b306b19`）。
- **票面**以 main 上那一份为底，只多认领、完成记录、评审记录三节；指向票 130 的链接变基后能连上。
- **并发窗口**（顺手第 6 条）：前版信封那一拍铸前版键的任务还没提交，更正版信封、补拍或重问那一拍读到链尾已是更正版，回看前版键看不见前一笔，于是铸第二项。
  - 改法：执行器先占（租户，段，对象）的事务级形成锁，再读段、查在册（`dade3c30`）。前一拍提交之后后一拍才查，查得到前版那一项，答`已在册`。
  - 用例：PG `TestHoldingAFormationSerialisesTwoBeatsOfTheSameEntry`；编排 `TestEveryBeatHoldsItsEntryBeforeAnswering`；parcel-api 真库并发首拍改证新语义。新 AT 是 `AT-TF-295`。
  - 手工建任务口不取这把锁，记进 ADR-0230 越权风险点 16。
- 本票代码在评审后无别的改动，「完成记录」的代决与用例仍有效。

