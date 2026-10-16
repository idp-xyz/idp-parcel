# 27 承运商首次有效收寄判断：失效命令带引用、不带种类时，原样重放答 `INPUT_NOT_ACCEPTED` 而不是 `ALREADY_RECORDED`

Category: bug
Status: in-progress——2026-10-16 作者会话（票 20 的代码作者，Claude Code 会话）认领，分支 `claude-oc27`，基线 `d3caf1b0`；评审照改裁走非作者通道或 `/code-review` 的隔离子代理。此前 ready-for-agent——2026-10-16 会话 `idp-parcel-9b` 立（票 26 序 3 评审阻断；作者会话在 main `4dd937eb` 上复现）
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

- [ ] 「现象」里第 3 步答 200 `ALREADY_RECORDED` 并带回链尾，首判仍答 201 `PICKUP_VOIDED`。
- [ ] 应用层与 HTTP 层各有一条回归用例，覆盖「失效命令带引用、不带种类」的首判与原样重放。
- [ ] 票 20 的既有用例照过；改动包 `go vet` 与 `go test -count=1` 绿。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审（空闲通道或 `/code-review` 的隔离子代理）、推送方在干净检出上带 DSN 跑全量。
