# 28 受控关闭与重开两口没核重放代数就开了真渠道：同一个关闭 POST 重试会再追加一条关闭

Category: bug
Status: resolved——2026-10-16 进 main（`6c9f4886` 起，分支 `claude-oc28` 快进，远端 main 含 `fcf9b4ea`）；见「进 main 记录」。此前 in-progress——2026-10-16 完工，待进 main；见「完成记录」。此前 in-progress——2026-10-16 作者会话（票 15 之六的代码作者，Claude Code 会话）分诊并认领：「要裁的一问」裁 (b)，见「裁定」；分支 `claude-oc28`。此前 needs-triage——2026-10-16 会话 `idp-parcel-9b` 立（票 26 序 1 评审阻断；评审会话在 main `4dd937eb` 上复核）。要先裁一问（见「要裁的一问」），裁完转 ready-for-agent。
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

## 裁定（2026-10-16，作者会话，用户「你帮我做决定吧」授权代决；机制取舍，不是租户取值）

**取 (b)**：不加请求键，按「册上已有同类同内容的决定」认重放，答`已有结果`（`EXISTING_RESULT`）交回原决定。比哪几格、在哪一步认、为什么不取 (a)、为什么不一律拒「关闭中再关闭」、调用方须固定生效时间——都以 [ADR-0183](../../../docs/adr/0183-operator-channel-addresses-controlled-closure-and-reopening-by-parcel-and-derives-the-shipper-from-the-accepted-request.md)「复核记录二」为准，本票不复述。

「一并看」两条非阻断一并修，理由同在那一节。

## 完成判据

- [x] 「要裁的一问」已裁，裁定写进本票，难逆转的进 ADR。
- [x] 同一个关闭请求原样重放，不再追加第二条关闭，答复带回原决定；重开同理。领域、编排、HTTP 各有回归用例。
- [x] 票 15 补记这两口凭哪条代数。
- [x] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审（空闲通道或 `/code-review` 的隔离子代理）、推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-16，作者会话，分支 `claude-oc28`，代码 `6c9f4886` / `312b8b54` / `752b03e9`）

- **落点**：领域 `ContinuedAttemptRequest` 与 `ContinuedAttemptRegister.RecordedRequest`；编排 `FormContinuedAttemptDecisionHandler.form` 在授权与终局之后、货主重授权那道门与签发标识之前认重放，新结果格 `ContinuedAttemptDecisionExistingResult`（`EXISTING_RESULT`，HTTP 200 带回原决定）；`formation.request` 直接带请求内容，`decisionSpecOf` 一处合成规格。Intake：关闭两道纯载荷校验挪到按包裹寻址之前，主体与证据全空白与没给同读；证据判据 `application.HasShipperAuthorizationEvidence` 一处定义。规则全文在 ADR-0183「复核记录二」。
- **补的格**：领域 `TestARegisterRecognisesTheSameRequestOnlyWhenEveryRequestCellMatches`（任一请求格不同都不是重放）；编排 `TestARetriedClosureAnswersTheExistingDecisionAndTheReopeningStillFindsIt`（关闭重投与重开重投各答已有结果、不签标识不写册不发意图，指名原关闭的重开照常形成）、`TestOnlyTheSameContentIsAReplayAndAuthorizationStillComesFirst`（换责任来源的第二条关闭照旧追加；未获授权的重放答未获授权）；Intake `TestABadClosurePayloadAnswers400EvenWhenTheParcelIsUnknown`、`TestABlankShipperAuthorizationEvidenceLeavesTheRequesterAbsent`；真库操作者渠道用例 `TestClosureAndReopeningThroughTheOperatorChannelAgainstARealDatabase` 补关闭与重开两次重投（200 `EXISTING_RESULT`、同一决定标识、册上不多一条）。
- **HTTP 映射为什么只有带库用例守**：`adapters/http` 测试文件头注定的分工——带业务结果的响应体逐格钉由 `cmd/parcel-api` 真库装配用例做，结果字段未导出是刻意的（传输层不该能捏造业务结果）。缺 DSN 时这些用例在 CI 上 `t.Fatal`（`pgtest.AdminDSN` 在设了 `CI` 时如此），本地缺 DSN 则跳过。
- **判据**：前三条见上方勾选（票 15 补记在其文末「补记」节）；第四条见「进 main 记录」。`internal/parcelshipment/...`、`cmd/parcel-api`、`cmd/parcel-dispatch`、`tests/`、`internal/architecture` 带 DSN 全绿，`go vet ./...` 退 0，`gofmt -l .` 无输出。机制清点重生成无差异。
- **评审**（非作者通道 `idp-parcel-9b` 在忙，用 `/code-review` 的隔离子代理，两轴分开）：
  - 第一轮（钉 `6c9f4886`）——Standards 阻断 1：请求内容类型插进了另一段头注中间，两段注释都成了假话；非阻断：编排逐格重拼请求内容、「空白与没给同读」两份、重放规则票与 ADR 各一份且对不上、两处头注过时。Spec 无阻断，非阻断：票 15 补记待做、头注过时、HTTP 映射只有带库用例守、调用方须固定生效时间、头注被劈开。全部修于 `312b8b54`。
  - 第二轮（钉 `312b8b54`）——Standards 阻断 1：新证据判据的头注在数调用方；非阻断：三处头注仍指票 28、一句断言与已接受取舍相左、`spec` 钩子只转发。修于 `752b03e9`。Spec 无阻断，非阻断两条（票 15 补记的写法要点、DSN 缺失时的准确说法）收在本记录与票 15 补记里。
  - 第三轮（Standards，钉 `752b03e9`）——无阻断、无非阻断；关闭仍带截断边界（值即决定标识）、重开不带，与改前逐格一致。
- 证据层级 `S`。

### 进 main 记录（2026-10-16）

- 分支 `claude-oc28` 四笔 `6c9f4886` / `312b8b54` / `752b03e9`（代码与 ADR-0183 复核记录二）/ `fcf9b4ea`（记录与票 15 补记）直接叠在远端 main `8d22729f` 之上，快进进 main、推的就是这个 SHA，未重放。机制清点重生成无差异，没有清点那一笔。
- 推送方全量：钉 `fcf9b4ea` 的干净 detached 检出，带 DSN，`go test -p 1 -count=1 -v ./...` 退 0——有测试的包 142 个 ok、0 FAIL，`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）；同一检出上 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0。
- 生产上仍潜伏：两口的授权映射为空、停在 `UNDECIDED / AUTHORITY_UNAVAILABLE`（现象节原话），授权缺口补上之后重放代数已就位。

