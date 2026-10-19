# 32 TF 更正载荷缺时刻答 400，与本包「缺席交编排、解不出才 400」相左；作业事实与运营决定 Intake 的逐口用例和头注补齐

Category: bug
Status: in-progress——2026-10-18 会话 `idp-parcel-9b` 立并认领（票 26 序 5、序 4 评审遗留；用户授权代决）；分支 `claude-oc32`
Blocked by: 无
父票：[26](./26-non-author-review-backlog-for-claude-code-landings.md)（序 4、序 5 的非阻断）。
出处：
- 票 [22](./22-tf-isolated-command-intake-retires.md) Comments「评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `7ec601ab`」的 Spec 非阻断 (1)(2)(3) 与 Standards (1)–(5)；
- 票 [15](./15-operation-decision-faces-take-operator-intake.md) Comments「钉 `60244fa6`」那条 Standards (3) 里 `decisionLines` 那一格；
- 簇 C4 研判的两路核验。
地盘：
- internal/transportfulfillment/adapters/http/work_fact_intake.go
- internal/transportfulfillment/adapters/http/work_fact_intake_test.go
- internal/transportfulfillment/adapters/http/operator_decision_intake_test.go
- internal/transportfulfillment/adapters/http/register_transport_handover.go
- internal/transportfulfillment/adapters/http/register_effective_delivery.go

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

## 现象（钉 `8d22729f`）

### 一、更正载荷的时刻格

`HandoverCorrectionPayload.Command`、`OffsitePickupCorrectionPayload.Command`、`DeliveryProofCorrectionPayload.Command` 对 `correctedAt`（揽收更正还有 `occurredAt`）直接调 `time.Parse`。下列情形一律答 400 `MALFORMED_REQUEST`：键缺席、空串、纯空白，以及前后带空白的合法时刻。交接更正的结论无条件过 `handoverVerdictFromWord`，结论缺席同样答 400。

- 本包其余时刻格都走 `parseOptionalInstant`，它的注释是：「空着交回零值，留给编排答`未受理`（200，形成了的业务答案）；给了却解不出是坏报文（400）——两者的续办不同，不折成一格。」
- 交接首登 `HandoverRegistrationPayload.Command` 写的是：「结论空着照零值交进去，其余成不成形都留给编排答`未受理`」。
- 编排和领域早已把缺更正时刻答成`未受理`：application 的 `TestAPickupCorrectionIsAsCompleteAsAFirstRegistration` 写明「缺四格任一、缺更正时刻或指名不全在受理处即未受理」；`OffsitePickup.Correct`、`TransportHandover.Correct`、`EffectiveDelivery.CorrectProof` 都有零值门。HTTP 先答 400，这些门从线上走不到。研判探针取证：只有把 `correctedAt` 写成零时刻字面量「0001-01-01T00:00:00Z」，请求才走到编排，答 200 `SOURCE_NOT_ACCEPTED`，不签版本。
- 同一份载荷里，`newProof`、`control` 等格缺席本来就交给编排（200 `SOURCE_NOT_ACCEPTED`）。
- 来历：更正载荷由 `ec00b795`（票 10 余下半边）新写，时刻那一格与本包端点测试里 Intake 替身的 `time.Parse` 同形；`parseOptionalInstant` 早在 `822c75a4`（票 08）就已存在。票 10 的判断项和它的非作者评审都没有提这一格，没有任何记录在案的决定。

### 二、测试缺口

下列每一条都用变异实测过：以 `go test -overlay` 挂进去，本包与 `cmd/parcel-api` 都全绿（即变异存活），另有注明的除外。

