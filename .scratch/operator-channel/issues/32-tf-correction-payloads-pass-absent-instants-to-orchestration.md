# 32 TF 更正载荷缺时刻答 400，与本包「缺席交编排、解不出才 400」相左；作业事实与运营决定 Intake 的逐口用例和头注补齐

Category: bug
Status: resolved——2026-10-19 进 main（45、no-reception-identity/03、32 同批，代码验证钉 `77ffe641`），见「进 main 记录」。此前 in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 续做：分支 `claude-oc32` 上的两笔先 rebase 到最新 main 并核一遍，再做完余下的格。此前 ready-for-agent——2026-10-18 暂缓：用户因 token 花费选「精简续做」，本票没做完；分支 `claude-oc32`（基 `b58b10ef`，工作树 `/home/tops/workspace/idp-parcel-claude-oc32`）上留有2 笔（更正载荷缺时刻交编排、作业事实表补更正各口），未经评审，接手前先 rebase 到最新 main 并核一遍。此前 in-progress——2026-10-18 会话 `idp-parcel-9b` 立并认领（票 26 序 5、序 4 评审遗留；用户授权代决）；分支 `claude-oc32`
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

- [x] 带★的各组在 main 上先红，改后转绿；红的输出记进完成记录。
- [x] 用 `go test -overlay` 取证并记进完成记录：下列变异各被至少一条新用例杀掉——交接更正口先解载荷后认证；Intake 转交常量令牌；更正各载荷各错译一格；时刻只判非空、不去空白；先认证后判三头或种类。
- [x] `work_fact_intake.go` 里不再有 `time.Parse(`。两份测试文件与两份 Intake 头注里不再有「三个头」「六个运营决定口」「六口」「迁来」「值与开关」「逐口放行」「越权探针一律以「未找到」作答」。
- [x] Go 门禁：`gofmt -l` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/` 全 ok，其中 `TestWorkFactFacesAnswerOnTheOperatorChannel`、`TestTransportFulfillmentCommandLinesAnswerBusinessOutcomesAgainstARealDatabase`、`TestTheWiredPickupCorrectionLandsANewVersionAgainstARealDatabase` 是 PASS 而不是 SKIP。机制清点生成器在干净检出上重跑零差（不新增文件）。
- [x] 走 Go 那条路（parallel-sessions）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。
- [x] 票 22 Comments 那条评审下追记处置（Spec 非阻断与 Standards 各条指向本票，cmd 部分指向「cmd/parcel-api 运输履约段行注」票）；票 15「钉 `60244fa6`」那条追记 `decisionLines` 一格；票 26「评审结果」序 4、序 5 的去向格补上本票号。证据只记 `S`。

## 完成记录（2026-10-19，作者子代理，分支 `claude-oc32`，待评审）

### rebase

两笔半成品原基 `b58b10ef`。先 rebase 到当时的 `origin/main` `4a09e100`；其后 main 又进了 tf-face-algebra/10、12、13 与 operator-channel/44 等，再 rebase 到 `19d9d486`。两次都没有文本冲突，有一处语义冲突：

- `work_fact_intake.go`：main 在 `b58b10ef` 之后只改了 `DeliveryProofCorrectionPayload`（票 41 加 `predecessorVersion`）。更正载荷的时刻仍是 `time.Parse`，交接更正结论仍无条件译，main 上没有等价的分格，也没有改走 `parseOptionalInstant`；本票的改动直接叠上，main 的格一个没删。
- `work_fact_intake_test.go`：main（票 41）已立同名常量 `workFactDeliveryProofCorrectionBody` 与 `TestWorkFactCommandIntakeTranslatesDeliveryProofCorrectionWithItsPredecessor`，与本分支的同名常量重定义，编译不过。以 main 为准：
  - 留 main 的常量（带 `predecessorVersion`）。`correctedAt` 字面量由 `2026-09-06T08:00:00Z` 改写成同一刻的 `2026-09-06T16:00:00+08:00`，以合本票「时刻带 `+08:00`」；main 那条用例的期望值一字不动。
  - 删本分支的同名常量，以及重复的 `TestWorkFactCommandIntakeTranslatesDeliveryProofCorrectionWithTheAuthenticatedTenant`：main 那条已逐格断言租户、尝试、对象、前版、新证据与更正时刻。
  - POD 端点夹具的更正载荷改从首登答复的 `deliveryVersion` 填 `predecessorVersion`。票 41 之后，前版是编排认「同一次更正」的锚，不填，对照格落不了 201。
- 解法改在各自那一笔里：fix 笔改常量与夹具，test 笔删重复用例。两笔各在临时检出上 `go vet` 与本包 `go test` 绿。test 笔的提交说明随之改写：原说明写「三份载荷各一条译装」，还引了以基线 `b58b10ef` 做的取证。

### 落点

- `be46c524` fix：`work_fact_intake.go` 更正三载荷的时刻改走 `parseOptionalInstant`，交接更正结论非空才译，删 `time` 导入，`HandoverCorrectionPayload` 头注加指针；带★三组用例。
- `f66e9fa1` test：`workFactLines` 补更正三口；揽收、交接更正译装；解不出点名该键；逐口先认证后解载荷。
- `34504292` test：运营决定口在包内钉先认证后解载荷（做什么第 3 条）；第 4 条各处注释（`work_fact_intake_test.go`、`operator_decision_intake_test.go`、`register_transport_handover.go`、`register_effective_delivery.go`）。
- 本笔 docs：本记录，以及票 22、15、26 的追记（只加行）。

### 各格对应用例（`internal/transportfulfillment/adapters/http/`）

| 票面格 | 用例 |
|---|---|
| 1 更正载荷分格 | 产品码；`work_fact_intake.go` 里 `time.Parse(` 零命中 |
| 2 `workFactLines` 补更正各口 | 表行 `/transport-fulfillment/offsite-pickup-corrections`、`/transport-fulfillment/handover-corrections`、`/transport-fulfillment/delivery-proof-corrections`；逐口跑它的是 `TestWorkFactCommandIntakeRefusesSelfReportedTenantOnEveryLine`、`TestWorkFactCommandIntakeRefusesTrailingContentAndUnknownKeysOnEveryLine`、`TestWorkFactCommandIntakeAuthenticatesBeforeDecodingOnEveryLine` |
| 2 更正译装 | `TestWorkFactCommandIntakeTranslatesPickupCorrectionWithTheAuthenticatedTenant`、`TestWorkFactCommandIntakeTranslatesHandoverCorrectionWithTheAuthenticatedTenant`；POD 由 main 已有的 `TestWorkFactCommandIntakeTranslatesDeliveryProofCorrectionWithItsPredecessor` 守 |
| 2 ★时刻缺席 | `TestWorkFactCommandIntakeLeavesAbsentCorrectionInstantsToTheOrchestration`：四行（揽收更正 `correctedAt` / `occurredAt`、交接更正 `correctedAt`、POD 更正 `correctedAt`），每行四格（键缺席、空串、纯空白、前后带空白的合法时刻） |
| 2 时刻解不出 | `TestWorkFactCommandIntakeRefusesUnparsableCorrectionInstants` |
| 2 ★交接更正结论 | `TestWorkFactCommandIntakeLeavesAnAbsentHandoverCorrectionVerdictToTheOrchestration` |
| 2 ★端点层 | `TestWorkFactCorrectionEndpointsLeaveAnAbsentCorrectionInstantToTheOrchestration`：三口各一子用例；末格带上时刻答 201 作对照；不断言签发次数 |
| 2 逐口先认证 | `TestWorkFactCommandIntakeAuthenticatesBeforeDecodingOnEveryLine` |
| 3 | `TestEachTFDecisionFaceAuthenticatesForItsOwnDecision`：请求体换成 `!!not-json!!`，另加 Covers |
| 4 注释 | 见 `34504292`；完成判据第三格列的七个词在两份测试文件与两份 Intake 头注里零命中 |

**旧名 → 新名**：`TestWorkFactCommandIntakeTakesDeviceAndScopeFromHeadersAndAuthenticatesFirst` → `TestWorkFactCommandIntakeAuthenticatesBeforeDecodingOnEveryLine`。票 22 完成记录按旧名引过它。

### 先红

日志：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/oc32-red.log`（本机 scratchpad，会随会话清掉，要点抄在下面）。做法：在 `34504292` 上用 `go test -overlay` 把 `work_fact_intake.go` 换成 `origin/main` `19d9d486` 的版本，只跑带★的三组。

- 三组顶层都 FAIL，FAIL 行共 22 条。
- 时刻那组 16 个子用例全红：缺席、空串、纯空白被当成坏报文，前后带空白的合法时刻被拒。
- 交接结论：`缺结论被当成坏报文：…verdict="" is not a handover verdict word`。
- 端点三口都红：`缺更正时刻：answer = 400 {"error":{"code":"MALFORMED_REQUEST"}}`。
- 对照：同组用例不挂 overlay，三组 PASS。

### 变异取证

变异文件与 overlay 在 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/oc32-mut/`。「main 用例」列是本分支产品码加变异，再挂上 `origin/main` 版的 `work_fact_intake_test.go` 与 `operator_decision_intake_test.go`；不加变异时这一组合本包 ok。

| 变异 | 本分支用例 | 杀掉它的用例 | main 用例 |
|---|---|---|---|
| m1 交接更正口先解载荷后认证 | 杀 | `TestWorkFactCommandIntakeAuthenticatesBeforeDecodingOnEveryLine`（设备不在用那一格交回了载荷校验的结果） | 存活 |
| m2 Intake 转交常量令牌 | 杀 | `TestWorkFactCommandIntakeAuthenticatesBeforeDecodingOnEveryLine`（没带令牌那一格） | 存活 |
| m3a 交接更正丢掉 `basis` | 杀 | `TestWorkFactCommandIntakeTranslatesHandoverCorrectionWithTheAuthenticatedTenant`、`TestWorkFactCorrectionEndpointsLeaveAnAbsentCorrectionInstantToTheOrchestration` | 存活 |
| m3b 揽收更正 `place` 与 `control` 对调 | 杀 | `TestWorkFactCommandIntakeTranslatesPickupCorrectionWithTheAuthenticatedTenant` | 存活 |
| m3c POD 更正 `newProof` 取成 `object` | 杀 | `TestWorkFactCommandIntakeTranslatesDeliveryProofCorrectionWithItsPredecessor` | 已杀（票 41 那条用例在本票钉 `8d22729f` 之后进的 main） |
| m4 更正时刻只判非空、不去空白 | 杀 | `TestWorkFactCommandIntakeLeavesAbsentCorrectionInstantsToTheOrchestration`（纯空白、前后带空白两格） | 存活 |
| m5a 先认证、后判三头 | 杀 | `TestWorkFactCommandIntakeAuthenticatesBeforeDecodingOnEveryLine`（缺头时认证方被问到） | 存活 |
| m5b 判完三头、先认证、后判种类 | 杀 | `TestWorkFactCommandIntakeAuthenticatesBeforeDecodingOnEveryLine`（NODE 时认证方被问到） | 存活 |
| m6 关段口先解载荷后认证（第 3 条） | 杀 | `TestEachTFDecisionFaceAuthenticatesForItsOwnDecision` | 存活 |

m5a、m5b 都只问认证方一次，答复留到判完头部或种类再交，这样就不会因多问一次被旧用例「恰好一次」那一格顺手杀掉。只在本包取证，没有挂到 `cmd/parcel-api` 上重跑。

### 门

- 改动的 `.go` 文件 `gofmt -l` 无输出；`go build ./...`、`go vet ./...` 退 0。
- 带 DSN（`127.0.0.1:55432`）跑 `go test -p 1 -count=1 -v ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...`：11 个包 ok（`ports` 无测试文件），0 FAIL，0 SKIP。`TestWorkFactFacesAnswerOnTheOperatorChannel`、`TestTransportFulfillmentCommandLinesAnswerBusinessOutcomesAgainstARealDatabase`、`TestTheWiredPickupCorrectionLandsANewVersionAgainstARealDatabase` 三条 PASS。
- 机制清点生成器在提交后的干净工作树上重跑，零差，不新增文件。
- 没跑全量 `./...`，留给推送方。

### 证据层级

`S`：合成 `SYN-` 值、替身认证方、包内编排接登记册替身，真库用例用本机测试库。没碰演示库。

### 越出地盘的改动

代码没有越出地盘。票 22、15、26 只加行，是完成判据要求的追记。地盘内动了 main（票 41）引入的常量 `workFactDeliveryProofCorrectionBody`：只改时刻写法，还是同一刻。

### 未做

- 完成判据「走 Go 那条路」里的非作者评审，以及推送方在干净检出上带 DSN 跑全量。
- 「不做」各条照守：编排、领域与 `cmd/parcel-api` 未动；判断载荷两处内联分格未收拢；POD 重放代数、交接更正内容冲突、POD 缺时刻也签版本号都未碰；NO 收寄分格未碰；带 OnEveryLine 的用例没改名；替身的令牌为空分支与 `HandoverIntake`、`DeliveryIntake` 合集接口都没删；`cmd` 真库表没补更正各行。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `82dc38ea`）

两轴合审，0 阻断。

- **Spec**：三份更正载荷的时刻改走 `parseOptionalInstant`，缺席、空串、纯空白交零值给编排，解不出才 400 并点名该键；交接更正结论空着时交 `HandoverVerdictInvalid` 零值，编排答未受理，与首登口 `HandoverRegistrationPayload.Command` 同形。端点层三口缺 `correctedAt` 答 200 `SOURCE_NOT_ACCEPTED`、册上不多一行。★三组用 overlay 换回 main 版先红（22 条 FAIL）。变异九个全杀，m3c 在 main 版测试下本就被票 41 的用例杀，完成记录照实写了。
- **Standards**：`work_fact_intake.go` 不再有 `time.Parse(`；`HandoverIntake`、`DeliveryIntake` 头注按票面改，没拿 ADR-0151 未裁的越权风险点当理由。与票 41 同名常量的语义冲突以 main 为准，只改时刻写法（同一刻），断言未动。
- 「不做」各条照守：编排、领域、`cmd/parcel-api` 都没动。

### 进 main 记录（2026-10-19，会话 idp-parcel-9b）

- operator-channel/45 与 no-reception-identity/03（分支 `claude-no4503`）、operator-channel/32（分支 `claude-oc32`，暂停前基于 `b58b10ef`，续做时 rebase 过）叠成一串，rebase 到远端 main 之上快进推送。期间远端进了 SELL 客户费用链（另一会话，改 `cmd/parcel-dispatch`、SA、PP 与演示种子），先在旧基上起的那次全量作废，rebase 后重跑。机制清点在叠好的树上重生成（cmd 测试文件 154）。
- 推送方全量一次：钉 `77ffe641` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 144 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。验证之后远端只前进了一笔纯票面认领，再 rebase 一次，未重跑。
