# 27 承运商首次有效收寄判断：失效命令带引用、不带种类时，原样重放答 `INPUT_NOT_ACCEPTED` 而不是 `ALREADY_RECORDED`

Category: bug
Status: in-progress——2026-10-16 完工，待进 main；见「完成记录」。此前 in-progress——2026-10-16 作者会话（票 20 的代码作者，Claude Code 会话）认领，分支 `claude-oc27`，基线 `d3caf1b0`；评审照改裁走非作者通道或 `/code-review` 的隔离子代理。此前 ready-for-agent——2026-10-16 会话 `idp-parcel-9b` 立（票 26 序 3 评审阻断；评审会话在 main `4dd937eb` 上复现）
Blocked by: 无
父票：[26](./26-non-author-review-backlog-for-claude-code-landings.md)（评审欠账；本票是它点名的「有阻断的，各立修复票」）。被评的是 [20](./20-tf-closure-dispatch-task-and-pickup-judgment-canonicalization.md)，代码 `857453d8`。
出处：票 20 的 Comments「评审 ← 会话 idp-parcel-9b 起的隔离子代理」那一条，Spec 轴阻断 1。

## 现象

在 `/transport-fulfillment-carrier-first-effective-pickup-judgments` 这一口上，复现步骤：

1. 先提交一份正常的收寄扫描判断，即包内测试常量 `scanPickupBody`，答 201。
2. 再提交一份失效判断：`evidenceVersion:"v2"`、`correctsSourceVersion:"v1"`、`expressesControl:false`，带着 `carrierReference`，但删掉 `carrierKind`。首判答 201 `PICKUP_VOIDED`，正确。
3. 把第 2 步的载荷原样再提交一次：答 200 `INPUT_NOT_ACCEPTED`。

评审会话（`idp-parcel-9b`）在 main `4dd937eb` 的隔离检出上，用包内夹具 `newCarrierPickupHTTPFixture` 实跑过，结果同上。

线上可达：生产入口 `OperatorDecisionIntake.IntakeCarrierPickupJudgment` 同样调 `CarrierPickupJudgmentPayload.Command`；`60244fa6` 起这一口挂在操作者真渠道上，持有这一种运营决定授予的操作者就能踩到。票 20 完成记录里「这一格只在编排直调时出现」的前提不成立。

## 应当

票 20 的裁定原句：同串重放「答 `ALREADY_RECORDED` 带回链尾（不变）」；「读法为不表达时只有读法……命令里给了也不构成内容」。第 3 步应答 200 `ALREADY_RECORDED` 并带回链尾。

## 根因（子代理评审给出，评审会话读过代码）

- `internal/transportfulfillment/application/judge_carrier_first_effective_pickup.go` 的 `replayOrConflictOnTheSameBasis` 不论读法，只要命令带了引用就先调 `NewCarrierSubject`；没有种类时构造失败，答未受理。
- 首判走 `rederive` 到 `Void`，根本不看主体，所以首判过得去、重放过不去。
- 让这一格从 HTTP 可达的是 `CarrierPickupJudgmentPayload.Command`：引用无条件写进命令，种类只在 `carrierKind` 非空时才写。

## 修复方向（供参考，由实施会话定）

只在命令表达控制（`ExpressesControl` 为真）时才推导或校验命令一侧的主体。读法为不表达时，同依据重放只比读法。

同一函数里还有一条评审非阻断，可在本票顺手一起看：免比主体轴的条件写成了「没给引用」，而票 20 的判断项只限「以名称素材指名」。后果是表达控制、但引用与素材都不给的命令，首判答 `INPUT_NOT_ACCEPTED`，落在同依据已形成的链尾上却答 `ALREADY_RECORDED`。要不要一起改由实施会话定；不一起改就另记一条。

## 完成判据

- [x] 「现象」里第 3 步答 200 `ALREADY_RECORDED` 并带回链尾，首判仍答 201 `PICKUP_VOIDED`。
- [x] 应用层与 HTTP 层各有一条回归用例，覆盖「失效命令带引用、不带种类」的首判与原样重放。
- [x] 票 20 的既有用例照过；改动包 `go vet` 与 `go test -count=1` 绿。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审（空闲通道或 `/code-review` 的隔离子代理）、推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-16，作者会话，分支 `claude-oc27`，代码 `1fcd01c6` / `11709736` / `c8768fd0`）

- **落点**：`replayOrConflictOnTheSameBasis` 只在命令读法表达控制时才立命令一侧的承运主体；读法不表达时主体轴不参与，与 TFC-1 在这一格只有读法一致。证据怎样指名承运主体（给引用 / 只给名称素材 / 都不给）抽成不碰 IO 的纯函数 `readCarrierNaming`（`application/carrier_naming.go`），首次有效收寄判断的 `resolveSubject`、实际承运商判断的 `resolveSubject` 与同依据重放三处共用。
- **票内非阻断一并修，理由**：它与本票是同一个函数里同一个口径的两半。只修失效那一半，表达控制而引用与素材都不给的命令仍会「首判未受理、重放已在册」，同一份输入两种答复。共用 `readCarrierNaming` 之后，两半由同一处判别给出。
- **补的格**：应用层 `TestAVoidingCommandWithAReferenceButNoKindReplaysAsAlreadyRecorded`（首判 `已失效`，原样重放 `已在册` 带回链尾，不落版本、不发意图）、`TestAControlCommandWithNoSubjectAtAllIsNotAcceptedOnTheSameBasisEither`（首判与同依据上都答 `未受理`，链不动）；HTTP 层 `TestAVoidingPayloadWithAReferenceButNoKindReplaysAsAlreadyRecorded`（照「现象」三步，201 `PICKUP_VOIDED` → 200 `ALREADY_RECORDED` 带 `pickup`）。三条在原函数上先红，见评审 Spec 轴实测。
- **票 20 完成记录里那句收窄了**：票 20 记的「同一依据重放时命令带了主体引用而分支词不在封闭集内，现在答`未受理`……这一格只在编排直调时出现」，现在只在读法表达控制时成立；「只在编排直调时出现」那半句已被本票证伪——载荷译装把引用无条件写进命令，操作者真渠道上也走得到。
- **判据**：前三条见上方勾选。第四条——分支、作者自验、隔离子代理评审已做，推送方全量见「进 main 记录」。`internal/transportfulfillment/...`、`cmd/parcel-api`、`cmd/parcel-dispatch`、`internal/architecture` 带 DSN 全绿，`go vet ./...` 退 0，`gofmt -l .` 无输出；机制清点 `021812e5`（TF 生产文件加一）。
- **评审**（用户 2026-10-16 改裁；当时唯一的非作者通道 `idp-parcel-9b` 在忙，用 `/code-review` 的隔离子代理，两轴分开）：
  - 第一轮（钉 `2b68b588`，重放前 SHA）——两轴都无阻断。Standards 非阻断两条：三处同形的指名判别靠手抄对齐，以及测试注释写了变更经过——都已修。Spec 非阻断三条：一并修的取舍要写进本票、票 20 那句要指明已收窄、推送方全量待做——前两条见上，第三条见进 main 记录。
  - 第二轮（Standards，钉 `b992860a`，重放前 SHA）——无阻断。实际承运商判断 `resolveSubject` 七格逐一核过，与改前一致。非阻断一条：头注里有对调用方的计数，已修于 `c8768fd0`。
- 证据层级 `S`。