- `workFactLines` 只有首登那几口，类型锁 `TestWorkFactCommandIntakeServesOnlyWorkFactLines` 却断言更正各口也在面上。带 OnEveryLine 的用例和 `workFactLine` 注释「对每一口都成立」因此名不副实。
- 更正载荷的译装没有任何用例。以下错译都存活：交接更正丢掉 `basis`；揽收更正的 `place` 与 `control` 对调；POD 更正的 `newProof` 取成 `object`。
- 「先认证后解载荷」只在 `IntakeRegistration` 一口上证过。把交接更正口改成先解后认，存活。
- `workFactAuthenticatorFake` 的令牌为空分支没有用例走到。把 Intake 改成转交常量令牌，本包全绿，只被 `cmd/parcel-api` 的 `TestWorkFactFacesAnswerOnTheOperatorChannel`「no bearer token」那一格拦下。
- 票 10 的判断项「三头缺一答 400 `MALFORMED_REQUEST`，不占渠道那几格」与「种类不对答 400，发生在铸造之前」没有用例钉住。先认证、后判头或种类的变异，在本包和带 DSN 的 `cmd/parcel-api` 下都存活。

### 三、注释与代码不符

对照 AGENTS.md「写代码注释」「改文档」两节：

- `work_fact_intake_test.go` 文件头「这组译装用例原挂在隔离命令 Intake 上，随它退场迁来（票 22；ADR-0150 决定三），期望值不变」是变更说明，而且不实：移动事实来源的期望值已从 `SYN-SOURCE/self-operated-executor` 换成 `SELF_OPERATED`。
- 文件头、`TestWorkFactCommandIntakeTakesDeviceAndScopeFromHeadersAndAuthenticatesFirst` 的 Covers 和失败消息里的「三个头」，数的是另一个文件 `work_fact_intake.go` 里的头常量。
- `TestWorkFactCommandIntakeRefusesSelfReportedTenantOnEveryLine` 的 Covers「值与开关相同也拒」里的「开关」是隔离形态的注入值，现在租户来自认证结果。
- `TestWorkFactCommandIntakeServesOnlyWorkFactLines` 的 Covers 把更正各口归到「ADR-0149 决定二」；更正口归族的出处是 ADR-0151 决定四。
- `operator_decision_intake_test.go`「以下四口的解码用例随换口从隔离命令 Intake 迁来……」是变更说明，还指向一个已删的类型。`decisionLines` 头注「本包六个运营决定口」与 `TestEveryDecisionFaceRefusesSelfReportedTenantTrailingContentAndUnknownKeys` 的「六口都一样」是计数。
- `HandoverIntake` 头注「分开的理由是逐口放行（ADR-0091 Consequences）：一个只放行了首登的实现……」已经落空：那样的实现 `IsolatedCommandIntake` 已在 `7ec601ab` 删掉，现存的 `WorkFactCommandIntake` 与 `UnconfiguredIntake` 两口都实现。
- `DeliveryIntake` 头注「它是接口而不是本包内的解析代码……实现是 WorkFactCommandIntake」自相矛盾，因为后者就在本包。「ADR-0029 要求越权探针一律以「未找到」作答」不是 ADR-0029 的话，它的原句是「「查无此解析」与「范围不符」落同一格」；作业事实渠道实际答的也不是「未找到」，而是 `ErrDeviceNotInUse`。

## 裁定（2026-10-16，会话 `idp-parcel-9b` 代决；机制，不是租户取值；代决 · 越权风险点 · 待 TF owner 复核）

**这是缺陷，不是有意的例外：更正口的时刻照本包既有的分格走。**

- 时刻格（各更正口的 `correctedAt`、揽收更正的 `occurredAt`）键缺席、空串或纯空白时，交零值给编排，由编排答`未受理`（200 `SOURCE_NOT_ACCEPTED`）。
- 给了值却解不出时，答 400 `MALFORMED_REQUEST`。
- 任何情况都不拿服务端时钟补。依据是 ADR-0023，以及 `DeliveryIntake` 契约「服务器代铸任何一样，离线补传的重放就会被误判成新事实」。
- 交接更正的结论与 `HandoverRegistrationPayload.Command` 用同一分格：空着照零值交进去，词表外答 400。

### 依据

