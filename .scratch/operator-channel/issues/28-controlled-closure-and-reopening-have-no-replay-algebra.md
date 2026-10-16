# 28 受控关闭与重开两口没核重放代数就开了真渠道：同一个关闭 POST 重试会再追加一条关闭

Category: bug
Status: needs-triage——2026-10-16 会话 `idp-parcel-9b` 立（票 26 序 1 评审阻断；评审会话在 main `4dd937eb` 上复核）。要先裁一问（见「要裁的一问」），裁完转 ready-for-agent。
Blocked by: 无
父票：[26](./26-non-author-review-backlog-for-claude-code-landings.md)（评审欠账；本票是它点名的「有阻断的，各立修复票」）。被评的是 [15](./15-operation-decision-faces-take-operator-intake.md) 之六，代码 `0e3bb7df`，裁定见 [ADR-0183](../../../docs/adr/0183-operator-channel-addresses-controlled-closure-and-reopening-by-parcel-and-derives-the-shipper-from-the-accepted-request.md)。
出处：票 15 的 Comments「评审 ← 会话 idp-parcel-9b 起的隔离子代理」钉 `0e3bb7df` 那一条，Spec 轴阻断 1。

## 现象

`/shipment-requests/continued-attempt-closures` 与 `/shipment-requests/continued-attempt-reopenings` 两口在 `0e3bb7df` 接上了操作者真渠道。

票 15「做什么」那一条写的是：「逐口核载荷规范化——领域已有同身份重放与内容冲突代数的口只核对，逐口记下凭哪条代数；没有的补一版带形状版本的规范化形状。**未核完的口不开真渠道。**」票 15 完成记录「之六」与 ADR-0183 里都没有这两口的核对记录。

这两口其实也没有重放代数：

- 决定标识由本上下文现铸：`FormContinuedAttemptDecisionHandler.form` 每次都调 `Identities.NextContinuedAttemptDecisionID`；命令里没有请求键。关闭这条路径也没有 `beforeIdentity` 守卫，不拒已经在关闭中的册。
- 领域层照收：评审会话在 main `4dd937eb` 上用领域探针取证，同样的关闭内容、同一生效时刻、换一个新铸的标识再追加一次，`ContinuedAttemptRegister.Append` 返回 nil。册上变成两条决定，`StandingClosure()` 换成第二条；之后指名第一条关闭的重开答 `ErrContinuedAttemptDecisionNotAdmitted`。

于是同一个关闭 POST 因网络超时重试一次，就会多出一条关闭；货主据第一次答复里的决定标识去请求重开时，被拒。

**目前潜伏，未发作**：生产装配上这两口的授权映射为空，停在 `UNDECIDED / AUTHORITY_UNAVAILABLE`，形成不了决定（票 15 完成记录原话）。授权缺口一旦补上就会发作，所以**必须在授权映射接上之前修**。授权缺口那张票不在本会话地盘，没去改它的 Blocked by，由 triage 时一并接上这条边。

## 要裁的一问

**按什么认出「同一次请求的重放」。** 这是机制取舍，归开发方做。若属难逆转的取舍就出 ADR，或在 ADR-0183 后追加复核记录（决定原文不动）。候选：

- (a) 命令带请求键：调用方给，或由入口按规范化载荷算出带形状版本的摘要，参照同仓 TFC-1、CCC-1 那一类做法；同键同内容答「已形成」并交回原决定，同键异内容答内容冲突。
- (b) 不加键，册上已有生效关闭、且请求方与原因等内容相同时，答「已形成」并交回那一条。

两个候选对「同一货主隔天再发一次同样内容的关闭」的读法不同：(a) 认作新请求，(b) 认作重放。这也是要裁的一部分。

## 一并看（同两口的评审非阻断，可在本票顺手处理，或另记）

- `IntakeReopening`：Intake 用 `!= ""` 判断「带了证据」，编排那道门用 `TrimSpace`。全空白的证据会让请求方记成客户，门却当作没带，与 ADR-0183 决定四「没带时请求方缺席」不符（评审探针实测）。
- `IntakeControlledClosure`：两道纯载荷的责任主体校验排在 `locateParcel` 之后，包裹查不到时答 403；而 ADR-0183 决定三要求这种坏载荷答 400（评审探针实测）。

## 完成判据

- [ ] 「要裁的一问」已裁，裁定写进本票，难逆转的进 ADR。
- [ ] 同一个关闭请求原样重放，不再追加第二条关闭，答复带回原决定；重开同理。领域、编排、HTTP 各有回归用例。
- [ ] 票 15 补记这两口凭哪条代数。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审（空闲通道或 `/code-review` 的隔离子代理）、推送方在干净检出上带 DSN 跑全量。