决定性的几条都在本上下文之内：
- 这条分格在本包由 `parseOptionalInstant` 定义，是票 08「事实身份与发生时间照 ADR-0023 从载荷收、缺席交编排、不代铸」在 TF 的落点。更正载荷是唯一的偏离，没有任何记录在案的决定。
- 编排和领域已经为缺更正时刻备好了`未受理`这一格。HTTP 先拒，这些门就从线上走不到，票 tf-segment-lifecycle-closure/08 裁的「每格完备性同首登」在线上也就不成立。
- 同一份载荷里，其余缺席格本来就交给编排。

旁证：
- ADR-0022「传输层不比应用层多知道什么，也不再自行归类」。
- ADR-0022 的表里，2xx 是「出队，按 `outcome` 呈现给操作员」，4xx 是「出队，交给人，不自动重发」。两格在离线端都出队、都不自动重发，改格不伤调用方。
- 今天 `apps/` 与 `scripts/` 都没有调用这几个更正口。

### 不据此立全仓规则

「缺席答 400」在别处有合法先例：ADR-0124 决定三（计价回放「缺任何一样是畸形请求」）；ADR-0055 决定二把 `MALFORMED_REQUEST` 的恢复动作定为「调用方改请求」。节点收寄的 `receptionCommand` 缺 `occurredAt` 或 `sourceId` 也答 400，那是 NO 的格，交「作业事实面缺席格」那张票。本票的注释和票面只说 TF 包内的分格。

### 不立 ADR

这是执行包内已有的规则，不是新取舍。业务规则不变：UC-TF-005「任何人工更正保留原证据、原判断、原因、决定方和生效时间」、UC-TF-006「任何人工更正保存决定方、授权、原因和生效时间」仍由领域的零值门守着，变的只是答复格。

### 可观测变化（请 TF owner 过目）

- 更正口的时刻缺席、空串或纯空白，或交接结论缺席：400 → 200 `SOURCE_NOT_ACCEPTED`。
- 时刻前后带空白的合法值：400 → 照常解出（揽收更正答 201 `PICKUP_CORRECTED`，其余同理）。时刻在 Intake 就折成 UTC；领域的 `Correct` 和 `canonicalInstant` 本来就折 UTC，已存的摘要不受影响。
- POD 更正缺时刻：编排在 `CorrectProof` 拒之前已经签出一个交付结果版本号，序列空推一格。这是既有的格：任何被 `CorrectProof` 拒的 POD 更正今天都这样，交「TF 更正口重放代数」票一并看。
- 揽收更正以缺 `correctedAt` 的同内容重放：答 `EXISTING_VERSION`（摘要不含更正时刻）。前版已被更正之后再指名它、且缺 `occurredAt`：答 `SOURCE_CONFLICT`。

## 做什么

1. `work_fact_intake.go`：
   - 更正各载荷的时刻改调 `parseOptionalInstant`。
   - `HandoverCorrectionPayload.Command` 照 `HandoverRegistrationPayload.Command` 的写法，结论非空才译。
   - 删掉不再用的 `time` 导入。
   - 头注只在 `HandoverCorrectionPayload` 上加一句指针「结论的分格同 HandoverRegistrationPayload.Command」。另外两个载荷不复述分格。
2. `work_fact_intake_test.go` 补用例（带★的各组在 main 上先红）：
   - `workFactLines` 补上更正各口。合规载荷常量取名 `workFactPickupCorrectionBody`、`workFactHandoverCorrectionBody`、`workFactDeliveryProofCorrectionBody`（避开同包已有的 `pickupCorrectionBody`、`handoverCorrectionBody`、`correctBody`），只用 `SYN-` 值，时刻带 `+08:00`。
   - 更正各口各一条译装用例，例如 `TestWorkFactCommandIntakeTranslatesPickupCorrectionWithTheAuthenticatedTenant`，交接与 POD 同形：各格逐字来自载荷，时刻相等，租户来自认证结果。
   - ★ `TestWorkFactCommandIntakeLeavesAbsentCorrectionInstantsToTheOrchestration`。行：揽收更正 `correctedAt`、揽收更正 `occurredAt`、交接更正 `correctedAt`、POD 更正 `correctedAt`。格：键缺席、空串、纯空白，期望零值且无错；前后带空白的合法时刻，期望解出同一时刻。
   - `TestWorkFactCommandIntakeRefusesUnparsableCorrectionInstants`：同样几行填「昨天」，期望 `ErrMalformedRequest`，且错误信息点名该键。
   - ★ 交接更正结论：缺席 → 零值、无错；`MAYBE` → `ErrMalformedRequest`。
   - ★ 端点层（同一文件）：真 `WorkFactCommandIntake`（替身认证方）接本包真编排与登记册替身，先登前版再更正。更正各口缺 `correctedAt` 时答 200 `SOURCE_NOT_ACCEPTED`，且册上不多一行；解不出时答 400 `MALFORMED_REQUEST`，不带 `outcome`。不断言签发次数。
   - 把 `TestWorkFactCommandIntakeTakesDeviceAndScopeFromHeadersAndAuthenticatesFirst` 改为逐口的 `TestWorkFactCommandIntakeAuthenticatesBeforeDecodingOnEveryLine`。旧名 → 新名记进完成记录，因为票 22 的完成记录按旧名引过它。对 `workFactLines` 的每一口验：
     · 合规出示时，认证方恰好被问一次，问的是请求头里那份设备与作业范围；
     · 认证方拒（`ErrDeviceNotInUse`）且载荷为 `!!not-json!!` 时，交回认证方的格；
     · 删掉 `Authorization` 且载荷坏时，答 `ErrOperatorCredentialRejected`；
     · 缺任一头或种类为 `NODE` 时，答 `ErrMalformedRequest`，**而且认证方一次也没被问到**。
3. `operator_decision_intake_test.go`：`TestEachTFDecisionFaceAuthenticatesForItsOwnDecision` 的请求体 `{}` 全换成 `!!not-json!!`，在包内另钉一处「先认证、后解载荷」。`cmd/parcel-api` 的 `TestEveryAssembledEndpointAnswersUnconfigured` 已经在未配置装配上钉住了这一条。
4. 注释。要求：中文，只写为什么，不写变更说明，跨文件只用符号名或引文，不计数。
   - `work_fact_intake_test.go` 文件头：删去「这组译装用例原挂在……期望值不变。」；「设备与作业范围只从三个头来」改为「设备与作业范围只从请求头来（读法见 WorkFactCommandIntake.tenantFrom）」；不另外复述时刻分格。
   - 逐口用例的 Covers 用引文指向票 10 的判断项「三头缺一答 400 `MALFORMED_REQUEST`，不占渠道那几格」「种类不对答 400，发生在铸造之前」。失败消息改为「want 请求头里的设备与作业范围」。
   - `workFactLine` 注释改为：「自报租户、封闭解码与先认证后解载荷对每一口都成立，带 OnEveryLine 的用例逐口跑这张表；类型锁 TestWorkFactCommandIntakeServesOnlyWorkFactLines 按接口另列，加口时两处都要加。」
   - `TestWorkFactCommandIntakeRefusesSelfReportedTenantOnEveryLine` 的 Covers：「值与开关相同也拒」改为「值与认证出的租户相同也拒」。
   - `TestWorkFactCommandIntakeServesOnlyWorkFactLines` 的 Covers 改为：作业事实这一面实现「首登各口（ADR-0149 决定二）与更正各口（随原事实归族，ADR-0151 决定四）」。
   - `operator_decision_intake_test.go`：那句「以下四口的解码用例随换口……」改为「以下解码用例证各口线格式译装没换字：期望值取自载荷字面量而不是重算，租户只来自认证结果。」；`decisionLines` 头注「本包六个运营决定口」改为「OperatorDecisionIntake 实现的各运营决定口」；「六口都一样」改为「每一口都一样」。
   - `HandoverIntake` 头注：
     · 首段改为「端点收接口的理由同 DeliveryIntake。实现是操作者渠道「作业事实登记」能力面的 WorkFactCommandIntake：首登（ADR-0149 决定二）与更正（随原事实归族、与首登同一份认证与授予，ADR-0151 决定四）两口都在。」
     · 末段的分开理由改为「分开是为了端点在类型上只依赖自己那一口：装配点逐端点换 Intake（assembleBusinessEndpoints 头注「逐端点替换」），往既有接口加方法又会打断它的每个实现者（理由同 PickupCorrectionIntake）。」
     · **不**拿 ADR-0151 越权风险点「更正的提交方不总是原提交方」当理由。它与决定四「更正与首登的提交方是同一方，认证与授予也应是同一份」相抵，而且仍未裁。
   - `DeliveryIntake` 头注：把「它是接口而不是本包内的解析代码……」那一段改为「端点收接口、不直接调本包的载荷解析：装配点要能逐端点在未配置即拒（UnconfiguredIntake，ADR-0055）与真渠道之间换，处理器与路由不动。真渠道是操作者渠道「作业事实登记」能力面的 WorkFactCommandIntake——首登归此面（ADR-0149 决定二），POD 更正随原事实归此面（ADR-0151 决定四）。租户身份按 ADR-0003 只能来自它的认证结果；设备查无、停用或登在别的作业范围上怎样分格，是认证层的话（见 accessidentity.ErrDeviceNotInUse）。本包不带任何采信头部自报的「开发用」实现。」「事实内容必须从请求体收（ADR-0023）」那一段和「分法与理由同 HandoverIntake」保留。其余各 Intake 头注里「接口而非（本包内）解析代码的理由同 DeliveryIntake」照旧指向它，不逐个改。

## 不做

- 不改编排、领域与 `cmd/parcel-api`。`cmd` 里的同类注释归「cmd/parcel-api 运输履约段行注」票。
- 判断载荷 `EffectiveTimeJudgmentPayload.Command`、`CarrierPickupJudgmentPayload.Command` 内联了与 `parseOptionalInstant` 同形的分格。收拢到 `parseOptionalInstant` 行为逐字不变（研判已实测），但簇 C3（TF 在册时刻精度）的研判草稿也要改这两个文件，所以并入 C3 那张票；若 C3 不立票，在本票进 main 之后另起一笔。
- 不补 POD 更正的重放代数与交接更正的内容冲突，也不处理「POD 缺时刻也签版本号」：归「TF 更正口重放代数」票。
- 不改 NO 收寄的分格：归「作业事实面缺席格」票。
- 带 OnEveryLine 的用例不改名；替身的令牌为空分支不删；`HandoverIntake`、`DeliveryIntake` 合集接口不删（它们是 `UnconfiguredIntake` 编译期断言和多处「理由同 …Intake」的锚）。
- 不在 `cmd/parcel-api` 的真库表里补更正各行。

## 完成判据

- [ ] 带★的各组在 main 上先红，改后转绿；红的输出记进完成记录。
- [ ] 用 `go test -overlay` 取证并记进完成记录：下列变异各被至少一条新用例杀掉——交接更正口先解载荷后认证；Intake 转交常量令牌；更正各载荷各错译一格；时刻只判非空、不去空白；先认证后判三头或种类。
- [ ] `work_fact_intake.go` 里不再有 `time.Parse(`。两份测试文件与两份 Intake 头注里不再有「三个头」「六个运营决定口」「六口」「迁来」「值与开关」「逐口放行」「越权探针一律以「未找到」作答」。
- [ ] Go 门禁：`gofmt -l` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/` 全 ok，其中 `TestWorkFactFacesAnswerOnTheOperatorChannel`、`TestTransportFulfillmentCommandLinesAnswerBusinessOutcomesAgainstARealDatabase`、`TestTheWiredPickupCorrectionLandsANewVersionAgainstARealDatabase` 是 PASS 而不是 SKIP。机制清点生成器在干净检出上重跑零差（不新增文件）。
- [ ] 走 Go 那条路（parallel-sessions）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。
- [ ] 票 22 Comments 那条评审下追记处置（Spec 非阻断与 Standards 各条指向本票，cmd 部分指向「cmd/parcel-api 运输履约段行注」票）；票 15「钉 `60244fa6`」那条追记 `decisionLines` 一格；票 26「评审结果」序 4、序 5 的去向格补上本票号。证据只记 `S`。
